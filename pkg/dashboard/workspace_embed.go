package dashboard

import "embed"

// workspaceTemplateRoot is the directory name the template tree is embedded
// under, and therefore the path prefix every workspaceTemplate entry carries.
const workspaceTemplateRoot = "workspace"

// workspaceTemplate embeds the workspace skeleton a fresh installation starts
// from: the agent definition files (AGENT.md, SOUL.md, USER.md), the memory
// seed and the shipped skill set.
//
// The tree lives next to this file because Go's embed directive cannot reach a
// parent directory - the same reason pkg/dashboard/dist lives here. `all:` is
// what pkg/dashboard embeds dist with, and it keeps the directive correct if a
// dotted or underscored file is ever added to the tree.
//
//go:embed all:workspace
var workspaceTemplate embed.FS
