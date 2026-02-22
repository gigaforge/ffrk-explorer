package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ---------- data types ----------

type Character struct {
	Realm   string
	Name    string
	Img     string
	ID      string
	Schools map[string]int // school name -> max rarity (1-6)
}

type SoulBreak struct {
	Character string
	Img       string
	Name      string
	Tier      string
	SBVer     string
	Type      string
	Element   string
	Time      string
	Effects   string
	ID        string
}

type HeroAbility struct {
	Character string
	Img       string
	Name      string
	HAVer     string
	Type      string
	Element   string
	Time      string
	Effects   string
	School    string
	ID        string
}

type RealmGroup struct {
	Realm      string
	Characters []Character
}

type CharDetail struct {
	Character     Character
	SoulBreaks    []SoulBreak
	HeroAbilities []HeroAbility
}

// ---------- globals ----------

var (
	characters    []Character
	soulBreaks    map[string][]SoulBreak  // keyed by character name
	heroAbilities map[string][]HeroAbility // keyed by character name
	realmGroups   []RealmGroup
	charByID      map[string]*Character
)

// ---------- CSV loading ----------

func mustReadCSV(path string) []map[string]string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	if len(records) < 2 {
		return nil
	}
	header := records[0]
	var rows []map[string]string
	for _, rec := range records[1:] {
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

var schoolNames = []string{
	"Black Magic", "White Magic", "Combat", "Support", "Celerity",
	"Summoning", "Spellblade", "Dragoon", "Monk", "Thief",
	"Knight", "Samurai", "Ninja", "Bard", "Dancer",
	"Machinist", "Darkness", "Sharpshooter", "Witch", "Heavy",
}

func loadCharacters() {
	rows := mustReadCSV("Characters.csv")
	for _, row := range rows {
		c := Character{
			Realm:   row["Realm"],
			Name:    row["Name"],
			Img:     row["Img"],
			ID:      row["ID"],
			Schools: make(map[string]int),
		}
		for _, s := range schoolNames {
			v := strings.TrimSpace(row[s])
			if v != "" {
				n, err := strconv.Atoi(v)
				if err == nil && n > 0 {
					c.Schools[s] = n
				}
			}
		}
		characters = append(characters, c)
	}
}

func loadSoulBreaks() {
	soulBreaks = make(map[string][]SoulBreak)
	rows := mustReadCSV("Soul-Breaks.csv")
	for _, row := range rows {
		sb := SoulBreak{
			Character: row["Character"],
			Img:       row["Img"],
			Name:      row["Name"],
			Tier:      row["Tier"],
			SBVer:     row["SB Ver"],
			Type:      row["Type"],
			Element:   row["Element"],
			Time:      row["Time"],
			Effects:   row["Effects"],
			ID:        row["ID"],
		}
		soulBreaks[sb.Character] = append(soulBreaks[sb.Character], sb)
	}
}

func loadHeroAbilities() {
	heroAbilities = make(map[string][]HeroAbility)
	rows := mustReadCSV("Hero-Abilities.csv")
	for _, row := range rows {
		ha := HeroAbility{
			Character: row["Character"],
			Img:       row["Img"],
			Name:      row["Name"],
			HAVer:     row["HA Ver"],
			Type:      row["Type"],
			Element:   row["Element"],
			Time:      row["Time"],
			Effects:   row["Effects"],
			School:    row["School"],
			ID:        row["ID"],
		}
		heroAbilities[ha.Character] = append(heroAbilities[ha.Character], ha)
	}
}

// Preferred realm display order (roughly follows mainline game numbering)
var realmOrder = map[string]int{
	"I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6,
	"VII": 7, "VIII": 8, "IX": 9, "X": 10, "XI": 11, "XII": 12,
	"XIII": 13, "XIV": 14, "XV": 15, "XVI": 16,
	"FFT": 17, "Type-0": 18, "KH": 19, "Beyond": 20, "Core": 21, "DB Only": 22,
}

func buildRealmGroups() {
	grouped := make(map[string][]Character)
	for _, c := range characters {
		grouped[c.Realm] = append(grouped[c.Realm], c)
	}
	for realm, chars := range grouped {
		sort.Slice(chars, func(i, j int) bool {
			return chars[i].Name < chars[j].Name
		})
		grouped[realm] = chars
		realmGroups = append(realmGroups, RealmGroup{Realm: realm, Characters: chars})
	}
	sort.Slice(realmGroups, func(i, j int) bool {
		oi, oj := realmOrder[realmGroups[i].Realm], realmOrder[realmGroups[j].Realm]
		if oi != oj {
			return oi < oj
		}
		return realmGroups[i].Realm < realmGroups[j].Realm
	})
	charByID = make(map[string]*Character)
	for i := range characters {
		charByID[characters[i].ID] = &characters[i]
	}
}

// ---------- image caching ----------

// ensureCachedImage checks for a local copy at dir/<id>.png; if missing it
// downloads from remoteFmt (which should contain two %s placeholders for the ID).
// Returns the URL path to serve the image, or "" on failure.
func ensureCachedImage(dir, urlPath, remoteFmt, id string) string {
	localPath := filepath.Join(dir, id+".png")
	servePath := "/" + urlPath + "/" + id + ".png"
	if _, err := os.Stat(localPath); err == nil {
		return servePath
	}
	url := fmt.Sprintf(remoteFmt, id, id)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("fetch image %s: %v", id, err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("fetch image %s: status %d", id, resp.StatusCode)
		return ""
	}
	f, err := os.Create(localPath)
	if err != nil {
		log.Printf("create image file %s: %v", localPath, err)
		return ""
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Printf("write image %s: %v", localPath, err)
		return ""
	}
	return servePath
}

func cacheAllImages() {
	dirs := []string{"images/characters", "images/abilities", "images/hero_abilities", "images/soulbreaks"}
	for _, d := range dirs {
		os.MkdirAll(d, 0o755)
	}

	const (
		charFmt    = "https://dff.sp.mbga.jp/dff/static/lang/image/buddy/%s/%s.png"
		abilityFmt = "https://dff.sp.mbga.jp/dff/static/lang/image/ability/%s/%s_256.png"
		sbFmt      = "https://dff.sp.mbga.jp/dff/static/lang/image/soulstrike/%s/%s_256.png"
	)

	for i := range characters {
		if img := ensureCachedImage("images/characters", "images/characters", charFmt, characters[i].ID); img != "" {
			characters[i].Img = img
		}
	}

	for name, haList := range heroAbilities {
		for i := range haList {
			if haList[i].ID != "" {
				if img := ensureCachedImage("images/hero_abilities", "images/hero_abilities", abilityFmt, haList[i].ID); img != "" {
					haList[i].Img = img
				}
			}
		}
		heroAbilities[name] = haList
	}

	for name, sbList := range soulBreaks {
		for i := range sbList {
			if sbList[i].ID != "" {
				if img := ensureCachedImage("images/soulbreaks", "images/soulbreaks", sbFmt, sbList[i].ID); img != "" {
					sbList[i].Img = img
				}
			}
		}
		soulBreaks[name] = sbList
	}
}

// ---------- templates ----------

var funcMap = template.FuncMap{
	"stars": func(n int) string {
		return strings.Repeat("★", n)
	},
}

var indexTmpl = template.Must(template.New("index").Funcs(funcMap).Parse(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>FFRK Community Database</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
       background: #1a1a2e; color: #e0e0e0; padding: 20px; }
h1 { text-align: center; color: #e94560; margin-bottom: 24px; }
h2 { color: #0f3460; background: #e94560; display: inline-block;
     padding: 4px 16px; border-radius: 4px; margin: 20px 0 12px; font-size: 1.1em; }
.realm-section { margin-bottom: 16px; }
.char-grid { display: flex; flex-wrap: wrap; gap: 8px; }
.char-card { background: #16213e; border: 1px solid #0f3460; border-radius: 6px;
             padding: 8px; width: 90px; text-align: center; cursor: pointer;
             transition: transform 0.1s, border-color 0.1s; text-decoration: none; color: #e0e0e0; }
.char-card:hover { transform: scale(1.05); border-color: #e94560; }
.char-card img { width: 64px; height: 64px; object-fit: contain; display: block; margin: 0 auto 4px; }
.char-card .placeholder { width: 64px; height: 64px; background: #0f3460; border-radius: 50%;
                          display: flex; align-items: center; justify-content: center;
                          margin: 0 auto 4px; font-size: 24px; color: #e94560; }
.char-card .name { font-size: 11px; line-height: 1.2; word-wrap: break-word; }
</style>
</head><body>
<h1>FFRK Community Database</h1>
{{range .}}
<div class="realm-section">
  <h2>{{.Realm}}</h2>
  <div class="char-grid">
    {{range .Characters}}
    <a class="char-card" href="/char/{{.ID}}">
      {{if .Img}}<img src="{{.Img}}" alt="{{.Name}}">
      {{else}}<div class="placeholder">{{slice .Name 0 1}}</div>{{end}}
      <div class="name">{{.Name}}</div>
    </a>
    {{end}}
  </div>
</div>
{{end}}
</body></html>`))

var charTmpl = template.Must(template.New("char").Funcs(funcMap).Parse(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>{{.Character.Name}} - FFRK</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
       background: #1a1a2e; color: #e0e0e0; padding: 20px; max-width: 1000px; margin: 0 auto; }
a { color: #e94560; }
h1 { color: #e94560; margin-bottom: 4px; }
.subtitle { color: #888; margin-bottom: 20px; }
.back { display: inline-block; margin-bottom: 16px; }
h2 { color: #0f3460; background: #e94560; display: inline-block;
     padding: 4px 16px; border-radius: 4px; margin: 20px 0 12px; font-size: 1em; }
.school-grid { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.school-badge { background: #16213e; border: 1px solid #0f3460; border-radius: 4px;
                padding: 4px 10px; font-size: 13px; }
.school-badge .stars { color: #e9c46a; }
table { width: 100%; border-collapse: collapse; margin-bottom: 8px; font-size: 13px; }
th { background: #0f3460; padding: 6px 8px; text-align: left; }
td { background: #16213e; padding: 6px 8px; border-bottom: 1px solid #1a1a2e; }
td img { width: 32px; height: 32px; object-fit: contain; vertical-align: middle; }
.effects { max-width: 500px; }
</style>
</head><body>
<a class="back" href="/">← All Characters</a>
<h1>{{.Character.Name}}</h1>
<div class="subtitle">{{.Character.Realm}}</div>

<h2>Ability Schools</h2>
<div class="school-grid">
  {{range $school, $rating := .Character.Schools}}
  <div class="school-badge">{{$school}} <span class="stars">{{stars $rating}}</span></div>
  {{end}}
</div>

{{if .HeroAbilities}}
<h2>Hero Abilities</h2>
<table>
<tr><th></th><th>Name</th><th>School</th><th>Type</th><th>Element</th><th>Effects</th></tr>
{{range .HeroAbilities}}
<tr>
  <td>{{if .Img}}<img src="{{.Img}}">{{end}}</td>
  <td>{{.Name}} ({{.HAVer}})</td>
  <td>{{.School}}</td>
  <td>{{.Type}}</td>
  <td>{{.Element}}</td>
  <td class="effects">{{.Effects}}</td>
</tr>
{{end}}
</table>
{{end}}

{{if .SoulBreaks}}
<h2>Soul Breaks ({{len .SoulBreaks}})</h2>
<table>
<tr><th></th><th>Name</th><th>Tier</th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
{{range .SoulBreaks}}
<tr>
  <td>{{if .Img}}<img src="{{.Img}}">{{end}}</td>
  <td>{{.Name}}</td>
  <td>{{.Tier}}</td>
  <td>{{.Type}}</td>
  <td>{{.Element}}</td>
  <td>{{.Time}}</td>
  <td class="effects">{{.Effects}}</td>
</tr>
{{end}}
</table>
{{end}}


</body></html>`))

// ---------- handlers ----------

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	indexTmpl.Execute(w, realmGroups)
}

func charHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/char/")
	ch, ok := charByID[id]
	if !ok {
		http.NotFound(w, r)
		return
	}

	detail := CharDetail{
		Character:     *ch,
		SoulBreaks:    soulBreaks[ch.Name],
		HeroAbilities: heroAbilities[ch.Name],
	}
	charTmpl.Execute(w, detail)
}

// ---------- main ----------

func main() {
	log.Println("Loading data...")
	loadCharacters()
	loadSoulBreaks()
	loadHeroAbilities()

	log.Println("Caching images...")
	cacheAllImages()

	buildRealmGroups()
	log.Printf("Loaded %d characters", len(characters))

	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/char/", charHandler)

	addr := "0.0.0.0:9090"
	fmt.Printf("Server running at http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
