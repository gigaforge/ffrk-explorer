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

	if len(members) > 0 {
		data.EffectSummary = buildEffectSummary(members, data.LoggedIn, data.OwnedSoulbreaks)
	}

	partyTmpl.Execute(w, data)
}

var summaryEffects = []struct {
	Key   string
	Label string
}{
	{"aegis_counter", "Aegis Counter"},
	{"fullbreak_counter", "Fullbreak Counter"},
	{"phys_job_break_counter", "Phys Job Break Counter"},
	{"mag_job_break_counter", "Mag Job Break Counter"},
	{"proshellga", "Proshellga"},
	{"last_stand", "Last Stand"},
	{"regenga", "Regenga"},
	{"astra", "Astra"},
	{"party_crit_chance", "Party Crit Chance"},
	{"party_crit_damage", "Party Crit Damage"},
	{"weakness_boost", "Weakness Boost"},
	{"magical_boost", "Magical Boost"},
	{"phy_boost", "PHY Boost"},
	{"sorcery_boost", "Sorcery Damage Boost"},
	{"pentabreak_boost", "Pentabreak Damage Boost"},
}

func buildEffectSummary(members []PartyMember, loggedIn bool, owned map[string]bool) []EffectSummary {
	var summary []EffectSummary
	for _, eff := range summaryEffects {
		count := 0
		var sources []EffectSource
		for _, m := range members {
			for _, sb := range m.SoulBreaks {
				if loggedIn && !owned[sb.ID] {
					continue
				}
				var found bool
				if eff.Key == "proshellga" {
					found = sbMatchesProshellga(sb)
				} else {
					checker := effectCheckers[eff.Key]
					if checker == nil {
						continue
					}
					found = walkSBTexts(sb, sbTextWalkOptions{
						IncludeStatuses: true,
						IncludeBrave:    true,
					}, checker)
				}
				if found {
					count++
					sources = append(sources, EffectSource{
						Character: m.Character.Name,
						SoulBreak: sb.Name,
					})
				}
			}
		}
		summary = append(summary, EffectSummary{
			Key:     eff.Key,
			Label:   eff.Label,
			Count:   count,
			Sources: sources,
		})
	}
	return summary
}

func tierAPIHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, d.TierNames)
}
