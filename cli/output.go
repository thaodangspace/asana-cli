package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/dtonair/asana-cli/asana"
)

// successEnvelope is the default machine-readable success output.
type successEnvelope struct {
	OK   bool `json:"ok"`
	Data any  `json:"data"`
}

// errorEnvelope is the default machine-readable error output.
type errorEnvelope struct {
	OK    bool        `json:"ok"`
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Status  int    `json:"status,omitempty"`
	Method  string `json:"method,omitempty"`
	Path    string `json:"path,omitempty"`
}

// writeJSON encodes v as indented JSON with a trailing newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// writeSuccess renders a successful result. In human mode it writes humanText;
// otherwise it writes the JSON success envelope wrapping data.
func writeSuccess(w io.Writer, data any, human bool, humanText string) error {
	if human {
		_, err := fmt.Fprintln(w, humanText)
		return err
	}
	return writeJSON(w, successEnvelope{OK: true, Data: data})
}

// writeError renders an error. In human mode it writes the plain message;
// otherwise it writes the JSON error envelope. The token never appears here.
func writeError(w io.Writer, err error, human bool) {
	if human {
		fmt.Fprintln(w, err.Error())
		return
	}
	detail := errorDetail{Message: err.Error()}
	if he, ok := err.(*asana.HTTPError); ok {
		detail.Status = he.Status
		detail.Method = he.Method
		detail.Path = he.Path
	}
	_ = writeJSON(w, errorEnvelope{OK: false, Error: detail})
}

// resource captures the subset of Asana fields used by the human summarizers.
type resource struct {
	GID             string `json:"gid"`
	Name            string `json:"name"`
	Completed       *bool  `json:"completed"`
	Text            string `json:"text"`
	ResourceSubtype string `json:"resource_subtype"`
	Host            string `json:"host"`
	CreatedBy       *struct {
		Name string `json:"name"`
	} `json:"created_by"`
}

func parseResource(raw json.RawMessage) resource {
	var r resource
	_ = json.Unmarshal(raw, &r)
	return r
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func summarizeUser(raw json.RawMessage) string {
	r := parseResource(raw)
	name := r.Name
	if name == "" {
		name = r.GID
	}
	if name == "" {
		name = "unknown"
	}
	return "Asana user: " + name
}

func summarizeWorkspace(raw json.RawMessage) string {
	r := parseResource(raw)
	name := r.Name
	if name == "" {
		name = "(unnamed workspace)"
	}
	return fmt.Sprintf("%s %s", orUnknown(r.GID), name)
}

func summarizeProject(raw json.RawMessage) string {
	r := parseResource(raw)
	name := r.Name
	if name == "" {
		name = "(unnamed project)"
	}
	return fmt.Sprintf("%s %s", orUnknown(r.GID), name)
}

func summarizeTask(raw json.RawMessage) string {
	r := parseResource(raw)
	name := r.Name
	if name == "" {
		name = "(unnamed task)"
	}
	state := "open"
	if r.Completed != nil && *r.Completed {
		state = "completed"
	}
	return fmt.Sprintf("%s %s [%s]", orUnknown(r.GID), name, state)
}

func summarizeStory(raw json.RawMessage) string {
	r := parseResource(raw)
	author := ""
	if r.CreatedBy != nil && r.CreatedBy.Name != "" {
		author = r.CreatedBy.Name + ": "
	}
	body := r.Text
	if body == "" {
		body = r.ResourceSubtype
	}
	if body == "" {
		body = "(story)"
	}
	return fmt.Sprintf("%s %s%s", orUnknown(r.GID), author, body)
}

func summarizeAttachment(raw json.RawMessage) string {
	r := parseResource(raw)
	name := r.Name
	if name == "" {
		name = "(unnamed attachment)"
	}
	kind := r.Host
	if kind == "" {
		kind = r.ResourceSubtype
	}
	if kind == "" {
		return fmt.Sprintf("%s %s", orUnknown(r.GID), name)
	}
	return fmt.Sprintf("%s %s [%s]", orUnknown(r.GID), name, kind)
}

// humanList joins per-item summaries, or returns emptyMsg when there are none.
func humanList(items []json.RawMessage, summarize func(json.RawMessage) string, emptyMsg string) string {
	if len(items) == 0 {
		return emptyMsg
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, summarize(it))
	}
	out := lines[0]
	for _, l := range lines[1:] {
		out += "\n" + l
	}
	return out
}
