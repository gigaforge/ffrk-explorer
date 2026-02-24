package main

import (
	"embed"
	"encoding/json"
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
	dataLock.RLock()
	defer dataLock.RUnlock()
	indexTmpl.Execute(w, realmGroups)
}

func characterAPIHandler(w http.ResponseWriter, r *http.Request) {
	dataLock.RLock()
	defer dataLock.RUnlock()
	names := make([]string, len(characters))
	for i, c := range characters {
		names[i] = c.Name
	}
	sort.Strings(names)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

func tierAPIHandler(w http.ResponseWriter, r *http.Request) {
	dataLock.RLock()
	defer dataLock.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tierNames)
}
