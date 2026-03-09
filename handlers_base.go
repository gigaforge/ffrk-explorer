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
var partyTmpl = mustParseTemplate("party", "web/party.html")

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

func partyHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
		return
	}

	idsParam := r.URL.Query().Get("ids")
	var members []PartyMember
	if idsParam != "" {
		for _, id := range strings.SplitN(idsParam, ",", 5) {
			id = strings.TrimSpace(id)
			ch, ok := d.CharByID[id]
			if !ok {
				continue
			}
			members = append(members, PartyMember{
				Character:     *ch,
				HeroAbilities: d.HeroAbilities[ch.Name],
				LegendMateria: d.LegendMateria[ch.Name],
				SoulBreaks:    d.SoulBreaks[ch.Name],
			})
		}
	}

	sorted := make([]Character, len(d.Characters))
	copy(sorted, d.Characters)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	data := PartyData{Members: members, AllCharacters: sorted}
	u := getCurrentUser(r)
	if u != nil {
		data.LoggedIn = true
		data.OwnedSoulbreaks = snapshotOwnedSoulbreaks(u.ID)
	} else {
		data.OwnedSoulbreaks = make(map[string]bool)
	}

	partyTmpl.Execute(w, data)
}

func tierAPIHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, d.TierNames)
}
