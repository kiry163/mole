package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func TestHandleListSkills(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(workspace, "skills", "workspace-skill"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workspace, "skills", "workspace-skill", "SKILL.md"),
		[]byte("---\nname: workspace-skill\ndescription: Workspace skill\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(workspace skill) error = %v", err)
	}

	globalSkillDir := filepath.Join(globalConfigDir(), "skills", "global-skill")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(globalSkillDir, "SKILL.md"),
		[]byte("---\nname: global-skill\ndescription: Global skill\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(global skill) error = %v", err)
	}

	builtinRoot := filepath.Join(t.TempDir(), "builtin-skills")
	oldBuiltin := os.Getenv("MOLE_BUILTIN_SKILLS")
	if err := os.Setenv("MOLE_BUILTIN_SKILLS", builtinRoot); err != nil {
		t.Fatalf("Setenv(MOLE_BUILTIN_SKILLS) error = %v", err)
	}
	defer func() {
		if oldBuiltin == "" {
			_ = os.Unsetenv("MOLE_BUILTIN_SKILLS")
		} else {
			_ = os.Setenv("MOLE_BUILTIN_SKILLS", oldBuiltin)
		}
	}()

	builtinSkillDir := filepath.Join(builtinRoot, "builtin-skill")
	if err := os.MkdirAll(builtinSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(builtin skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(builtinSkillDir, "SKILL.md"),
		[]byte("---\nname: builtin-skill\ndescription: Builtin skill\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(builtin skill) error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp skillSupportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(resp.Skills) != 3 {
		t.Fatalf("skills count = %d, want 3", len(resp.Skills))
	}

	gotSkills := make(map[string]string, len(resp.Skills))
	gotOriginKinds := make(map[string]string, len(resp.Skills))
	for _, skill := range resp.Skills {
		gotSkills[skill.Name] = skill.Source
		gotOriginKinds[skill.Name] = skill.OriginKind
	}
	if gotSkills["workspace-skill"] != "workspace" {
		t.Fatalf("workspace-skill source = %q, want workspace", gotSkills["workspace-skill"])
	}
	if gotSkills["global-skill"] != "global" {
		t.Fatalf("global-skill source = %q, want global", gotSkills["global-skill"])
	}
	if gotSkills["builtin-skill"] != "builtin" {
		t.Fatalf("builtin-skill source = %q, want builtin", gotSkills["builtin-skill"])
	}
	if gotOriginKinds["workspace-skill"] != "builtin" {
		t.Fatalf("workspace-skill origin_kind = %q, want builtin", gotOriginKinds["workspace-skill"])
	}
	if gotOriginKinds["global-skill"] != "builtin" {
		t.Fatalf("global-skill origin_kind = %q, want builtin", gotOriginKinds["global-skill"])
	}
	if gotOriginKinds["builtin-skill"] != "builtin" {
		t.Fatalf("builtin-skill origin_kind = %q, want builtin", gotOriginKinds["builtin-skill"])
	}
}

func TestHandleGetSkill(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	skillDir := filepath.Join(workspace, "skills", "viewer-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte(
			"---\nname: viewer-skill\ndescription: Viewable skill\n---\n# Viewer Skill\n\nThis is visible content.\n",
		),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills/viewer-skill", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp skillDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.Name != "viewer-skill" || resp.Source != "workspace" || resp.Description != "Viewable skill" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp.OriginKind != "builtin" {
		t.Fatalf("resp.OriginKind = %q, want builtin", resp.OriginKind)
	}
	if resp.Content != "# Viewer Skill\n\nThis is visible content.\n" {
		t.Fatalf("content = %q", resp.Content)
	}
}

func TestHandleGetSkillUsesResolvedPath(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	skillDir := filepath.Join(workspace, "skills", "folder-name")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: display-name\ndescription: Mismatched path skill\n---\n# Display Name\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills/display-name", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp skillDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.Name != "display-name" {
		t.Fatalf("resp.Name = %q, want display-name", resp.Name)
	}
	if resp.Content != "# Display Name\n" {
		t.Fatalf("content = %q", resp.Content)
	}
}

func TestHandleImportSkill(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "Plain Skill.md")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	_, err = io.WriteString(part, "# Plain Skill\n\nUse this skill to test imports.\n")
	if err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/skills/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	skillFile := filepath.Join(workspace, "skills", "plain-skill", "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	expected := "---\nname: plain-skill\ndescription: Plain Skill\n---\n\n# Plain Skill\n\nUse this skill to test imports.\n"
	if string(content) != expected {
		t.Fatalf("saved skill content mismatch:\n%s", string(content))
	}
	metaContent, err := os.ReadFile(filepath.Join(workspace, "skills", "plain-skill", ".skill-origin.json"))
	if err != nil {
		t.Fatalf("ReadFile(origin metadata) error = %v", err)
	}
	var originMeta installedSkillOriginMeta
	if err := json.Unmarshal(metaContent, &originMeta); err != nil {
		t.Fatalf("Unmarshal(origin metadata) error = %v", err)
	}
	if originMeta.OriginKind != "manual" {
		t.Fatalf("originMeta.OriginKind = %q, want manual", originMeta.OriginKind)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", rec2.Code, http.StatusOK, rec2.Body.String())
	}
	var listResp skillSupportResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Unmarshal list response error = %v", err)
	}
	found := false
	for _, skill := range listResp.Skills {
		if skill.Name == "plain-skill" && skill.Source == "workspace" && skill.Description == "Plain Skill" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plain-skill should be listed after import, got %#v", listResp.Skills)
	}
}

func TestHandleImportSkillZip(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, loadErr := config.LoadConfig(configPath)
	if loadErr != nil {
		t.Fatalf("LoadConfig() error = %v", loadErr)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	zipContent := buildSkillZip(t, map[string]string{
		"Wrapped Skill/SKILL.md":       "---\nname: wrapped-skill\ndescription: Wrapped skill\n---\n# Wrapped Skill\n\nUse this skill from zip.\n",
		"Wrapped Skill/docs/README.md": "# Extra file\n",
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, createErr := writer.CreateFormFile("file", "Wrapped Skill.zip")
	if createErr != nil {
		t.Fatalf("CreateFormFile() error = %v", createErr)
	}
	if _, writeErr := part.Write(zipContent); writeErr != nil {
		t.Fatalf("Write(zipContent) error = %v", writeErr)
	}
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/skills/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	skillDir := filepath.Join(workspace, "skills", "wrapped-skill")
	skillFile := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	expected := "---\nname: wrapped-skill\ndescription: Wrapped skill\n---\n\n# Wrapped Skill\n\nUse this skill from zip.\n"
	if string(content) != expected {
		t.Fatalf("saved skill content mismatch:\n%s", string(content))
	}

	extraFile := filepath.Join(skillDir, "docs", "README.md")
	extraContent, err := os.ReadFile(extraFile)
	if err != nil {
		t.Fatalf("ReadFile(extra file) error = %v", err)
	}
	if string(extraContent) != "# Extra file\n" {
		t.Fatalf("extra file content = %q", string(extraContent))
	}
}

func TestHandleImportSkillZipRejectsArchiveWithoutSkill(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, loadErr := config.LoadConfig(configPath)
	if loadErr != nil {
		t.Fatalf("LoadConfig() error = %v", loadErr)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	zipContent := buildSkillZip(t, map[string]string{
		"README.md": "# Not a skill\n",
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "invalid.zip")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(zipContent); err != nil {
		t.Fatalf("Write(zipContent) error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/skills/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", "invalid")); !os.IsNotExist(err) {
		t.Fatalf("invalid archive should not leave behind a skill dir, stat err=%v", err)
	}
}

func TestHandleDeleteSkill(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	skillDir := filepath.Join(workspace, "skills", "delete-me")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: delete-me\ndescription: delete me\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/skills/delete-me", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("skill directory should be removed, stat err=%v", err)
	}
}

func TestHandleDeleteSkillPrefersWorkspaceMatch(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	homeDir := t.TempDir()
	t.Setenv(config.EnvHome, homeDir)
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	workspaceSkillDir := filepath.Join(workspace, "skills", "delete-me-workspace")
	if err := os.MkdirAll(workspaceSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workspaceSkillDir, "SKILL.md"),
		[]byte("---\nname: delete-me\ndescription: workspace delete me\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(workspace) error = %v", err)
	}

	globalSkillDir := filepath.Join(homeDir, "skills", "delete-me-global")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(globalSkillDir, "SKILL.md"),
		[]byte("---\nname: delete-me\ndescription: global delete me\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/skills/delete-me", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, err := os.Stat(workspaceSkillDir); !os.IsNotExist(err) {
		t.Fatalf("workspace skill directory should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(globalSkillDir); err != nil {
		t.Fatalf("global skill directory should remain, stat err=%v", err)
	}
}

func buildSkillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := io.WriteString(writer, content); err != nil {
			t.Fatalf("WriteString(%q) error = %v", name, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}
