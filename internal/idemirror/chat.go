package idemirror

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
)

// oscChatTag carries structured conversation messages so KoKo can render the chat
// natively (markdown, tool rows, questionnaires) instead of the terminal transcript.
const oscChatTag = "\x1b]7790;"

type chatMessage struct {
	Idx      int           `json:"idx"`
	ID       string        `json:"id"`
	Role     string        `json:"role"` // user | assistant | tool | question
	Text     string        `json:"text,omitempty"`
	Tool     *chatTool     `json:"tool,omitempty"`
	Question *chatQuestion `json:"question,omitempty"`
}

type chatTool struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Summary string `json:"summary,omitempty"`
	Status  string `json:"status,omitempty"`
}

type chatQuestion struct {
	ToolCallID string           `json:"toolCallId"`
	Title      string           `json:"title,omitempty"`
	Status     string           `json:"status"` // pending | submitted | cancelled
	Questions  []questionItem   `json:"questions"`
	Answers    []questionAnswer `json:"answers,omitempty"`
}

type questionItem struct {
	ID            string           `json:"id"`
	Prompt        string           `json:"prompt"`
	AllowMultiple bool             `json:"allowMultiple,omitempty"`
	Options       []questionOption `json:"options"`
}

type questionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type questionAnswer struct {
	QuestionID        string   `json:"questionId"`
	SelectedOptionIDs []string `json:"selectedOptionIds"`
	FreeformText      string   `json:"freeformText,omitempty"`
}

type chatFrame struct {
	Reset    bool          `json:"reset,omitempty"`
	Messages []chatMessage `json:"messages"`
}

// chatMessageFor converts a stored bubble; ok is false for rows with nothing to show.
func chatMessageFor(b bubble) (chatMessage, bool) {
	msg := chatMessage{Idx: b.Idx, ID: b.ID}
	switch {
	case b.Type == 1:
		msg.Role, msg.Text = "user", strings.TrimSpace(b.Text)
		return msg, msg.Text != ""
	case strings.HasPrefix(b.Tool, "ask_question"):
		q, ok := parseQuestion(b)
		if !ok {
			return msg, false
		}
		msg.Role, msg.Question = "question", &q
		return msg, true
	case b.Tool != "":
		base := strings.TrimSuffix(strings.TrimSuffix(b.Tool, "_v2"), "_v3")
		label := toolLabels[base]
		if label == "" {
			label = base
		}
		msg.Role = "tool"
		msg.Tool = &chatTool{Name: base, Label: label, Summary: strings.Join(strings.Fields(b.Args), " "), Status: b.Status}
		return msg, true
	default:
		msg.Role, msg.Text = "assistant", b.Text
		return msg, strings.TrimSpace(b.Text) != ""
	}
}

func parseQuestion(b bubble) (chatQuestion, bool) {
	var params struct {
		Title     string         `json:"title"`
		Questions []questionItem `json:"questions"`
	}
	if json.Unmarshal([]byte(b.QParams), &params) != nil || len(params.Questions) == 0 {
		return chatQuestion{}, false
	}
	q := chatQuestion{ToolCallID: b.ToolCallID, Title: params.Title, Status: b.QStatus, Questions: params.Questions}
	if q.Status == "" {
		q.Status = "pending"
	}
	var result struct {
		Answers []questionAnswer `json:"answers"`
	}
	if b.QResult != "" && json.Unmarshal([]byte(b.QResult), &result) == nil {
		q.Answers = result.Answers
	}
	return q, true
}

// publishChatLocked sends rows whose rendering changed since they were last sent.
// Caller holds m.mu.
func (m *mirror) publishChatLocked(rows []bubble, reset bool) {
	if reset {
		m.sentChat = map[int]string{}
	}
	frame := chatFrame{Reset: reset}
	for _, b := range rows {
		if !b.Have {
			break
		}
		msg, ok := chatMessageFor(b)
		if !ok {
			continue
		}
		if q := msg.Question; q != nil && q.Status != "submitted" {
			if a, ok := m.answered[q.ToolCallID]; ok {
				q.Status, q.Answers = "answered", a
			}
		}
		raw, err := json.Marshal(msg)
		if err != nil || m.sentChat[b.Idx] == string(raw) {
			continue
		}
		m.sentChat[b.Idx] = string(raw)
		frame.Messages = append(frame.Messages, msg)
		if msg.Question != nil && msg.Question.Status == "pending" && (m.pendingQ < 0 || b.Idx < m.pendingQ) {
			m.pendingQ = b.Idx
		}
	}
	if !reset && len(frame.Messages) == 0 {
		return
	}
	raw, err := json.Marshal(frame)
	if err != nil {
		return
	}
	_, _ = io.WriteString(m.out, oscChatTag+base64.StdEncoding.EncodeToString(raw)+"\x07")
}

// settlePendingQuestionLocked moves the re-poll floor past questions that are answered.
func (m *mirror) settlePendingQuestionLocked(rows []bubble) {
	if m.pendingQ < 0 {
		return
	}
	for _, b := range rows {
		if b.Idx == m.pendingQ && b.Have {
			if b.QStatus != "" && b.QStatus != "pending" {
				m.pendingQ = -1
			}
			return
		}
	}
}

// answerText turns phone selections into a chat reply Cursor delivers to the model.
func answerText(q chatQuestion, answers []questionAnswer) string {
	byID := map[string]questionAnswer{}
	for _, a := range answers {
		byID[a.QuestionID] = a
	}
	var sb strings.Builder
	for _, item := range q.Questions {
		a, ok := byID[item.ID]
		if !ok {
			continue
		}
		var picked []string
		for _, id := range a.SelectedOptionIDs {
			for _, o := range item.Options {
				if o.ID == id {
					picked = append(picked, o.Label)
				}
			}
		}
		if t := strings.TrimSpace(a.FreeformText); t != "" {
			picked = append(picked, t)
		}
		if len(picked) == 0 {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(strings.TrimSpace(item.Prompt) + "\n→ " + strings.Join(picked, "; "))
	}
	return sb.String()
}
