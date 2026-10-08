package idemirror

import (
	"strings"
	"time"
)

const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
	ansiUser  = "\x1b[1;36m"
	ansiErr   = "\x1b[31m"
	ansiOK    = "\x1b[32m"
)

// renderer turns the ordered bubble stream into terminal lines. Only the last
// bubble may still change; assistant text is emitted line by line as it grows.
type renderer struct {
	next      int // first header index not yet settled
	openID    string
	emitted   int // bytes of the open bubble's text already printed
	seenLen   int
	toolShown bool
	changedAt time.Time
}

// feed consumes rows (header index >= r.next, ascending) and returns terminal output.
func (r *renderer) feed(rows []bubble, now time.Time) string {
	var sb strings.Builder
	for i, b := range rows {
		if b.Idx < r.next {
			continue
		}
		if !b.Have {
			break
		}
		last := i == len(rows)-1
		if b.ID != r.openID {
			r.openID, r.emitted, r.seenLen, r.toolShown, r.changedAt = b.ID, 0, 0, false, now
		}
		switch {
		case b.Type == 1:
			if strings.TrimSpace(b.Text) != "" {
				sb.WriteString("\r\n" + ansiUser + "❯ " + crlf(strings.TrimSpace(b.Text), "  ") + ansiReset + "\r\n")
			}
			r.settle(b.Idx)
			continue
		case b.Tool != "":
			if !r.toolShown {
				sb.WriteString(ansiDim + "  ⏺ " + toolLine(b.Tool, b.Args) + ansiReset + "\r\n")
				r.toolShown = true
			}
		default:
			sb.WriteString(r.text(b.Text, last, now))
		}
		if !last {
			r.settle(b.Idx)
		}
	}
	return sb.String()
}

// text prints complete new lines of an assistant bubble; a trailing partial line is
// printed once the bubble is settled or has stopped changing for idleFlush.
func (r *renderer) text(full string, last bool, now time.Time) string {
	if len(full) != r.seenLen {
		r.seenLen, r.changedAt = len(full), now
	}
	if len(full) < r.emitted {
		r.emitted = len(full)
		return ""
	}
	pending := full[r.emitted:]
	cut := len(pending)
	if last && now.Sub(r.changedAt) < idleFlush {
		cut = strings.LastIndex(pending, "\n") + 1
	}
	if cut == 0 {
		return ""
	}
	start := ""
	if r.emitted == 0 {
		start = "\r\n"
	}
	chunk := strings.TrimSuffix(pending[:cut], "\n")
	r.emitted += cut
	if strings.TrimSpace(chunk) == "" {
		return ""
	}
	return start + crlf(chunk, "") + "\r\n"
}

const idleFlush = 1500 * time.Millisecond

func (r *renderer) settle(idx int) {
	r.next = idx + 1
	r.openID, r.emitted, r.seenLen, r.toolShown = "", 0, 0, false
}

func crlf(s, indent string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n"+indent)
}

var toolLabels = map[string]string{
	"read_file":            "Read",
	"run_terminal_command": "Shell",
	"edit_file":            "Edit",
	"ripgrep_raw_search":   "Grep",
	"glob_file_search":     "Glob",
	"todo_write":           "Todo",
	"task":                 "Task",
	"await":                "Await",
	"read_lints":           "Lints",
	"ask_question":         "Ask",
	"web_search":           "WebSearch",
	"web_fetch":            "WebFetch",
	"delete_file":          "Delete",
	"get_mcp_tools":        "MCP",
}

// toolLine labels a tool call; summary is the main argument (command, path, pattern…).
func toolLine(name, summary string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(name, "_v2"), "_v3")
	label := toolLabels[base]
	if label == "" {
		label = base
	}
	summary = strings.Join(strings.Fields(summary), " ")
	if r := []rune(summary); len(r) > 90 {
		summary = string(r[:90]) + "…"
	}
	if summary != "" {
		return label + "  " + summary
	}
	return label
}
