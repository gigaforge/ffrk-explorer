package main

import (
	"bytes"
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

	ids := parsePartyIDs(r.URL.Query().Get("ids"))
	hiddenSBs := makeHiddenSBMap(parseHiddenSBIDs(r.URL.Query().Get("hidden")))
	data := buildPartyData(d, r, ids, hiddenSBs)
	partyTmpl.Execute(w, data)
}

func parseCSVIDs(raw string, limit int) []string {
	if raw == "" {
		return nil
	}
	var ids []string
	seen := make(map[string]bool)
	for _, id := range strings.Split(raw, ",") {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	return ids
}

func parsePartyIDs(raw string) []string {
	return parseCSVIDs(raw, 5)
}

func parseHiddenSBIDs(raw string) []string {
	return parseCSVIDs(raw, 0)
}

func makeHiddenSBMap(ids []string) map[string]bool {
	hiddenSBs := make(map[string]bool, len(ids))
	for _, id := range ids {
		hiddenSBs[id] = true
	}
	return hiddenSBs
}

func hiddenSBList(hiddenSBs map[string]bool) []string {
	ids := make([]string, 0, len(hiddenSBs))
	for id := range hiddenSBs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func buildPartyData(d *AppData, r *http.Request, ids []string, hiddenSBs map[string]bool) PartyData {
	var members []PartyMember
	for _, id := range ids {
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

	sorted := make([]Character, len(d.Characters))
	copy(sorted, d.Characters)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	data := PartyData{
		Members:       members,
		AllCharacters: sorted,
		HiddenSBs:     hiddenSBs,
	}

	u := getCurrentUser(r)
	if u != nil {
		data.LoggedIn = true
		data.OwnedSoulbreaks = snapshotOwnedSoulbreaks(u.ID)
	} else {
		data.OwnedSoulbreaks = make(map[string]bool)
	}

	if len(members) > 0 {
		data.EffectSummary = buildEffectSummary(members, data.LoggedIn, data.OwnedSoulbreaks, hiddenSBs)
	}

	return data
}

func renderPartyHTML(data PartyData) (string, error) {
	var buf bytes.Buffer
	if err := partyTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func partyVisibilityHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	d, ok := requireAppData(w)
	if !ok {
		return
	}

	var req struct {
		CharacterIDs []string `json:"character_ids"`
		HiddenSBs    []string `json:"hidden_sbs"`
		SoulbreakID  string   `json:"soulbreak_id"`
		Hidden       bool     `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.SoulbreakID == "" {
		jsonError(w, http.StatusBadRequest, "soulbreak_id required")
		return
	}

	req.CharacterIDs = parsePartyIDs(strings.Join(req.CharacterIDs, ","))
	req.HiddenSBs = parseHiddenSBIDs(strings.Join(req.HiddenSBs, ","))

	hiddenSBs := makeHiddenSBMap(req.HiddenSBs)
	if req.Hidden {
		hiddenSBs[req.SoulbreakID] = true
	} else {
		delete(hiddenSBs, req.SoulbreakID)
	}

	data := buildPartyData(d, r, req.CharacterIDs, hiddenSBs)
	html, err := renderPartyHTML(data)
	if err != nil {
		http.Error(w, "could not render party", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"soulbreak_id": req.SoulbreakID,
		"hidden":       req.Hidden,
		"hidden_sbs":   hiddenSBList(hiddenSBs),
		"html":         html,
	})
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
	{"bard_6", "6★ Bard Access"},
	{"dancer_6", "6★ Dancer Access"},
	{"support_6", "6★ Support Access"},
}

var schoolAccessEffects = map[string]string{
	"bard_6":    "Bard",
	"dancer_6":  "Dancer",
	"support_6": "Support",
}

func buildEffectSummary(members []PartyMember, loggedIn bool, owned map[string]bool, hiddenSBs map[string]bool) []EffectSummary {
	var summary []EffectSummary
	for _, eff := range summaryEffects {
		count := 0
		var sources []EffectSource

		if school, ok := schoolAccessEffects[eff.Key]; ok {
			for _, m := range members {
				if m.Character.Schools[school] >= 6 {
					count++
					sources = append(sources, EffectSource{
						Character: m.Character.Name,
					})
				}
			}
		} else {
			for _, m := range members {
				for _, sb := range m.SoulBreaks {
					if loggedIn && !owned[sb.ID] {
						continue
					}
					if hiddenSBs[sb.ID] {
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
		}

		summary = append(summary, EffectSummary{
			Key:       eff.Key,
			Label:     eff.Label,
			Count:     count,
			Sources:   sources,
			Separator: eff.Key == "bard_6",
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
