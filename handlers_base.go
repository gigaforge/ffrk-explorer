package main

import (
	"embed"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

// ---------- templates ----------

var funcMap = template.FuncMap{
	"stars": func(n int) string {
		return strings.Repeat("★", n)
	},
}

//go:embed web/*.html web/partials/*.html
var webTemplatesFS embed.FS

func mustParseTemplate(name, path string) *template.Template {
	return template.Must(template.New(filepath.Base(path)).Funcs(funcMap).ParseFS(webTemplatesFS, path, "web/partials/*.html"))
}

var indexTmpl = mustParseTemplate("index", "web/index.html")
var charTmpl = mustParseTemplate("char", "web/char.html")
var searchTmpl = mustParseTemplate("search", "web/search.html")

// ---------- handlers ----------

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	d, ok := requireAppData(w)
	if !ok {
		return
	}
	indexTmpl.Execute(w, d.RealmGroups)
}

func characterAPIHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
		return
	}
	names := make([]string, len(d.Characters))
	for i, c := range d.Characters {
		names[i] = c.Name
	}
	sort.Strings(names)
	writeJSON(w, http.StatusOK, names)
}

func tierAPIHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, d.TierNames)
}
