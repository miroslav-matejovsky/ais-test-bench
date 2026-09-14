package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

// renderer executes named templates from a shared template set extended with
// page files. The shared set is parsed once; page files are parsed per call.
type renderer struct {
	templateFS fs.FS
	shared     *template.Template
}

// newRenderer parses sharedFiles (glob patterns) from templateFS into the
// shared template set.
func newRenderer(templateFS fs.FS, sharedFiles ...string) (*renderer, error) {
	shared, err := template.New("").ParseFS(templateFS, sharedFiles...)
	if err != nil {
		return nil, fmt.Errorf("parse shared templates %v: %w", sharedFiles, err)
	}
	return &renderer{templateFS: templateFS, shared: shared}, nil
}

// render clones the shared set, parses pageFiles into the clone, and executes
// the template called name with data. Output is buffered, so an execution
// error writes nothing and the caller can still send an error response.
func (r *renderer) render(w http.ResponseWriter, status int, data any, name string, pageFiles ...string) error {
	ts, err := r.shared.Clone()
	if err != nil {
		return fmt.Errorf("clone shared templates: %w", err)
	}
	if len(pageFiles) > 0 {
		ts, err = ts.ParseFS(r.templateFS, pageFiles...)
		if err != nil {
			return fmt.Errorf("parse page templates %v: %w", pageFiles, err)
		}
	}

	var buf bytes.Buffer
	if err := ts.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request-Type")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}
