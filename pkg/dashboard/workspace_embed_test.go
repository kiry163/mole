package dashboard

import (
	"io/fs"
	"testing"
)

// The workspace template is what a fresh installation starts from, so the
// embedded tree must actually carry the files the agent reads at startup:
// loadAgentDefinition (pkg/agent/definition.go) reads AGENT.md/SOUL.md/USER.md
// and the skills loader (pkg/skills/loader.go) walks skills/*/SKILL.md. A
// missing persona file or an empty skill set is invisible until an operator
// notices an anonymous, skill-less agent, so it is pinned here.
func TestWorkspaceTemplateCarriesAgentDefinitionAndSkills(t *testing.T) {
	for _, name := range []string{"AGENT.md", "SOUL.md", "USER.md"} {
		p := workspaceTemplateRoot + "/" + name
		data, err := fs.ReadFile(workspaceTemplate, p)
		if err != nil {
			t.Fatalf("embedded template is missing %s: %v", p, err)
		}
		if len(data) == 0 {
			t.Fatalf("embedded template %s is empty", p)
		}
	}

	skills, err := fs.Glob(workspaceTemplate, workspaceTemplateRoot+"/skills/*/SKILL.md")
	if err != nil {
		t.Fatalf("glob embedded skills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("embedded template carries no skills/*/SKILL.md")
	}
}

// The template ships two shell scripts that the expansion layer has to reproduce
// as executable - a non-executable wait-for-text.sh silently breaks the tmux
// skill.
//
// The bit cannot come from the embed: embed.FS reports 0444 for every embedded
// file (0555 for directories) regardless of the source file's mode, so nothing
// downstream may infer executability from info.Mode(). Both halves of that
// contract are pinned here: the embed really carries the scripts, and it really
// reports no executable bit. That the tracked sources are 100755 in git is a
// plan-level check (`git ls-files -s pkg/dashboard/workspace/`), deliberately
// not a stat of a working tree whose bits depend on core.fileMode.
func TestWorkspaceTemplateCarriesExecutableTmuxScripts(t *testing.T) {
	scripts := []string{"find-sessions.sh", "wait-for-text.sh"}
	for _, name := range scripts {
		p := workspaceTemplateRoot + "/skills/tmux/scripts/" + name
		data, err := fs.ReadFile(workspaceTemplate, p)
		if err != nil {
			t.Fatalf("embedded template is missing %s: %v", p, err)
		}
		if len(data) == 0 {
			t.Fatalf("embedded template %s is empty", p)
		}
		info, err := fs.Stat(workspaceTemplate, p)
		if err != nil {
			t.Fatalf("stat embedded %s: %v", p, err)
		}
		if info.Mode()&0o111 != 0 {
			t.Fatalf("embedded %s mode = %v, want no executable bit: go:embed went back to preserving modes, so the expansion layer could rely on info.Mode() again", p, info.Mode())
		}
	}
}
