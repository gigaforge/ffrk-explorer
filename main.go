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
	"regexp"
	"sort"
	"sync"
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

type StatusEffect struct {
	Name        string
	Description string
	Duration    string
}

type BurstCommand struct {
	Img            string
	Name           string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	MatchedEffects []StatusEffect
}

type SynchroAbility struct {
	Img            string
	Name           string
	Slot           string
	Condition      string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	ConditionID    string
	MatchedEffects []StatusEffect
}

type ZenithAbility struct {
	Img            string
	Name           string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	MatchedEffects []StatusEffect
}

type BraveLevel struct {
	Level          string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	MatchedEffects []StatusEffect
}

type BraveCommand struct {
	Name      string
	School    string
	Condition string
	Levels    []BraveLevel // 0-3
}

type SoulBreak struct {
	Character        string
	Img              string
	Name             string
	Tier             string
	SBVer            string
	Type             string
	Element          string
	Time             string
	Effects          string
	ID               string
	MatchedEffects   []StatusEffect
	BurstCommands    []BurstCommand
	SynchroAbilities []SynchroAbility
	ZenithAbilities  []ZenithAbility
	DualShift        *SoulBreak
	BraveCommand     *BraveCommand
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
	burstCommands    map[string][]BurstCommand    // keyed by "Character|Source"
	synchroAbilities map[string][]SynchroAbility  // keyed by "Character|Source"
	zenithAbilities  map[string][]ZenithAbility   // keyed by "Character|Source"
	braveCommands    map[string]*BraveCommand     // keyed by "Character|Source"
	statusEffects map[string]StatusEffect   // keyed by Common Name
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

func loadBurstCommands() {
	burstCommands = make(map[string][]BurstCommand)
	rows := mustReadCSV("Burst.csv")
	for _, row := range rows {
		bc := BurstCommand{
			Name:    row["Name"],
			Type:    row["Type"],
			Target:  row["Target"],
			Element: row["Element"],
			Time:    row["Time"],
			Effects: row["Effects"],
			School:  row["School"],
			ID:      row["ID"],
		}
		key := row["Character"] + "|" + row["Source"]
		burstCommands[key] = append(burstCommands[key], bc)
	}
	// Sort each group so ID ending in '1' comes before '2'
	for key, cmds := range burstCommands {
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].ID < cmds[j].ID
		})
		burstCommands[key] = cmds
	}
	log.Printf("Loaded %d burst command groups", len(burstCommands))
}

func matchBurstCommands() {
	for name, sbList := range soulBreaks {
		for i := range sbList {
			if sbList[i].Tier != "BSB" {
				continue
			}
			key := sbList[i].Character + "|" + sbList[i].Name
			if cmds, ok := burstCommands[key]; ok {
				// Deep copy and match status effects for each command
				matched := make([]BurstCommand, len(cmds))
				copy(matched, cmds)
				for j := range matched {
					matched[j].MatchedEffects = matchEffectsInText(matched[j].Effects)
				}
				sbList[i].BurstCommands = matched
			}
		}
		soulBreaks[name] = sbList
	}
}

func loadSynchroAbilities() {
	synchroAbilities = make(map[string][]SynchroAbility)
	rows := mustReadCSV("Synchro.csv")
	for _, row := range rows {
		sa := SynchroAbility{
			Name:      row["Name"],
			Slot:      row["Synchro Ability Slot"],
			Condition: row["Synchro Condition"],
			Type:      row["Type"],
			Target:    row["Target"],
			Element:   row["Element"],
			Time:      row["Time"],
			Effects:   row["Effects"],
			School:      row["School"],
			ID:          row["ID"],
			ConditionID: row["Synchro Condition ID"],
		}
		key := row["Character"] + "|" + row["Source"]
		synchroAbilities[key] = append(synchroAbilities[key], sa)
	}
	for key, abs := range synchroAbilities {
		sort.Slice(abs, func(i, j int) bool {
			return abs[i].Slot < abs[j].Slot
		})
		synchroAbilities[key] = abs
	}
	log.Printf("Loaded %d synchro ability groups", len(synchroAbilities))
}

func matchSynchroAbilities() {
	for name, sbList := range soulBreaks {
		for i := range sbList {
			if sbList[i].Tier != "SASB" {
				continue
			}
			key := sbList[i].Character + "|" + sbList[i].Name
			if abs, ok := synchroAbilities[key]; ok {
				matched := make([]SynchroAbility, len(abs))
				copy(matched, abs)
				for j := range matched {
					matched[j].MatchedEffects = matchEffectsInText(matched[j].Effects)
				}
				sbList[i].SynchroAbilities = matched
			}
		}
		soulBreaks[name] = sbList
	}
}

func loadZenithAbilities() {
	zenithAbilities = make(map[string][]ZenithAbility)
	rows := mustReadCSV("Zenith-SB-Abilities.csv")
	for _, row := range rows {
		za := ZenithAbility{
			Name:    row["Name"],
			Type:    row["Type"],
			Target:  row["Target"],
			Element: row["Element"],
			Time:    row["Time"],
			Effects: row["Effects"],
			School:  row["School"],
			ID:      row["ID"],
		}
		key := row["Character"] + "|" + row["Source"]
		zenithAbilities[key] = append(zenithAbilities[key], za)
	}
	log.Printf("Loaded %d zenith ability groups", len(zenithAbilities))
}

func matchZenithAbilities() {
	for name, sbList := range soulBreaks {
		for i := range sbList {
			if sbList[i].Tier != "ZSB" {
				continue
			}
			key := sbList[i].Character + "|" + sbList[i].Name
			if abs, ok := zenithAbilities[key]; ok {
				matched := make([]ZenithAbility, len(abs))
				copy(matched, abs)
				for j := range matched {
					matched[j].MatchedEffects = matchEffectsInText(matched[j].Effects)
				}
				sbList[i].ZenithAbilities = matched
			}
		}
		soulBreaks[name] = sbList
	}
}

func loadBraveCommands() {
	braveCommands = make(map[string]*BraveCommand)
	rows := mustReadCSV("Brave.csv")
	for _, row := range rows {
		key := row["Character"] + "|" + row["Source"]
		bc, ok := braveCommands[key]
		if !ok {
			bc = &BraveCommand{
				Name:      row["Name"],
				School:    row["School"],
				Condition: row["Brave Condition"],
			}
			braveCommands[key] = bc
		}
		bl := BraveLevel{
			Level:   row["Brave"],
			Type:    row["Type"],
			Target:  row["Target"],
			Element: row["Element"],
			Time:    row["Time"],
			Effects: row["Effects"],
		}
		bc.Levels = append(bc.Levels, bl)
	}
	// Sort levels by Brave value (0-3)
	for _, bc := range braveCommands {
		sort.Slice(bc.Levels, func(i, j int) bool {
			return bc.Levels[i].Level < bc.Levels[j].Level
		})
	}
	log.Printf("Loaded %d brave commands", len(braveCommands))
}

func matchBraveCommands() {
	for name, sbList := range soulBreaks {
		for i := range sbList {
			if !strings.Contains(sbList[i].Effects, "[Brave Mode]") {
				continue
			}
			key := sbList[i].Character + "|" + sbList[i].Name
			if bc, ok := braveCommands[key]; ok {
				matched := *bc
				matched.Levels = make([]BraveLevel, len(bc.Levels))
				copy(matched.Levels, bc.Levels)
				for j := range matched.Levels {
					matched.Levels[j].MatchedEffects = matchEffectsInText(matched.Levels[j].Effects)
				}
				sbList[i].BraveCommand = &matched
			}
		}
		soulBreaks[name] = sbList
	}
}

func loadStatuses() {
	statusEffects = make(map[string]StatusEffect)
	rows := mustReadCSV("Status.csv")
	for _, row := range rows {
		name := strings.TrimSpace(row["Common Name"])
		if name == "" {
			continue
		}
		statusEffects[name] = StatusEffect{
			Name:        name,
			Description: strings.TrimSpace(row["Effects"]),
			Duration:    strings.TrimSpace(row["Default Duration"]),
		}
	}
	log.Printf("Loaded %d status effects", len(statusEffects))
}

var bracketRe = regexp.MustCompile(`\[([^\]]+)\]`)
var pctRe = regexp.MustCompile(`([+-])\d+%`)
var forSecRe = regexp.MustCompile(`for (\d+(?:\.\d+)?) seconds`)

func lookupStatus(term string) (StatusEffect, bool) {
	// Try exact match first
	if se, ok := statusEffects[term]; ok {
		return se, true
	}
	// Fallback: replace specific percentages (+30%, -50%) with +X%/-X%
	generic := pctRe.ReplaceAllString(term, "${1}X%")
	if generic != term {
		if se, ok := statusEffects[generic]; ok {
			// Substitute actual percentages into the description
			// Build a list of the actual values from the term
			actuals := pctRe.FindAllString(term, -1)
			result := se
			result.Name = term
			result.Description = se.Description
			for _, actual := range actuals {
				result.Description = strings.Replace(result.Description, string(actual[0])+"X%", actual, 1)
			}
			return result, true
		}
	}
	return StatusEffect{}, false
}

// matchEffectsInText extracts bracketed status effects from text, inferring
// duration from the next "for N seconds" that follows each bracket when the
// status has no default duration.
func matchEffectsInText(text string) []StatusEffect {
	bracketLocs := bracketRe.FindAllStringSubmatchIndex(text, -1)
	durLocs := forSecRe.FindAllStringSubmatchIndex(text, -1)

	var results []StatusEffect
	for _, bloc := range bracketLocs {
		term := text[bloc[2]:bloc[3]]
		se, ok := lookupStatus(term)
		if !ok {
			continue
		}
		if se.Duration == "" || se.Duration == "-" {
			// Find the first "for N seconds" whose start is after this bracket's end
			bracketEnd := bloc[1]
			for _, dloc := range durLocs {
				if dloc[0] >= bracketEnd {
					se.Duration = text[dloc[2]:dloc[3]] + " seconds"
					break
				}
			}
		}
		results = append(results, se)
	}
	return results
}

func matchSoulBreakEffects() {
	for name, sbList := range soulBreaks {
		for i := range sbList {
			sbList[i].MatchedEffects = matchEffectsInText(sbList[i].Effects)
		}
		soulBreaks[name] = sbList
	}
}

func pairDualShifts() {
	for name, sbList := range soulBreaks {
		// Index primary DASBs by "SBVer" for this character
		primaries := make(map[string]int) // SBVer -> index in sbList
		for i, sb := range sbList {
			if sb.Tier == "DASB" && !strings.HasSuffix(sb.Name, "(Dual Shift)") {
				primaries[sb.SBVer] = i
			}
		}
		// Match Dual Shift entries to their primaries
		remove := make(map[int]bool)
		for i, sb := range sbList {
			if sb.Tier == "DASB" && strings.HasSuffix(sb.Name, "(Dual Shift)") {
				if pi, ok := primaries[sb.SBVer]; ok {
					ds := sb
					sbList[pi].DualShift = &ds
					remove[i] = true
				}
			}
		}
		if len(remove) > 0 {
			var filtered []SoulBreak
			for i, sb := range sbList {
				if !remove[i] {
					filtered = append(filtered, sb)
				}
			}
			soulBreaks[name] = filtered
		}
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
// downloads from remoteFmt (which should contain one or more %s placeholders for the ID).
// Returns the URL path to serve the image, or "" on failure.
func ensureCachedImage(dir, urlPath, remoteFmt, id string) string {
	localPath := filepath.Join(dir, id+".png")
	servePath := "/" + urlPath + "/" + id + ".png"
	if _, err := os.Stat(localPath); err == nil {
		return servePath
	}
	n := strings.Count(remoteFmt, "%s")
	args := make([]any, n)
	for i := range args {
		args[i] = id
	}
	url := fmt.Sprintf(remoteFmt, args...)
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

// ensureCachedFile downloads a fixed URL to localPath if not already present.
func ensureCachedFile(localPath, remoteURL string) {
	if _, err := os.Stat(localPath); err == nil {
		return
	}
	resp, err := http.Get(remoteURL)
	if err != nil {
		log.Printf("fetch %s: %v", remoteURL, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("fetch %s: status %d", remoteURL, resp.StatusCode)
		return
	}
	f, err := os.Create(localPath)
	if err != nil {
		log.Printf("create %s: %v", localPath, err)
		return
	}
	defer f.Close()
	io.Copy(f, resp.Body)
}

type imageJob struct {
	dir       string
	urlPath   string
	remoteFmt string
	id        string
	result    *string // pointer to the Img field to update
}

func cacheAllImages() {
	dirs := []string{"images/characters", "images/abilities", "images/hero_abilities", "images/soulbreaks", "images/burst", "images/synchro", "images/zenith", "images/brave"}
	for _, d := range dirs {
		os.MkdirAll(d, 0o755)
	}

	// Cache static brave images
	ensureCachedFile("images/brave/BraveBase.png", "https://dff.sp.mbga.jp/dff/static/lang/image/ability/30151001/30151001_128.png")
	for i := 0; i <= 3; i++ {
		ensureCachedFile(fmt.Sprintf("images/brave/BraveAttack%d.png", i),
			fmt.Sprintf("https://dff.sp.mbga.jp/dff/static/lang/image/brave/level/level_%d.png", i))
	}

	const (
		charFmt    = "https://dff.sp.mbga.jp/dff/static/lang/image/buddy/%s/%s.png"
		abilityFmt = "https://dff.sp.mbga.jp/dff/static/lang/image/ability/%s/%s_256.png"
		sbFmt      = "https://dff.sp.mbga.jp/dff/static/lang/image/soulstrike/%s/%s_256.png"
		burstFmt   = "https://dff.sp.mbga.jp/dff/static/lang/image/ability/%s/%s_128.png"
		synchroFmt = "https://dff.sp.mbga.jp/dff/static/lang/image/synchro/%s.png"
	)

	// Collect all jobs
	var jobs []imageJob

	for i := range characters {
		jobs = append(jobs, imageJob{"images/characters", "images/characters", charFmt, characters[i].ID, &characters[i].Img})
	}

	for name := range heroAbilities {
		haList := heroAbilities[name]
		for i := range haList {
			if haList[i].ID != "" {
				jobs = append(jobs, imageJob{"images/hero_abilities", "images/hero_abilities", abilityFmt, haList[i].ID, &haList[i].Img})
			}
		}
		heroAbilities[name] = haList
	}

	for name := range soulBreaks {
		sbList := soulBreaks[name]
		for i := range sbList {
			if sbList[i].ID != "" {
				jobs = append(jobs, imageJob{"images/soulbreaks", "images/soulbreaks", sbFmt, sbList[i].ID, &sbList[i].Img})
			}
			for j := range sbList[i].BurstCommands {
				if sbList[i].BurstCommands[j].ID != "" {
					jobs = append(jobs, imageJob{"images/burst", "images/burst", burstFmt, sbList[i].BurstCommands[j].ID, &sbList[i].BurstCommands[j].Img})
				}
			}
			for j := range sbList[i].SynchroAbilities {
				if sbList[i].SynchroAbilities[j].ConditionID != "" {
					jobs = append(jobs, imageJob{"images/synchro", "images/synchro", synchroFmt, sbList[i].SynchroAbilities[j].ConditionID, &sbList[i].SynchroAbilities[j].Img})
				}
			}
			for j := range sbList[i].ZenithAbilities {
				if sbList[i].ZenithAbilities[j].ID != "" {
					jobs = append(jobs, imageJob{"images/zenith", "images/zenith", burstFmt, sbList[i].ZenithAbilities[j].ID, &sbList[i].ZenithAbilities[j].Img})
				}
			}
		}
		soulBreaks[name] = sbList
	}

	// Process with 20 parallel workers
	var wg sync.WaitGroup
	ch := make(chan imageJob)

	for w := 0; w < 20; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range ch {
				if img := ensureCachedImage(job.dir, job.urlPath, job.remoteFmt, job.id); img != "" {
					*job.result = img
				}
			}
		}()
	}

	for _, job := range jobs {
		ch <- job
	}
	close(ch)
	wg.Wait()
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
.char-card .name { font-size: 14px; line-height: 1.3; word-wrap: break-word; }
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
       background: #1a1a2e; color: #e0e0e0; padding: 20px; max-width: 1200px; margin: 0 auto;
       font-size: 16px; }
a { color: #e94560; }
h1 { color: #e94560; margin-bottom: 4px; }
.subtitle { color: #888; margin-bottom: 20px; }
.back { display: inline-block; margin-bottom: 16px; }
h2 { color: #0f3460; background: #e94560; display: inline-block;
     padding: 4px 16px; border-radius: 4px; margin: 20px 0 12px; font-size: 1em; }
.school-grid { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.school-badge { background: #16213e; border: 1px solid #0f3460; border-radius: 4px;
                padding: 4px 10px; font-size: 15px; }
.school-badge .stars { color: #e9c46a; }
table { width: 100%; border-collapse: collapse; margin-bottom: 8px; font-size: 15px; }
th { background: #0f3460; padding: 6px 8px; text-align: left; }
td { background: #16213e; padding: 6px 8px; border-bottom: 1px solid #1a1a2e; }
td img { width: 64px; height: 64px; object-fit: contain; vertical-align: middle; }
.effects { max-width: 500px; }
.sb-icon { width: 64px; height: 64px; overflow: hidden; display: inline-block; vertical-align: middle; }
.sb-icon img { width: 96px; height: 96px; object-fit: contain; margin: -16px; }
tr.sb-row { cursor: pointer; }
tr.sb-row:hover td { background: #1b2a4a; }
td.sb-chevron { width: 16px; padding: 6px 2px; text-align: center; }
td.sb-chevron .chevron { display: inline-block; font-size: 14px; color: #e94560;
                          transition: transform 0.2s; }
tr.sb-row.expanded td.sb-chevron .chevron { transform: rotate(90deg); }
tr.sb-detail { display: none; }
tr.sb-detail.visible { display: table-row; }
tr.sb-detail td { background: #0d1a30; padding: 8px 16px; }
.effect-list { list-style: none; padding: 0; margin: 0; }
.effect-list li { padding: 4px 0; border-bottom: 1px solid #16213e; }
.effect-list li:last-child { border-bottom: none; }
.effect-name { color: #e94560; font-weight: bold; }
.effect-duration { color: #e9c46a; font-size: 14px; margin-left: 8px; }
.effect-desc { color: #aaa; font-size: 14px; display: block; margin-top: 2px; }
.burst-commands { margin-bottom: 8px; }
.burst-title { color: #e94560; font-weight: bold; margin-bottom: 6px; font-size: 15px; }
.burst-table { width: 100%; border-collapse: collapse; margin-bottom: 4px; font-size: 14px; }
.burst-table th { background: #162040; padding: 4px 6px; text-align: left; font-size: 13px; }
.burst-table td { background: #0f1a2e; padding: 4px 6px; border-bottom: 1px solid #1a1a2e; }
.burst-icon { width: 32px; height: 32px; object-fit: contain; vertical-align: middle; }
tr.bc-row { cursor: pointer; }
tr.bc-row:hover td { background: #152035; }
tr.bc-row td.bc-chevron { width: 16px; padding: 4px 2px; text-align: center; }
tr.bc-row td.bc-chevron .chevron { display: inline-block; font-size: 12px; color: #e94560;
                                    transition: transform 0.2s; }
tr.bc-row.expanded td.bc-chevron .chevron { transform: rotate(90deg); }
tr.bc-detail { display: none; }
tr.bc-detail.visible { display: table-row; }
tr.bc-detail td { background: #0a1220; padding: 6px 12px; }
.brave-condition { color: #aaa; font-size: 13px; margin-bottom: 6px; }
.brave-icon-wrap { position: relative; width: 32px; height: 32px; }
.brave-icon-wrap .brave-bg { width: 32px; height: 32px; object-fit: contain; position: absolute; top: 0; left: 0; }
.brave-icon-wrap .brave-fg { width: 32px; height: 32px; object-fit: contain; position: absolute; top: 0; left: 0; }
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
<tr><th></th><th></th><th>Name</th><th>Tier</th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
{{range .SoulBreaks}}
<tr class="sb-row{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .BraveCommand}} expandable{{end}}" onclick="toggleDetail(this)">
  <td class="sb-chevron">{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .BraveCommand}}<span class="chevron">&#9654;</span>{{end}}</td>
  <td>{{if .Img}}<span class="sb-icon"><img src="{{.Img}}"></span>{{end}}</td>
  <td>{{.Name}}</td>
  <td>{{.Tier}}</td>
  <td>{{.Type}}</td>
  <td>{{.Element}}</td>
  <td>{{.Time}}</td>
  <td class="effects">{{.Effects}}</td>
</tr>
{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .BraveCommand}}
<tr class="sb-detail">
  <td colspan="8">
    {{if .BraveCommand}}
    <div class="burst-commands">
      <div class="burst-title">Brave Command: {{.BraveCommand.Name}}</div>
      <div class="brave-condition">Condition: {{.BraveCommand.Condition}} | School: {{.BraveCommand.School}}</div>
      <table class="burst-table">
        <tr><th></th><th></th><th>Level</th><th>Type</th><th>Target</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        {{range .BraveCommand.Levels}}
        <tr class="bc-row{{if .MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td><div class="brave-icon-wrap"><img src="/images/brave/BraveAttack{{.Level}}.png" class="brave-bg"><img src="/images/brave/BraveBase.png" class="brave-fg"></div></td>
          <td>{{.Level}}</td>
          <td>{{.Type}}</td>
          <td>{{.Target}}</td>
          <td>{{.Element}}</td>
          <td>{{.Time}}</td>
          <td class="effects">{{.Effects}}</td>
        </tr>
        {{if .MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="8">
            <ul class="effect-list">
            {{range .MatchedEffects}}
              <li>
                <span class="effect-name">{{.Name}}</span>
                {{if and .Duration (ne .Duration "-")}}<span class="effect-duration">({{.Duration}})</span>{{end}}
                <span class="effect-desc">{{.Description}}</span>
              </li>
            {{end}}
            </ul>
          </td>
        </tr>
        {{end}}
        {{end}}
      </table>
    </div>
    {{end}}
    {{if .DualShift}}
    <div class="burst-commands">
      <div class="burst-title">Dual Shift</div>
      <table class="burst-table">
        <tr><th></th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        <tr class="bc-row{{if .DualShift.MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .DualShift.MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td>{{.DualShift.Type}}</td>
          <td>{{.DualShift.Element}}</td>
          <td>{{.DualShift.Time}}</td>
          <td class="effects">{{.DualShift.Effects}}</td>
        </tr>
        {{if .DualShift.MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="5">
            <ul class="effect-list">
            {{range .DualShift.MatchedEffects}}
              <li>
                <span class="effect-name">{{.Name}}</span>
                {{if and .Duration (ne .Duration "-")}}<span class="effect-duration">({{.Duration}})</span>{{end}}
                <span class="effect-desc">{{.Description}}</span>
              </li>
            {{end}}
            </ul>
          </td>
        </tr>
        {{end}}
      </table>
    </div>
    {{end}}
    {{if .ZenithAbilities}}
    <div class="burst-commands">
      <div class="burst-title">Zenith SB Abilities</div>
      <table class="burst-table">
        <tr><th></th><th></th><th>Name</th><th>School</th><th>Type</th><th>Target</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        {{range .ZenithAbilities}}
        <tr class="bc-row{{if .MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td>{{if .Img}}<img src="{{.Img}}" class="burst-icon">{{end}}</td>
          <td>{{.Name}}</td>
          <td>{{.School}}</td>
          <td>{{.Type}}</td>
          <td>{{.Target}}</td>
          <td>{{.Element}}</td>
          <td>{{.Time}}</td>
          <td class="effects">{{.Effects}}</td>
        </tr>
        {{if .MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="9">
            <ul class="effect-list">
            {{range .MatchedEffects}}
              <li>
                <span class="effect-name">{{.Name}}</span>
                {{if and .Duration (ne .Duration "-")}}<span class="effect-duration">({{.Duration}})</span>{{end}}
                <span class="effect-desc">{{.Description}}</span>
              </li>
            {{end}}
            </ul>
          </td>
        </tr>
        {{end}}
        {{end}}
      </table>
    </div>
    {{end}}
    {{if .SynchroAbilities}}
    <div class="burst-commands">
      <div class="burst-title">Synchro Abilities</div>
      <table class="burst-table">
        <tr><th></th><th></th><th>Name</th><th>School</th><th>Condition</th><th>Type</th><th>Target</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        {{range .SynchroAbilities}}
        <tr class="bc-row{{if .MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td>{{if .Img}}<img src="{{.Img}}" class="burst-icon">{{end}}</td>
          <td>{{.Name}}</td>
          <td>{{.School}}</td>
          <td>{{.Condition}}</td>
          <td>{{.Type}}</td>
          <td>{{.Target}}</td>
          <td>{{.Element}}</td>
          <td>{{.Time}}</td>
          <td class="effects">{{.Effects}}</td>
        </tr>
        {{if .MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="10">
            <ul class="effect-list">
            {{range .MatchedEffects}}
              <li>
                <span class="effect-name">{{.Name}}</span>
                {{if and .Duration (ne .Duration "-")}}<span class="effect-duration">({{.Duration}})</span>{{end}}
                <span class="effect-desc">{{.Description}}</span>
              </li>
            {{end}}
            </ul>
          </td>
        </tr>
        {{end}}
        {{end}}
      </table>
    </div>
    {{end}}
    {{if .BurstCommands}}
    <div class="burst-commands">
      <div class="burst-title">Burst Commands</div>
      <table class="burst-table">
        <tr><th></th><th></th><th>Name</th><th>School</th><th>Type</th><th>Target</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        {{range .BurstCommands}}
        <tr class="bc-row{{if .MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td>{{if .Img}}<img src="{{.Img}}" class="burst-icon">{{end}}</td>
          <td>{{.Name}}</td>
          <td>{{.School}}</td>
          <td>{{.Type}}</td>
          <td>{{.Target}}</td>
          <td>{{.Element}}</td>
          <td>{{.Time}}</td>
          <td class="effects">{{.Effects}}</td>
        </tr>
        {{if .MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="9">
            <ul class="effect-list">
            {{range .MatchedEffects}}
              <li>
                <span class="effect-name">{{.Name}}</span>
                {{if and .Duration (ne .Duration "-")}}<span class="effect-duration">({{.Duration}})</span>{{end}}
                <span class="effect-desc">{{.Description}}</span>
              </li>
            {{end}}
            </ul>
          </td>
        </tr>
        {{end}}
        {{end}}
      </table>
    </div>
    {{end}}
    {{if .MatchedEffects}}
    <ul class="effect-list">
    {{range .MatchedEffects}}
      <li>
        <span class="effect-name">{{.Name}}</span>
        {{if and .Duration (ne .Duration "-")}}<span class="effect-duration">({{.Duration}})</span>{{end}}
        <span class="effect-desc">{{.Description}}</span>
      </li>
    {{end}}
    </ul>
    {{end}}
  </td>
</tr>
{{end}}
{{end}}
</table>
{{end}}

<script>
function toggleDetail(row) {
  var detail = row.nextElementSibling;
  if (!detail || !detail.classList.contains('sb-detail')) return;
  row.classList.toggle('expanded');
  detail.classList.toggle('visible');
}
function toggleBcDetail(row) {
  var detail = row.nextElementSibling;
  if (!detail || !detail.classList.contains('bc-detail')) return;
  row.classList.toggle('expanded');
  detail.classList.toggle('visible');
  event.stopPropagation();
}
</script>
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
	loadBurstCommands()
	loadSynchroAbilities()
	loadZenithAbilities()
	loadBraveCommands()
	loadStatuses()
	matchSoulBreakEffects()
	pairDualShifts()
	matchBurstCommands()
	matchSynchroAbilities()
	matchZenithAbilities()
	matchBraveCommands()

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
