package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

type breadcrumbItem struct {
	Label string
	Href  string
}

type basePage struct {
	Title      string
	Host       string
	Breadcrumb []breadcrumbItem
	LogoutHref string
}

type settingsIndexItem struct {
	Title       string
	Description string
	Status      string
	Tone        string
	Href        string
}

type settingsIndexData struct {
	basePage
	Items []settingsIndexItem
}

type loginPageData struct {
	basePage
	Username     string
	Next         string
	ErrorMessage string
}

type githubEmptyData struct {
	basePage
	ErrorMessage string
}

type githubConfiguredData struct {
	basePage
	AppName      string
	AppSlug      string
	AppID        int64
	ErrorField   string
	ErrorMessage string
}

type githubErrorData struct {
	basePage
	Title       string
	Description string
	HTTPStatus  string
	ErrorCode   string
}

type githubRedirectData struct {
	basePage
	GitHubURL    template.URL
	ManifestJSON string
}

type templateRegistry struct {
	tmpls map[string]*template.Template
}

var pageTemplates = map[string]string{
	"login":             "templates/login.html",
	"settings_index":    "templates/settings_index.html",
	"github_empty":      "templates/github_empty.html",
	"github_configured": "templates/github_configured.html",
	"github_error":      "templates/github_error.html",
	"github_redirect":   "templates/github_redirect.html",
}

func loadTemplates() (*templateRegistry, error) {
	reg := &templateRegistry{tmpls: make(map[string]*template.Template)}
	for name, path := range pageTemplates {
		t, err := template.ParseFS(templatesFS, "templates/base.html", path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		reg.tmpls[name] = t
	}
	return reg, nil
}

func (r *templateRegistry) render(w http.ResponseWriter, status int, name string, data any) {
	t, ok := r.tmpls[name]
	if !ok {
		slog.Error("template not found", "name", name)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		slog.Error("template render failed", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Error("template write failed", "name", name, "error", err)
	}
}
