package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// BaseData is embedded in every page payload.
type BaseData struct {
	Title     string
	User      string
	CSRF      string
	Version   string
	Path      string
	Flash     string
	FlashKind string
	Docker    bool
	Systemd   bool
	CF        bool
	OpenClaw  bool
	ReadOnly  bool
}

// Renderer holds one parsed template set per page.
type Renderer struct {
	pages map[string]*template.Template
}

var templateFuncs = template.FuncMap{
	"shortID": func(s string) string {
		if len(s) > 12 {
			return s[:12]
		}
		return s
	},
	"ago": func(v any) string {
		t, ok := v.(time.Time)
		if !ok || t.IsZero() {
			return "-"
		}
		d := time.Since(t)
		if d < time.Minute {
			return "just now"
		}
		if d < time.Hour {
			return fmt.Sprintf("%dm ago", int(d.Minutes()))
		}
		if d < 24*time.Hour {
			return fmt.Sprintf("%dh ago", int(d.Hours()))
		}
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	},
	"truncate": func(n int, s string) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "…"
	},
	"hasPrefix": func(s, prefix string) bool { return strings.HasPrefix(s, prefix) },
	"stateClass": func(s string) string {
		switch strings.ToLower(s) {
		case "running", "active", "up", "ok":
			return "ok"
		case "exited", "dead", "failed", "inactive":
			return "bad"
		default:
			return "warn"
		}
	},
}

// NewRenderer parses base.html plus every page file into isolated template sets.
func NewRenderer() (*Renderer, error) {
	entries, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	pages := make(map[string]*template.Template)
	for _, path := range entries {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "templates/"), ".html")
		if name == "base" {
			continue
		}
		t, err := template.New(name).Funcs(templateFuncs).ParseFS(templateFS, "templates/base.html", path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		pages[name] = t
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no page templates found")
	}
	return &Renderer{pages: pages}, nil
}

// Render writes a page using base.html as the layout.
func (r *Renderer) Render(w http.ResponseWriter, status int, page string, data any) {
	t, ok := r.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		// Headers are already sent; log via caller-provided writer is not possible here.
		fmt.Printf("template render error page=%s: %v\n", page, err)
	}
}

// StaticHandler serves the embedded assets.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
