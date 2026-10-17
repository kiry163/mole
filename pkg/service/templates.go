// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

// launchdConfig is passed to kardianos/service as the LaunchdConfig option for
// user services. The service manager supplies Name, Path, Arguments,
// WorkingDirectory, EnvVars, StandardOutPath and StandardErrorPath; the fixed
// restart and shutdown timings keep a crashed agent from hot-looping while
// still allowing an in-flight gateway shutdown to finish.
const launchdConfig = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{Name | html}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{Path | html}}</string>
{{range Arguments}}		<string>{{. | html}}</string>
{{end}}	</array>
	{{if WorkingDirectory}}<key>WorkingDirectory</key>
	<string>{{WorkingDirectory | html}}</string>
	{{end}}{{if EnvVars}}<key>EnvironmentVariables</key>
	<dict>
{{range EnvVars}}{{.}}
{{end}}	</dict>
	{{end}}<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>ExitTimeOut</key>
	<integer>45</integer>
	<key>ProcessType</key>
	<string>Background</string>
	{{if StandardOutPath}}<key>StandardOutPath</key>
	<string>{{StandardOutPath | html}}</string>
	{{end}}{{if StandardErrorPath}}<key>StandardErrorPath</key>
	<string>{{StandardErrorPath | html}}</string>
	{{end}}</dict>
</plist>
`

// systemdConfig is passed to kardianos/service as the SystemdScript option for
// user services. It runs in the user manager (WantedBy=default.target), logs to
// the journal, and uses the same restart/stop timings as launchd.
const systemdConfig = `[Unit]
Description={{Description}}
ConditionFileIsExecutable={{Path | cmdEscape}}
[Service]
WorkingDirectory={{WorkingDirectory | cmdEscape}}
ExecStart={{Path | cmdEscape}}{{range Arguments}} {{. | cmd}}{{end}}
Restart=on-failure
RestartSec=10
TimeoutStopSec=45
{{range EnvVars}}{{.}}
{{end}}StandardOutput=journal
StandardError=journal
[Install]
WantedBy=default.target
`
