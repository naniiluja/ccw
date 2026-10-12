package diabridge

// Dia's model gateway is not a general Anthropic endpoint. It accepts the
// request shape Dia's own bundled Claude Code produces and answers anything
// else with HTTP 400, which ends the handler turn and takes its listener down
// with it. Two of those constraints bite every third-party client, so the back
// face rewrites the request before it reaches the gateway.
//
//  1. Tool names. The gateway whitelists Claude Code's PascalCase tool names and
//     rejects anything else with "Request contains an unrecognized tool". omp,
//     OpenAI-shaped SDKs and hand-written clients all use their own lowercase
//     names, so each one is renamed to the Claude Code tool that does the same
//     job.
//
//  2. Thinking. claude-opus-4-8 refuses a fixed thinking budget
//     ("thinking.type.enabled" is not supported for this model); it takes
//     thinking.type=adaptive plus an output_config effort level instead. A fixed
//     budget from the caller is therefore folded into an effort level.

import (
	"encoding/json"
	"io"
	"strings"
)

// effortLevels are the levels the gateway accepts, weakest first. A fixed
// thinking budget is folded onto one of them because the model that needs the
// adaptive form takes an effort level in place of the budget.
var effortLevels = []string{"low", "medium", "high"}

// diaTools maps a caller's tool name to the Claude Code tool Dia accepts.
//
// The values are the whitelist measured one by one against Dia 1.51.1's gateway:
// Bash, Read, Write, Edit, Glob, Grep, TodoWrite, Task, Skill, TaskCreate,
// TaskGet, TaskList and TaskUpdate accept; WebFetch, WebSearch, NotebookEdit,
// NotebookRead, MultiEdit, ExitPlanMode, BashOutput, KillShell, TaskOutput,
// TaskStop, LSP, Monitor, ListAgents and SendMessage are all rejected. A tool
// with no entry is dropped rather than renamed, because sending it under a name
// that does not match its schema would have the model call the wrong tool.
//
// Names already spelled the Claude Code way pass through, so a Claude Code
// client pointed at ccw needs no rewriting at all.
var diaTools = map[string]string{
	// omp and most coding agents.
	"shell": "Bash", "bash": "Bash", "run": "Bash", "exec": "Bash", "terminal": "Bash",
	"read": "Read", "write": "Write", "edit": "Edit", "editfile": "Edit",
	"glob": "Glob", "ls": "Read", "listdir": "Read", "listfiles": "Read",
	"grep": "Grep", "search": "Grep", "ripgrep": "Grep",
	"todo": "TodoWrite", "todowrite": "TodoWrite", "plan": "TodoWrite",
	"task": "Task", "agent": "Task", "subagent": "Task", "agenttool": "Agent",
	"skill": "Skill",
	// The task checklist Claude Code offers on top of TodoWrite. Each one is its
	// own Dia tool, so they are mapped to themselves rather than folded into Task,
	// which is the subagent tool and does something else entirely.
	"taskcreate": "TaskCreate", "taskget": "TaskGet",
	"tasklist": "TaskList", "taskupdate": "TaskUpdate",

	// Claude Code's own names the gateway refuses, each mapped to the tool that
	// does the same job. Measured against Dia 1.51.1's gateway: MultiEdit, LS,
	// NotebookRead, NotebookEdit, TaskOutput, BashOutput, TaskStop, KillShell,
	// SendMessage, ListAgents, ScheduleWakeup, ReportFindings, EnterPlanMode,
	// ExitPlanMode, EnterWorktree, ExitWorktree, CronCreate, CronList, CronDelete,
	// ToolSearch, Workflow, Monitor, LSP, SlashCommand, PushNotification,
	// SendFeedback, EndConversation and the two MCP resource tools are all
	// rejected, so a client asking for them gets the closest tool instead of a
	// silent drop. Asking is better than not having the capability at all: the
	// model reads the description of what it actually got.
	"multiedit": "Edit", "notebookread": "Read", "notebookedit": "Edit",
	"taskoutput": "Read", "bashoutput": "Read",
	"taskstop": "Bash", "killshell": "Bash", "killbash": "Bash",
	"sendmessage": "Task", "listagents": "Task",
	"schedulewakeup": "Bash", "reportfindings": "TodoWrite",
	"enterplanmode": "TodoWrite", "exitplanmode": "TodoWrite",
	"enterworktree": "Bash", "exitworktree": "Bash",
	"croncreate": "TodoWrite", "cronlist": "TodoWrite", "crondelete": "TodoWrite",
	"toolsearch": "Skill", "workflow": "Task", "monitor": "Bash", "lsp": "Grep",
	"slashcommand": "Skill", "pushnotification": "Bash", "sendfeedback": "Bash",
	"endconversation":      "Bash",
	"listmcpresourcestool": "Read", "readmcpresourcetool": "Read",
	"askuserquestion": "AskUserQuestion",

	// Web tools have no counterpart: the gateway refuses WebSearch and WebFetch
	// by name. They are client tools, though. Claude Code runs WebSearch itself
	// through a second request that declares Anthropic's hosted search, which
	// ccw's /v1 answers through Antigravity like it does for every provider, and
	// WebFetch fetches locally. So they only need a name the gateway accepts:
	// see spareSlots.
}

// canonicalDiaTools are the exact Claude Code spellings the gateway accepts.
// A tool whose name is already one of these is forwarded untouched.
var canonicalDiaTools = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "TodoWrite": true, "Task": true, "Skill": true,
	"TaskCreate": true, "TaskGet": true, "TaskList": true, "TaskUpdate": true,
	"Agent": true, "AskUserQuestion": true,
}

// slotTools are the client tools with no Dia counterpart that are sent under a
// whitelisted name the request leaves unused. The gateway checks only the name,
// so the model sees the tool's own schema and description and calls it
// correctly; the reply is renamed back, so the client runs its own tool.
var slotTools = map[string]bool{"websearch": true, "webfetch": true}

// spareSlots are the whitelisted names a slot tool may borrow, most harmless
// first. Claude Code itself declares none of the first three, so they are free
// in its requests; a client that does declare one keeps it.
var spareSlots = []string{"Glob", "Grep", "TodoWrite", "Task", "Skill", "AskUserQuestion"}

// applyCompat rewrites a decoded Messages request into the shape Dia's gateway
// accepts and returns the tool-name mapping, so the reply can be translated back
// into the caller's spelling.
//
// Renaming tools also renames the tool_use blocks that come back inside
// messages, and the tool_choice that selects one, so a name the caller used
// still refers to the same tool.
func applyCompat(body map[string]any) map[string]string {
	renamed := renameTools(body)
	choice, hasChoice := body["tool_choice"].(map[string]any)
	if _, hasTools := body["tools"]; !hasTools {
		// Nothing survived the whitelist, so a tool_choice would pin a tool the
		// request no longer offers.
		delete(body, "tool_choice")
	} else if hasChoice {
		name, _ := choice["name"].(string)
		if target := renamed[name]; target == "" {
			// The pinned tool lost a name collision, so it is not in the request
			// any more. Any tool is a closer answer than a name the gateway will
			// reject.
			delete(body, "tool_choice")
		} else {
			choice["name"] = target
		}
	}
	adaptThinking(body)
	return renamed
}

// renameTools rewrites the request's tool list, plus every tool_use block and
// tool_choice reference inside it, and returns the old-to-new name mapping so
// the reply can be translated back.
//
// Several caller tools can land on one Dia tool: Claude Code's Bash, BashOutput
// and KillBash all have to become the shell tool Dia accepts. Sending those as
// three tools named Bash is refused outright with "tools: Tool names must be
// unique", which ends the handler turn, so only the first of a colliding group
// is sent. The gateway tolerates a tool_use block in the history that names a
// tool the request no longer declares, so the dropped tools keep working as
// past turns instead of breaking the whole conversation.
func renameTools(body map[string]any) map[string]string {
	tools, _ := body["tools"].([]any)
	mapping := make(map[string]string, len(tools))
	kept := make([]any, 0, len(tools))
	taken := make(map[string]string, len(tools))
	var slotted []map[string]any
	for _, entry := range tools {
		t, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, ok := t["name"].(string)
		if !ok || name == "" {
			continue
		}
		target, ok := diaToolName(name)
		if !ok {
			if slotTools[normalizedToolName(name)] {
				slotted = append(slotted, t)
			}
			continue
		}
		if _, alreadySent := taken[target]; alreadySent {
			// The target is already in this request under the caller's earlier
			// name, which is the one the reply will be read back with.
			mapping[name] = ""
			continue
		}
		taken[target] = name
		t["name"] = target
		mapping[name] = target
		kept = append(kept, t)
	}
	// Slot tools go last, so a real Glob or Grep the client declared keeps its
	// own name and only an unused one is borrowed.
	for _, t := range slotted {
		name := t["name"].(string)
		slot := freeSlot(taken)
		if slot == "" {
			break
		}
		taken[slot] = name
		t["name"] = slot
		mapping[name] = slot
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		delete(body, "tools")
		mapping = map[string]string{}
	} else {
		body["tools"] = kept
	}
	rewriteMessageToolNames(body, mapping)
	return mapping
}

// freeSlot returns the first spare whitelisted name the request has not used,
// or "" when every one is taken.
func freeSlot(taken map[string]string) string {
	for _, s := range spareSlots {
		if _, used := taken[s]; !used {
			return s
		}
	}
	return ""
}

// restoredBody rewrites the tool names in a Dia SSE stream while passing every
// other byte through in order, so a streaming reply keeps its exact event
// framing and only the names the caller recognises are changed.
type restoredBody struct {
	body    io.Reader
	reverse map[string]string
	// pending holds a rewritten data line that was longer than the space left
	// in the previous read, so it is returned by the next one.
	pending []byte
}

// Read returns the upstream stream with tool_use names put back into the
// caller's spelling. A read either drains what fits into p or returns a whole
// rewritten line, whichever comes first, so a name is never split across two
// reads.
func (r *restoredBody) Read(p []byte) (int, error) {
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	chunk := make([]byte, len(p))
	n, err := r.body.Read(chunk)
	if n == 0 {
		return 0, err
	}
	// Only whole lines are rewritten; a partial trailing line waits for the
	// rest of itself rather than being emitted split.
	text := string(chunk[:n])
	lines := strings.Split(text, "\n")
	out := make([]byte, 0, len(text)+64)
	for i, line := range lines {
		// The last piece is either an incomplete line or empty.
		last := i == len(lines)-1
		rewritten := line
		if !last {
			rewritten = restoreStreamName(line, r.reverse)
		} else if line != "" {
			r.pending = []byte("\n" + restoreStreamName(line, r.reverse) + "\n")
		}
		out = append(out, rewritten...)
		if !last {
			out = append(out, '\n')
		}
	}
	written := copy(p, out)
	if written < len(out) {
		r.pending = append(out[written:], r.pending...)
	}
	if err != nil && written == len(out) {
		return written, err
	}
	return written, nil
}

// diaToolName reports the Claude Code tool a caller's tool becomes, and whether
// the gateway will take it at all.
func diaToolName(name string) (string, bool) {
	if canonicalDiaTools[name] {
		return name, true
	}
	if target, ok := diaTools[normalizedToolName(name)]; ok {
		return target, true
	}
	return "", false
}

// normalizedToolName makes tool names comparable without regard to case or the
// separators different clients use, so "multi_edit", "multiEdit" and
// "MultiEditTool" all reach the same entry.
func normalizedToolName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r == '_' || r == '-' || r == ' ':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rewriteMessageToolNames renames tool_use blocks in the message history so the
// names inside it match the tool list Dia was sent. tool_result blocks carry no
// name, so they need no change.
//
// A block whose tool lost a name collision keeps its own name: the gateway
// accepts history that names a tool the request no longer declares, so the
// turn that really did use that tool still reads correctly.
func rewriteMessageToolNames(body map[string]any, mapping map[string]string) {
	if len(mapping) == 0 {
		return
	}
	messages, _ := body["messages"].([]any)
	for _, entry := range messages {
		msg, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		blocks, _ := msg["content"].([]any)
		for _, raw := range blocks {
			block, ok := raw.(map[string]any)
			if !ok || block["type"] != "tool_use" {
				continue
			}
			name, ok := block["name"].(string)
			if !ok {
				continue
			}
			if target := mapping[name]; target != "" {
				block["name"] = target
			}
		}
	}
}

// adaptThinking puts a request's thinking into a form Dia's Bedrock upstream
// accepts. Each refusal below ends the handler turn, and the bridge with it.
//
//   - A fixed budget ("enabled") is refused for this model family, so it becomes
//     the adaptive form with an effort level.
//   - "disabled" is refused too ("thinking.type.disabled is not supported for
//     this model"). Claude Code sends it on the side request its WebSearch tool
//     makes. A request with no thinking field is accepted, so the field goes.
//   - An adaptive display of "updates", which Claude Code sends, is refused with
//     "Input should be 'summarized', 'omitted'". "summarized" is the closest: the
//     client still sees thinking text, only condensed.
func adaptThinking(body map[string]any) {
	thinking, ok := body["thinking"].(map[string]any)
	if !ok {
		return
	}
	kind, _ := thinking["type"].(string)
	switch kind {
	case "adaptive":
		if d, has := thinking["display"].(string); has && d != "summarized" && d != "omitted" {
			thinking["display"] = "summarized"
		}
		return
	case "disabled":
		delete(body, "thinking")
		return
	case "enabled":
	default:
		return
	}
	budget, _ := thinking["budget_tokens"].(float64)
	body["thinking"] = map[string]any{"type": "adaptive"}
	body["output_config"] = map[string]any{"effort": effortForBudget(budget)}
}

// effortForBudget maps a fixed thinking budget onto the effort level that
// replaces it. Budgets are the Anthropic shape (a minimum of 1024), so the
// thresholds sit where the levels actually differ.
func effortForBudget(budget float64) string {
	switch {
	case budget >= 24000:
		return effortLevels[2]
	case budget >= 8000:
		return effortLevels[1]
	default:
		return effortLevels[0]
	}
}

// reverseMapping inverts the rename map so a reply's Dia names can be looked up
// by the caller's name. A tool that lost a name collision maps to the empty
// string and is left out entirely: there was only ever one Dia tool for it, so
// the reply belongs to the caller tool that was sent first.
func reverseMapping(mapping map[string]string) map[string]string {
	if len(mapping) == 0 {
		return nil
	}
	out := make(map[string]string, len(mapping))
	for caller, dia := range mapping {
		if dia == "" {
			continue
		}
		out[dia] = caller
	}
	return out
}

// contentBlockStartOfToolUse is the only SSE event that carries a tool name on
// the way out, so a streaming reply is rewritten by touching nothing else.
type contentBlockStartOfToolUse struct {
	Type         string `json:"type"`
	ContentBlock struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"content_block"`
}

// restoreStreamName rewrites the tool name in one content_block_start data
// line and leaves every other line byte-identical. A line that does not name a
// tool is returned untouched, so streaming stays chunk-for-chunk.
func restoreStreamName(line string, reverse map[string]string) string {
	payload, ok := strings.CutPrefix(line, "data: ")
	if !ok {
		return line
	}
	var probe contentBlockStartOfToolUse
	if json.Unmarshal([]byte(payload), &probe) != nil {
		return line
	}
	if probe.Type != "content_block_start" || probe.ContentBlock.Type != "tool_use" {
		return line
	}
	original, ok := reverse[probe.ContentBlock.Name]
	if !ok {
		return line
	}
	var fields map[string]any
	if json.Unmarshal([]byte(payload), &fields) != nil {
		return line
	}
	block, ok := fields["content_block"].(map[string]any)
	if !ok {
		return line
	}
	block["name"] = original
	rewritten, err := json.Marshal(fields)
	if err != nil {
		return line
	}
	return "data: " + string(rewritten)
}
