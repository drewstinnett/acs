package web

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

var tmpl *template.Template

func init() {
	funcMap := template.FuncMap{
		"inc": func(i int) int { return i + 1 },
		"snippetHTML": func(s string) template.HTML {
			// The snippet from SQLite FTS5 uses <mark> tags; render them safely.
			s = template.HTMLEscapeString(s)
			s = strings.ReplaceAll(s, "&lt;mark&gt;", "<mark>")
			s = strings.ReplaceAll(s, "&lt;/mark&gt;", "</mark>")
			return template.HTML(s)
		},
	}

	var err error
	tmpl, err = template.New("").Funcs(funcMap).ParseFS(templateFS,
		"templates/base.html",
		"templates/index.html",
		"templates/results.html",
		"templates/results_inner.html",
		"templates/items.html",
		"templates/item.html",
		"templates/page.html",
	)
	if err != nil {
		panic(fmt.Sprintf("parse templates: %v", err))
	}
}

func renderTemplate(name string, data any) ([]byte, error) {
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
