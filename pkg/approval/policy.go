// Package approval decides which tool calls need a human decision and keeps the
// pending requests a console can answer.
package approval

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Action is what the policy wants done with one tool call.
type Action string

const (
	// ActionAllow lets the call run without asking anyone.
	ActionAllow Action = "allow"
	// ActionRequire asks a human; when nobody can answer it fails closed.
	ActionRequire Action = "require"
	// ActionDeny refuses the call outright.
	ActionDeny Action = "deny"
)

// Tool categories. Unknown tools are CategoryOther, which requires approval.
const (
	CategoryRead    = "read"
	CategorySearch  = "search"
	CategoryMessage = "message"
	CategoryAgent   = "agent"
	CategoryExec    = "exec"
	CategoryWrite   = "write"
	CategoryOther   = "other"
)

// Categories is every category a rule may name. It is the single source of
// truth the validator searches, so a new category only has to be added here and
// to Category to stay saveable through the console.
var Categories = []string{
	CategoryRead,
	CategorySearch,
	CategoryMessage,
	CategoryAgent,
	CategoryExec,
	CategoryWrite,
	CategoryOther,
}

// Category maps a runtime tool name to its category.
func Category(tool string) string {
	switch tool {
	case "read_file", "list_dir", "load_image", "find_skills":
		return CategoryRead
	case "web_search", "web_fetch", "tool_search_tool_regex":
		return CategorySearch
	case "message", "reaction", "send_file":
		return CategoryMessage
	case "subagent", "spawn", "spawn_status":
		return CategoryAgent
	case "exec":
		return CategoryExec
	case "write_file", "edit_file", "append_file":
		return CategoryWrite
	default:
		return CategoryOther
	}
}

// PinArg is the argument an "always allow" entry must freeze for this tool, or ""
// when remembering the whole tool is acceptable. Freezing is what keeps
// "always allow exec" from meaning "any shell command, forever".
func PinArg(tool string) string {
	switch tool {
	case "exec":
		return "command"
	case "write_file", "edit_file", "append_file":
		return "path"
	default:
		return ""
	}
}

// Rule is one configured policy entry; the first matching rule wins.
type Rule struct {
	Tool     string `json:"tool,omitempty"`
	Category string `json:"category,omitempty"`
	Action   string `json:"action"`
}

// AllowEntry is a decision a human asked to remember.
type AllowEntry struct {
	Tool  string `json:"tool"`
	Arg   string `json:"arg,omitempty"`
	Value string `json:"value,omitempty"`
}

// Matches reports whether this entry covers the call. An entry with no Arg
// allows the whole tool; an entry with an Arg requires an exact string match.
func (e AllowEntry) Matches(tool string, args map[string]any) bool {
	if e.Tool != tool {
		return false
	}
	if e.Arg == "" {
		return true
	}
	value, ok := args[e.Arg]
	if !ok {
		return false
	}
	text, ok := value.(string)
	return ok && text == e.Value
}

// Describe renders the entry for logs and events.
func (e AllowEntry) Describe() string {
	if e.Arg == "" {
		return e.Tool
	}
	return fmt.Sprintf("%s %s=%s", e.Tool, e.Arg, e.Value)
}

// Policy is the tools.approval config section.
type Policy struct {
	Rules       []Rule
	AlwaysAllow []AllowEntry
}

// Origin describes the turn a call came from. The JSON tags are what the
// console reads, so they must stay snake_case.
type Origin struct {
	Channel    string `json:"channel,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	SenderID   string `json:"sender_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	TurnID     string `json:"turn_id,omitempty"`
	Iteration  int    `json:"iteration,omitempty"`
}

// Decide applies the remembered entries, then the first matching rule, then the
// category default.
func (p Policy) Decide(tool string, args map[string]any, origin Origin) Decision {
	for _, entry := range p.AlwaysAllow {
		if entry.Matches(tool, args) {
			return Decision{Action: ActionAllow, Reason: "allowed without asking: " + entry.Describe()}
		}
	}

	category := Category(tool)
	action := ActionRequire
	if categoryAllowsByDefault(category) {
		action = ActionAllow
	}
	for _, rule := range p.Rules {
		if !rule.matches(tool, category) {
			continue
		}
		switch Action(strings.TrimSpace(rule.Action)) {
		case ActionAllow:
			action = ActionAllow
		case ActionRequire:
			action = ActionRequire
		case ActionDeny:
			action = ActionDeny
		default:
			// An action the validator should have rejected: fail closed.
			action = ActionRequire
		}
		break
	}

	switch action {
	case ActionAllow:
		return Decision{Action: ActionAllow}
	case ActionDeny:
		return Decision{Action: ActionDeny, Reason: fmt.Sprintf("%s is denied by the approval policy", tool)}
	}
	return Decision{
		Action: ActionRequire,
		Reason: fmt.Sprintf("%s needs approval", tool),
		Pin:    PinArg(tool),
	}
}

func (r Rule) matches(tool, category string) bool {
	if r.Tool != "" && r.Tool == tool {
		return true
	}
	return r.Category != "" && r.Category == category
}

func categoryAllowsByDefault(category string) bool {
	switch category {
	case CategoryRead, CategorySearch, CategoryMessage, CategoryAgent:
		return true
	default:
		return false
	}
}

// Summarize renders the one-line description the console shows for a call. It
// only uses string arguments, so it is safe for malformed input.
func Summarize(tool string, args map[string]any) string {
	for _, key := range []string{"command", "path", "message", "query", "url"} {
		if text, ok := args[key].(string); ok && strings.TrimSpace(text) != "" {
			return tool + ": " + oneLine(text)
		}
	}
	if len(args) == 0 {
		return tool
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return tool
	}
	return tool + ": " + oneLine(string(encoded))
}

// oneLine collapses all whitespace runs so a summary stays a single display
// line, however the caller formatted the argument.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// Decision is what Decide tells the caller to do.
type Decision struct {
	Action Action
	Reason string
	Pin    string
}
