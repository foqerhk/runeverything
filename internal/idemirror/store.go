package idemirror

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var composerIDRE = regexp.MustCompile(`^[0-9A-Za-z-]{8,64}$`)

// cursorStore reads one Cursor IDE composer from the global state DB (read-only).
type cursorStore struct {
	sqlite string
	db     string
	id     string
}

type composerMeta struct {
	Name           string        `json:"name"`
	Cwd            string        `json:"cwd"`
	Count          int           `json:"count"`
	Mode           string        `json:"mode"`
	Model          string        `json:"model"`
	ContextPercent float64       `json:"contextPercent"`
	LinesAdded     int           `json:"linesAdded"`
	LinesRemoved   int           `json:"linesRemoved"`
	Files          []changedFile `json:"files"`
	CreatedFiles   []string      `json:"createdFiles"`
}

type changedFile struct {
	URI string `json:"uri"`
	New bool   `json:"new"`
}

// option is a selectable Cursor mode or model.
type option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// bubble is one rendered unit of a Cursor conversation, in header order.
type bubble struct {
	Idx      int    `json:"idx"`
	ID       string `json:"id"`
	Type     int    `json:"type"` // 1 user, 2 assistant
	Text     string `json:"text"`
	Tool     string `json:"tool"`
	Status   string `json:"status"`
	Args     string `json:"args"`
	Thinking bool   `json:"thinking"`
	Have     bool   `json:"have"` // false until Cursor has written the bubble row
	// ask_question only: raw params / result JSON and questionnaire status.
	ToolCallID string `json:"toolCallId"`
	QParams    string `json:"qparams"`
	QStatus    string `json:"qstatus"`
	QResult    string `json:"qresult"`
}

func newCursorStore(db, composerID string) (*cursorStore, error) {
	if !composerIDRE.MatchString(composerID) {
		return nil, fmt.Errorf("invalid composer id %q", composerID)
	}
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, errors.New("sqlite3 not found")
	}
	return &cursorStore{sqlite: sqlite, db: db, id: composerID}, nil
}

func (s *cursorStore) query(sql string) ([]string, error) {
	out, err := exec.Command(s.sqlite, "-readonly", "file:"+s.db+"?mode=ro", sql).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("sqlite3: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func (s *cursorStore) meta() (composerMeta, error) {
	lines, err := s.query(fmt.Sprintf(`select json_object(
  'name', json_extract(c.value, '$.name'),
  'cwd', json_extract(c.value, '$.workspaceIdentifier.uri.fsPath'),
  'count', coalesce(json_array_length(c.value, '$.fullConversationHeadersOnly'), 0),
  'mode', coalesce(json_extract(c.value, '$.unifiedMode'), ''),
  'model', coalesce(json_extract(c.value, '$.modelConfig.modelName'), ''),
  'contextPercent', coalesce(json_extract(c.value, '$.contextUsagePercent'), 0),
  'linesAdded', coalesce(json_extract(c.value, '$.totalLinesAdded'), 0),
  'linesRemoved', coalesce(json_extract(c.value, '$.totalLinesRemoved'), 0),
  'files', (select json_group_array(json_object('uri', f.key,
      'new', json(case when json_extract(f.value, '$.isNewlyCreated') then 'true' else 'false' end)))
    from json_each(c.value, '$.originalFileStates') f),
  'createdFiles', (select json_group_array(json_extract(n.value, '$.uri.external'))
    from json_each(c.value, '$.newlyCreatedFiles') n))
from cursorDiskKV c where c.key = 'composerData:%s'`, s.id))
	if err != nil {
		return composerMeta{}, err
	}
	if len(lines) == 0 {
		return composerMeta{}, errors.New("conversation not found in Cursor")
	}
	var m composerMeta
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		return composerMeta{}, err
	}
	return m, nil
}

// options lists Cursor's agent modes and the models enabled in its model picker.
func (s *cursorStore) options() (modes, models []option, err error) {
	lines, err := s.query(`select json_object(
  'modes', json_extract(value, '$.composerState.modes4'),
  'models', json_extract(value, '$.availableDefaultModels2'),
  'on', json_extract(value, '$.aiSettings.modelOverrideEnabled'),
  'off', json_extract(value, '$.aiSettings.modelOverrideDisabled'))
from ItemTable where key = 'src.vs.platform.reactivestorage.browser.reactiveStorageServiceImpl.persistentStorage.applicationUser'`)
	if err != nil || len(lines) == 0 {
		return nil, nil, err
	}
	var raw struct {
		Modes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"modes"`
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"clientDisplayName"`
			DefaultOn   bool   `json:"defaultOn"`
			Agent       *bool  `json:"supportsAgent"`
		} `json:"models"`
		On  []string `json:"on"`
		Off []string `json:"off"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &raw); err != nil {
		return nil, nil, err
	}
	for _, m := range raw.Modes {
		if m.ID != "" && m.ID != "background" {
			modes = append(modes, option{ID: m.ID, Name: m.Name})
		}
	}
	on := map[string]bool{}
	for _, id := range raw.On {
		on[id] = true
	}
	off := map[string]bool{}
	for _, id := range raw.Off {
		off[id] = true
	}
	for _, m := range raw.Models {
		if m.Agent != nil && !*m.Agent {
			continue
		}
		if (m.DefaultOn && !off[m.Name]) || on[m.Name] {
			name := m.DisplayName
			if name == "" {
				name = m.Name
			}
			models = append(models, option{ID: m.Name, Name: name})
		}
	}
	return modes, models, nil
}

// newComposerSince finds a composer created in folder at or after sinceMs.
func (s *cursorStore) newComposerSince(folder string, sinceMs int64) (string, error) {
	lines, err := s.query(fmt.Sprintf(`select substr(key, 14) from cursorDiskKV
where key like 'composerData:%%'
  and json_valid(value)
  and json_extract(value, '$.workspaceIdentifier.uri.fsPath') = '%s'
  and coalesce(json_extract(value, '$.createdAt'), 0) >= %d
order by json_extract(value, '$.createdAt') desc limit 1`, strings.ReplaceAll(folder, "'", "''"), sinceMs))
	if err != nil || len(lines) == 0 {
		return "", err
	}
	return strings.TrimSpace(lines[0]), nil
}

// bubbles returns conversation entries with header index >= from.
func (s *cursorStore) bubbles(from int) ([]bubble, error) {
	lines, err := s.query(fmt.Sprintf(`with h as (
  select cast(j.key as integer) as idx,
         json_extract(j.value, '$.bubbleId') as bid,
         json_extract(j.value, '$.type') as htype
  from cursorDiskKV c, json_each(c.value, '$.fullConversationHeadersOnly') j
  where c.key = 'composerData:%[1]s'),
j as (
  select h.idx, h.bid, h.htype, b.key as bkey, b.value as v,
         coalesce(nullif(json_extract(b.value, '$.toolFormerData.rawArgs'), ''),
                  json_extract(b.value, '$.toolFormerData.params'), '') as p
  from h left join cursorDiskKV b on b.key = 'bubbleId:%[1]s:' || h.bid
  where h.idx >= %[2]d)
select json_object(
  'idx', idx,
  'id', bid,
  'type', coalesce(json_extract(v, '$.type'), htype),
  'text', coalesce(json_extract(v, '$.text'), ''),
  'tool', coalesce(json_extract(v, '$.toolFormerData.name'), ''),
  'status', coalesce(json_extract(v, '$.toolFormerData.status'), ''),
  'args', case when json_valid(p) and json_type(p) = 'object' then substr(coalesce(
    json_extract(p, '$.command'), json_extract(p, '$.targetFile'), json_extract(p, '$.target_file'),
    json_extract(p, '$.relativeWorkspacePath'), json_extract(p, '$.path'), json_extract(p, '$.file_path'),
    json_extract(p, '$.pattern'), json_extract(p, '$.globPattern'), json_extract(p, '$.glob_pattern'),
    json_extract(p, '$.query'), json_extract(p, '$.searchTerm'), json_extract(p, '$.url'),
    json_extract(p, '$.description'), ''), 1, 300) else '' end,
  'thinking', json(case when json_extract(v, '$.thinking') is not null then 'true' else 'false' end),
  'have', json(case when bkey is not null then 'true' else 'false' end),
  'toolCallId', coalesce(json_extract(v, '$.toolFormerData.toolCallId'), ''),
  'qparams', case when json_extract(v, '$.toolFormerData.name') like 'ask_question%%' then coalesce(p, '') else '' end,
  'qstatus', case when json_extract(v, '$.toolFormerData.name') like 'ask_question%%'
    then coalesce(json_extract(v, '$.toolFormerData.additionalData.status'), '') else '' end,
  'qresult', case when json_extract(v, '$.toolFormerData.name') like 'ask_question%%'
    then coalesce(json_extract(v, '$.toolFormerData.result'), '') else '' end)
from j
order by idx`, s.id, from))
	if err != nil {
		return nil, err
	}
	out := make([]bubble, 0, len(lines))
	for _, l := range lines {
		var b bubble
		if json.Unmarshal([]byte(l), &b) == nil {
			out = append(out, b)
		}
	}
	return out, nil
}
