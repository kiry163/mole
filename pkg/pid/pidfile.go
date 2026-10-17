package pid

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
)

const pidFileName = ".mole.pid"

var errInvalidPidFile = errors.New("invalid pid file")

// projectProcessNames are the accepted binary base names.
var projectProcessNames = []string{"mole"}

// isProjectProcessName reports whether name — a /proc/<pid>/comm value or a
// full executable path — belongs to this project. The comparison is done on the
// base name (sans directory and ".exe"), also accepting "name-<suffix>" variants,
// so that unrelated processes merely containing the substring (e.g. "guacamole")
// are not mistaken for one of ours.
func isProjectProcessName(name string) bool {
	base := strings.TrimSuffix(filepath.Base(strings.ToLower(strings.TrimSpace(name))), ".exe")
	if base == "" || base == "." {
		return false
	}
	for _, n := range projectProcessNames {
		if base == n || strings.HasPrefix(base, n+"-") {
			return true
		}
	}
	return false
}

// PidFileData is the JSON structure stored in the PID file.
type PidFileData struct {
	PID     int    `json:"pid"`
	Token   string `json:"token"`
	Version string `json:"version"`
	Port    int    `json:"port"`
	Host    string `json:"host"`
}

var pidMu sync.Mutex

// pidFilePath returns the absolute path for the PID file given the home directory.
func pidFilePath(homePath string) string {
	return filepath.Join(homePath, pidFileName)
}

// FilePath returns the absolute path of the PID file for a mole home.
func FilePath(homePath string) string {
	return pidFilePath(homePath)
}

// generateToken creates a cryptographically random 32-character hex token.
func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to something pseudo-random if crypto/rand fails
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// WritePidFile creates (or overwrites) the PID file atomically.
// It returns an error if another gateway instance appears to be running
// (a valid PID file exists with a live process).
func WritePidFile(homePath, host string, port int) (*PidFileData, error) {
	pidMu.Lock()
	defer pidMu.Unlock()

	pidPath := pidFilePath(homePath)

	// Check for existing PID file → singleton enforcement.
	if data, err := readPidFileUnlocked(pidPath); err == nil {
		if os.Getpid() != data.PID {
			logger.Infof("found pid file (PID: %d, version: %s)", data.PID, data.Version)
			// PID 1 is typically init/systemd on the host or the entrypoint
			// inside a container. When a container stops and leaves behind a
			// PID file on a shared volume, the host's PID 1 (init) would
			// pass the isProcessRunning check, blocking new gateway starts.
			// Treat recorded PID 1 as always stale.
			if data.PID != 1 && isProcessRunning(data.PID) {
				// Verify the process is actually a mole instance.
				// If the PID was reused by an unrelated process
				// (e.g. systemd-resolved after a kill -9), treat
				// the PID file as stale and proceed with startup.
				if isMoleProcess(data.PID) {
					home, err := filepath.Abs(homePath)
					if err != nil {
						home = filepath.Clean(homePath)
					}
					return nil, fmt.Errorf(
						"config home %s is already in use by mole (PID %d); use a different MOLE_HOME or stop that instance",
						home, data.PID)
				}
				logger.Warnf("found pid file (PID: %d) but process is not mole", data.PID)
			}
			logger.Warnf("not running (PID: %d) so will remove the pid file: %s", data.PID, pidPath)
		}
		// Stale PID file; process no longer exists → clean up.
		os.Remove(pidPath)
	}

	data := &PidFileData{
		PID:     os.Getpid(),
		Version: config.GetVersion(),
		Port:    port,
		Host:    host,
	}

	token := generateToken()
	data.Token = token

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pid file: %w", err)
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(pidPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create pid directory: %w", err)
	}

	// Write atomically via temp file + rename.
	tmp := pidPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write pid file: %w", err)
	}
	if err := os.Rename(tmp, pidPath); err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("failed to rename pid file: %w", err)
	}
	logger.Debugf("wrote pid file: %s success", pidPath)

	return data, nil
}

// ReadPidFileWithCheck reads the PID file and additionally checks if
// the recorded process is still alive. Returns nil if the file is
// missing, unreadable, or the process has exited.
func ReadPidFileWithCheck(homePath string) *PidFileData {
	pidMu.Lock()
	defer pidMu.Unlock()

	pidPath := pidFilePath(homePath)
	data, err := readPidFileUnlocked(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		if errors.Is(err, errInvalidPidFile) {
			logger.Warnf("invalid pid file, remove it: %s (%v)", pidPath, err)
			_ = os.Remove(pidPath)
			return nil
		}
		logger.Debugf("failed to read pid file: %s", err)
		return nil
	}

	// Treat PID 1 as stale when we are not PID 1 ourselves (container
	// leftover on a shared volume — host PID 1 is init, not gateway).
	if data.PID == 1 && os.Getpid() != 1 {
		logger.Debugf("stale container PID 1, remove pid file: %s", pidPath)
		os.Remove(pidPath)
		return nil
	}

	if !isProcessRunning(data.PID) {
		logger.Debugf("process not running, remove pid file: %s", pidPath)
		os.Remove(pidPath)
		return nil
	}

	return data
}

// RemovePidFile deletes the PID file (e.g. on graceful shutdown).
func RemovePidFile(homePath string) {
	pidMu.Lock()
	defer pidMu.Unlock()

	pidPath := pidFilePath(homePath)
	// Only remove if the PID matches our own process (avoid deleting
	// a file that belongs to a newer gateway instance).
	if data, err := readPidFileUnlocked(pidPath); err == nil {
		if data.PID != os.Getpid() {
			return
		}
	}

	logger.Infof("remove pid file: %s", pidPath)
	os.Remove(pidPath)
}

// RemovePidFileIfPID deletes the PID file only when the recorded PID matches
// expectedPID. It returns true when the file is removed successfully.
func RemovePidFileIfPID(homePath string, expectedPID int) bool {
	if expectedPID <= 0 {
		return false
	}

	pidMu.Lock()
	defer pidMu.Unlock()

	pidPath := pidFilePath(homePath)
	data, err := readPidFileUnlocked(pidPath)
	if err != nil {
		return false
	}
	if data.PID != expectedPID {
		return false
	}
	if err := os.Remove(pidPath); err != nil {
		return false
	}
	return true
}

// readPidFileUnlocked reads the PID file without acquiring the lock.
// Caller must hold pidMu.
func readPidFileUnlocked(pidPath string) (*PidFileData, error) {
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return nil, err
	}

	var data PidFileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidPidFile, err)
	}

	// Validate PID is a positive integer.
	if data.PID <= 0 {
		return nil, fmt.Errorf("%w: pid=%d", errInvalidPidFile, data.PID)
	}

	return &data, nil
}
