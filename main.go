package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
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
	ArcaneDyad       *SoulBreak
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

type SearchResult struct {
	Character     Character
	HeroAbilities []HeroAbility
	SoulBreaks    []SoulBreak
}

type RealmGroup struct {
	Realm      string
	Characters []Character
}

type CharDetail struct {
	Character       Character
	SoulBreaks      []SoulBreak
	HeroAbilities   []HeroAbility
	LoggedIn        bool
	OwnedSoulbreaks map[string]bool
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
	tierNames     []string
	dataLock      sync.RWMutex
)

// ---------- user types & globals ----------

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	APIKey       string `json:"api_key"`
}

var (
	users          map[int]*User          // id → user
	usersByName    map[string]*User       // lowercase username → user
	userSoulbreaks map[int]map[string]bool // user_id → set of soulbreak IDs
	sessions       map[string]int          // session_token → user_id
	userLock       sync.RWMutex
	nextUserID     int
)

// ---------- CSV auto-update ----------

type csvSheet struct {
	Filename        string
	GID             string
	ExpectedHeaders []string
}

var csvSheets = []csvSheet{
	{"Characters.csv", "1771023676", []string{"Realm", "Name", "ID"}},
	{"Soul-Breaks.csv", "344457459", []string{"Character", "Name", "Tier"}},
	{"Hero-Abilities.csv", "329671300", []string{"Character", "Name", "Type"}},
	{"Burst.csv", "1373487754", []string{"Character", "Source", "Name"}},
	{"Synchro.csv", "13552509", []string{"Character", "Source", "Name"}},
	{"Zenith-SB-Abilities.csv", "1801274757", []string{"Character", "Source", "Name"}},
	{"Brave.csv", "1286318057", []string{"Character", "Source", "Name"}},
	{"Status.csv", "1899148923", []string{"Common Name", "Effects"}},
}

const sheetID = "1f8OJIQhpycljDQ8QNDk_va1GJ1u7RVoMaNjFcHH0LKk"

func downloadCSV(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func validateCSV(data []byte, expectedHeaders []string, currentRowCount int) error {
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) < 1 {
		return fmt.Errorf("CSV has no rows")
	}
	header := records[0]
	headerSet := make(map[string]bool, len(header))
	for _, h := range header {
		headerSet[h] = true
	}
	for _, exp := range expectedHeaders {
		if !headerSet[exp] {
			return fmt.Errorf("missing expected header column %q", exp)
		}
	}
	newRowCount := len(records) - 1 // exclude header
	if currentRowCount > 0 && newRowCount < currentRowCount/2 {
		return fmt.Errorf("row count %d is less than 50%% of current %d", newRowCount, currentRowCount)
	}
	fieldCount := len(header)
	for i, rec := range records[1:] {
		if len(rec) > fieldCount*2 {
			return fmt.Errorf("row %d has %d fields (header has %d), possible mangled row", i+1, len(rec), fieldCount)
		}
	}
	return nil
}

func countCSVRows(filename string) int {
	f, err := os.Open(filename)
	if err != nil {
		return 0
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return 0
	}
	if len(records) < 1 {
		return 0
	}
	return len(records) - 1
}

func updateCSVs() {
	anyUpdated := false
	for _, sheet := range csvSheets {
		url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s", sheetID, sheet.GID)
		data, err := downloadCSV(url)
		if err != nil {
			log.Printf("WARNING: failed to download %s: %v", sheet.Filename, err)
			continue
		}
		currentRows := countCSVRows(sheet.Filename)
		if err := validateCSV(data, sheet.ExpectedHeaders, currentRows); err != nil {
			log.Printf("WARNING: validation failed for %s: %v", sheet.Filename, err)
			continue
		}
		tmpFile := sheet.Filename + ".tmp"
		if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
			log.Printf("WARNING: failed to write %s: %v", tmpFile, err)
			continue
		}
		if err := os.Rename(tmpFile, sheet.Filename); err != nil {
			log.Printf("WARNING: failed to rename %s to %s: %v", tmpFile, sheet.Filename, err)
			continue
		}
		log.Printf("Updated %s (%d bytes)", sheet.Filename, len(data))
		anyUpdated = true
	}
	if anyUpdated {
		log.Println("Reloading data after CSV update...")
		reloadData()
		log.Println("Data reload complete.")
	}
}

func reloadData() {
	dataLock.Lock()
	defer dataLock.Unlock()

	characters = nil
	realmGroups = nil

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
	pairArcaneDyads()
	matchBurstCommands()
	matchSynchroAbilities()
	matchZenithAbilities()
	matchBraveCommands()
	cacheAllImages()
	buildRealmGroups()

	log.Printf("Reloaded %d characters", len(characters))
}

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

var invalidTiers = map[string]bool{
	"Tier": true, "Default": true, "Shared": true, "Self": true,
	"N": true,
}

func isValidTier(s string) bool {
	if len(s) == 0 || len(s) > 10 || invalidTiers[s] {
		return false
	}
	hasUpper := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		} else if (r >= 'a' && r <= 'z') || r == '+' {
			continue
		} else {
			return false
		}
	}
	return hasUpper
}

func loadSoulBreaks() {
	soulBreaks = make(map[string][]SoulBreak)
	tierSet := make(map[string]bool)
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
		if isValidTier(sb.Tier) {
			tierSet[sb.Tier] = true
		}
	}
	tierNames = nil
	for t := range tierSet {
		tierNames = append(tierNames, t)
	}
	sort.Strings(tierNames)
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

func pairArcaneDyads() {
	for name, sbList := range soulBreaks {
		// Index primary ADSBs (Engaged) by "SBVer" for this character
		primaries := make(map[string]int) // SBVer -> index in sbList
		for i, sb := range sbList {
			if sb.Tier == "ADSB" && strings.HasSuffix(sb.Name, "(Engaged)") {
				primaries[sb.SBVer] = i
			}
		}
		// Match non-Engaged entries to their primaries
		remove := make(map[int]bool)
		for i, sb := range sbList {
			if sb.Tier == "ADSB" && !strings.HasSuffix(sb.Name, "(Engaged)") {
				if pi, ok := primaries[sb.SBVer]; ok {
					ad := sb
					sbList[pi].ArcaneDyad = &ad
					// Strip "(Engaged)" from the primary's display name
					sbList[pi].Name = strings.TrimSpace(strings.TrimSuffix(sbList[pi].Name, "(Engaged)"))
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

// ---------- user data persistence ----------

const soulbreaksDir = "data/soulbreaks"

func initUserData() {
	users = make(map[int]*User)
	usersByName = make(map[string]*User)
	userSoulbreaks = make(map[int]map[string]bool)
	sessions = make(map[string]int)
	nextUserID = 1

	usersJSON := "data/users.json"

	os.MkdirAll("data", 0o755)
	os.MkdirAll(soulbreaksDir, 0o755)

	// Try loading from JSON first
	if _, err := os.Stat(usersJSON); err == nil {
		loadUsersJSON(usersJSON)
		loadAllUserSoulbreaks()
		return
	}

	// Try importing from CSV
	if _, err := os.Stat("users.csv"); err == nil {
		importUsersFromCSV()
		saveUsersJSON(usersJSON)
		saveAllUserSoulbreaks()
		return
	}

	log.Println("No user data found, starting fresh")
}

func loadUsersJSON(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("WARNING: could not read %s: %v", path, err)
		return
	}
	var userList []User
	if err := json.Unmarshal(data, &userList); err != nil {
		log.Printf("WARNING: could not parse %s: %v", path, err)
		return
	}
	for i := range userList {
		u := &userList[i]
		users[u.ID] = u
		usersByName[strings.ToLower(u.Username)] = u
		if u.ID >= nextUserID {
			nextUserID = u.ID + 1
		}
	}
	log.Printf("Loaded %d users from %s", len(users), path)
}

func loadAllUserSoulbreaks() {
	entries, err := os.ReadDir(soulbreaksDir)
	if err != nil {
		log.Printf("WARNING: could not read %s: %v", soulbreaksDir, err)
		return
	}
	total := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		uidStr := strings.TrimSuffix(name, ".json")
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(soulbreaksDir, name))
		if err != nil {
			log.Printf("WARNING: could not read %s: %v", name, err)
			continue
		}
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			log.Printf("WARNING: could not parse %s: %v", name, err)
			continue
		}
		set := make(map[string]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		userSoulbreaks[uid] = set
		total += len(ids)
	}
	log.Printf("Loaded %d user soulbreak records from %s/", total, soulbreaksDir)
}

func saveUsersJSON(path string) {
	var userList []User
	for _, u := range users {
		userList = append(userList, *u)
	}
	sort.Slice(userList, func(i, j int) bool { return userList[i].ID < userList[j].ID })
	data, err := json.Marshal(userList)
	if err != nil {
		log.Printf("WARNING: could not marshal users: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARNING: could not write %s: %v", path, err)
	}
}

func saveUserSoulbreaksForUser(uid int) {
	set := userSoulbreaks[uid]
	path := filepath.Join(soulbreaksDir, strconv.Itoa(uid)+".json")
	if len(set) == 0 {
		os.Remove(path)
		return
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	data, err := json.Marshal(ids)
	if err != nil {
		log.Printf("WARNING: could not marshal soulbreaks for user %d: %v", uid, err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARNING: could not write %s: %v", path, err)
	}
}

func saveAllUserSoulbreaks() {
	for uid := range userSoulbreaks {
		saveUserSoulbreaksForUser(uid)
	}
}

func importUsersFromCSV() {
	log.Println("Importing users from CSV...")

	// Read users.csv: id, username, bcrypt_hash, api_key (no headers)
	f, err := os.Open("users.csv")
	if err != nil {
		log.Printf("WARNING: could not open users.csv: %v", err)
		return
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		log.Printf("WARNING: could not read users.csv: %v", err)
		return
	}
	for _, rec := range records {
		if len(rec) < 4 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			continue
		}
		u := &User{
			ID:           id,
			Username:     strings.TrimSpace(rec[1]),
			PasswordHash: strings.TrimSpace(rec[2]),
			APIKey:       strings.TrimSpace(rec[3]),
		}
		users[u.ID] = u
		usersByName[strings.ToLower(u.Username)] = u
		if u.ID >= nextUserID {
			nextUserID = u.ID + 1
		}
	}
	log.Printf("Imported %d users from CSV", len(users))

	// Read user_soulbreaks.csv: user_id, soulbreak_id (no headers)
	if _, err := os.Stat("user_soulbreaks.csv"); err != nil {
		return
	}
	f2, err := os.Open("user_soulbreaks.csv")
	if err != nil {
		log.Printf("WARNING: could not open user_soulbreaks.csv: %v", err)
		return
	}
	defer f2.Close()
	r2 := csv.NewReader(f2)
	r2.LazyQuotes = true
	records2, err := r2.ReadAll()
	if err != nil {
		log.Printf("WARNING: could not read user_soulbreaks.csv: %v", err)
		return
	}
	count := 0
	for _, rec := range records2 {
		if len(rec) < 2 {
			continue
		}
		uid, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			continue
		}
		sbID := strings.TrimSpace(rec[1])
		if sbID == "" {
			continue
		}
		if userSoulbreaks[uid] == nil {
			userSoulbreaks[uid] = make(map[string]bool)
		}
		userSoulbreaks[uid][sbID] = true
		count++
	}
	log.Printf("Imported %d soulbreak ownership records from CSV", count)
}

// ---------- session management ----------

func generateSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand failed: %v", err)
	}
	return hex.EncodeToString(b)
}

func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password + "ffrk"))
	return hex.EncodeToString(h[:])
}

func createSession(w http.ResponseWriter, userID int) {
	token := generateSessionToken()
	userLock.Lock()
	sessions[token] = userID
	userLock.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getCurrentUser(r *http.Request) *User {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	userLock.RLock()
	defer userLock.RUnlock()
	uid, ok := sessions[cookie.Value]
	if !ok {
		return nil
	}
	return users[uid]
}

// ---------- auth API handlers ----------

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || req.Password == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username and password are required"})
		return
	}

	userLock.RLock()
	u := usersByName[strings.ToLower(req.Username)]
	userLock.RUnlock()
	if u == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid username or password"})
		return
	}

	sha256Hex := hashPassword(req.Password)
	// PHP uses $2y$ prefix; Go's bcrypt accepts $2a$ — replace prefix for compat
	storedHash := strings.Replace(u.PasswordHash, "$2y$", "$2a$", 1)
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(sha256Hex)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid username or password"})
		return
	}

	createSession(w, u.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": u.Username})
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || len(req.Password) < 6 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username required, password must be at least 6 characters"})
		return
	}
	if len(req.Username) > 50 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username too long (max 50 characters)"})
		return
	}

	userLock.Lock()
	if _, exists := usersByName[strings.ToLower(req.Username)]; exists {
		userLock.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "Username already taken"})
		return
	}

	sha256Hex := hashPassword(req.Password)
	hash, err := bcrypt.GenerateFromPassword([]byte(sha256Hex), bcrypt.DefaultCost)
	if err != nil {
		userLock.Unlock()
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	apiKey := fmt.Sprintf("%x", md5.Sum([]byte(time.Now().String()+req.Username)))
	u := &User{
		ID:           nextUserID,
		Username:     req.Username,
		PasswordHash: string(hash),
		APIKey:       apiKey,
	}
	nextUserID++
	users[u.ID] = u
	usersByName[strings.ToLower(u.Username)] = u
	userLock.Unlock()

	saveUsersJSON("data/users.json")
	createSession(w, u.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": u.Username})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie("session")
	if err == nil {
		userLock.Lock()
		delete(sessions, cookie.Value)
		userLock.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not logged in"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": u.Username})
}

func userSoulbreaksGetHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not logged in"})
		return
	}
	userLock.RLock()
	owned := userSoulbreaks[u.ID]
	var ids []string
	for id := range owned {
		ids = append(ids, id)
	}
	userLock.RUnlock()
	sort.Strings(ids)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ids)
}

func userSoulbreaksPostHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not logged in"})
		return
	}
	var req struct {
		SoulbreakID string `json:"soulbreak_id"`
		Owned       bool   `json:"owned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SoulbreakID == "" {
		http.Error(w, "soulbreak_id required", http.StatusBadRequest)
		return
	}

	userLock.Lock()
	if userSoulbreaks[u.ID] == nil {
		userSoulbreaks[u.ID] = make(map[string]bool)
	}
	if req.Owned {
		userSoulbreaks[u.ID][req.SoulbreakID] = true
	} else {
		delete(userSoulbreaks[u.ID], req.SoulbreakID)
	}
	userLock.Unlock()

	saveUserSoulbreaksForUser(u.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

func userSoulbreaksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userSoulbreaksGetHandler(w, r)
	case http.MethodPost:
		userSoulbreaksPostHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- templates ----------

const searchBarCSS = `
.search-bar { position: fixed; top: 0; left: 0; right: 0; z-index: 100;
              background: #16213e; border-bottom: 2px solid #0f3460;
              padding: 10px 20px; display: flex; flex-wrap: wrap; gap: 8px;
              align-items: center; }
.search-bar input, .search-bar select { background: #1a1a2e; color: #e0e0e0;
              border: 1px solid #0f3460; border-radius: 4px; padding: 6px 10px;
              font-size: 14px; }
.search-bar input:focus, .search-bar select:focus { outline: none; border-color: #e94560; }
.search-bar input[type="text"] { width: 180px; }
.search-bar select { min-width: 100px; }
.search-bar button { background: #e94560; color: #fff; border: none; border-radius: 4px;
                     padding: 6px 16px; font-size: 14px; cursor: pointer; }
.search-bar button:hover { background: #d63050; }
.search-bar label { color: #888; font-size: 12px; margin-right: -4px; }
.modal-overlay { display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0;
                 background: rgba(0,0,0,0.6); z-index: 200; align-items: center; justify-content: center; }
.modal-overlay.visible { display: flex; }
.modal { background: #16213e; border: 2px solid #0f3460; border-radius: 8px;
         padding: 20px; min-width: 500px; max-width: 90vw; }
.modal h3 { color: #e94560; margin-bottom: 12px; }
.modal-columns { display: flex; gap: 24px; flex-wrap: wrap; }
.modal-col { display: flex; flex-direction: column; gap: 8px; min-width: 120px; }
.modal-col label { color: #e0e0e0; font-size: 14px; cursor: pointer; display: flex; align-items: center; gap: 6px; }
.modal-col input[type="checkbox"] { accent-color: #e94560; }
.modal-buttons { margin-top: 16px; display: flex; gap: 8px; justify-content: flex-end; }
.modal-buttons button { background: #e94560; color: #fff; border: none; border-radius: 4px;
                        padding: 6px 16px; font-size: 14px; cursor: pointer; }
.modal-buttons button:hover { background: #d63050; }
.modal-buttons button.secondary { background: #0f3460; }
.modal-buttons button.secondary:hover { background: #1a3a6e; }
.effects-badge { background: #0f3460; color: #e94560; border-radius: 10px; padding: 1px 7px;
                 font-size: 11px; margin-left: 4px; }
#owned-filter { color: #e0e0e0; font-size: 13px; cursor: pointer; display: flex; align-items: center; gap: 4px; }
.school-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px 16px; }
.school-row { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
.school-row span { color: #e0e0e0; font-size: 13px; white-space: nowrap; }
.school-row select { background: #1a1a2e; color: #e0e0e0; border: 1px solid #0f3460;
                     border-radius: 4px; padding: 2px 4px; font-size: 12px; }
.school-toggle { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; color: #e0e0e0; font-size: 14px; }
.school-toggle button { padding: 4px 12px; font-size: 12px; border: 1px solid #0f3460;
                        border-radius: 4px; cursor: pointer; background: #1a1a2e; color: #e0e0e0; }
.school-toggle button.active { background: #e94560; border-color: #e94560; color: #fff; }
#owned-filter input[type="checkbox"] { accent-color: #e94560; cursor: pointer; }
.search-bar .auth-area { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.search-bar .auth-area .welcome { color: #e0e0e0; font-size: 14px; }
.search-bar .auth-area button { font-size: 13px; padding: 5px 12px; }
.auth-modal-overlay { display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0;
                      background: rgba(0,0,0,0.6); z-index: 200; align-items: center; justify-content: center; }
.auth-modal-overlay.visible { display: flex; }
.auth-modal { background: #16213e; border: 2px solid #0f3460; border-radius: 8px;
              padding: 24px; min-width: 320px; max-width: 90vw; }
.auth-modal h3 { color: #e94560; margin-bottom: 16px; }
.auth-modal .form-group { margin-bottom: 12px; }
.auth-modal label { display: block; color: #e0e0e0; font-size: 14px; margin-bottom: 4px; }
.auth-modal input[type="text"], .auth-modal input[type="password"] {
    width: 100%; background: #1a1a2e; color: #e0e0e0; border: 1px solid #0f3460;
    border-radius: 4px; padding: 8px 10px; font-size: 14px; box-sizing: border-box; }
.auth-modal .error-msg { color: #e94560; font-size: 13px; margin-bottom: 8px; display: none; }
.auth-modal .modal-buttons { margin-top: 16px; }
`

const searchBarHTML = `
<div class="search-bar">
  <label>Character</label>
  <input type="text" id="char-input" list="char-list" placeholder="Name...">
  <datalist id="char-list"></datalist>
  <label>Realm</label>
  <select id="realm-select">
    <option value="">Any</option>
    <option>I</option><option>II</option><option>III</option><option>IV</option>
    <option>V</option><option>VI</option><option>VII</option><option>VIII</option>
    <option>IX</option><option>X</option><option>XI</option><option>XII</option>
    <option>XIII</option><option>XIV</option><option>XV</option><option>XVI</option>
    <option>FFT</option><option>Type-0</option><option>KH</option><option>Beyond</option><option>Core</option>
  </select>
  <label>SB Tier</label>
  <select id="tier-select">
    <option value="">Any</option>
  </select>
  <label>En-Element</label>
  <select id="en-element-select">
    <option value="">Any</option>
    <option>Fire</option><option>Ice</option><option>Wind</option><option>Earth</option>
    <option>Lightning</option><option>Water</option><option>Dark</option><option>Holy</option>
    <option>Poison</option>
  </select>
  <label>Imperil</label>
  <select id="imperil-select">
    <option value="">Any</option>
    <option>Fire</option><option>Ice</option><option>Wind</option><option>Earth</option>
    <option>Lightning</option><option>Water</option><option>Dark</option><option>Holy</option>
    <option>Poison</option><option>Prismatic</option>
  </select>
  <button onclick="openEffectsModal()">Additional Effects<span id="effects-badge" class="effects-badge" style="display:none"></span></button>
  <button onclick="openSchoolsModal()">Job Requirements<span id="schools-badge" class="effects-badge" style="display:none"></span></button>
  <button onclick="doSearch()">Search</button>
  <label id="owned-filter" style="display:none"><input type="checkbox" id="owned-only"> Owned only</label>
  <div class="auth-area" id="auth-area"></div>
</div>
<div class="auth-modal-overlay" id="login-modal">
  <div class="auth-modal">
    <h3>Login</h3>
    <div class="error-msg" id="login-error"></div>
    <div class="form-group"><label>Username</label><input type="text" id="login-username"></div>
    <div class="form-group"><label>Password</label><input type="password" id="login-password"></div>
    <div class="modal-buttons">
      <button class="secondary" onclick="closeAuthModals()">Cancel</button>
      <button onclick="doLogin()">Login</button>
    </div>
  </div>
</div>
<div class="auth-modal-overlay" id="register-modal">
  <div class="auth-modal">
    <h3>Register</h3>
    <div class="error-msg" id="register-error"></div>
    <div class="form-group"><label>Username</label><input type="text" id="register-username"></div>
    <div class="form-group"><label>Password (min 6 chars)</label><input type="password" id="register-password"></div>
    <div class="modal-buttons">
      <button class="secondary" onclick="closeAuthModals()">Cancel</button>
      <button onclick="doRegister()">Register</button>
    </div>
  </div>
</div>
<div class="modal-overlay" id="effects-modal">
  <div class="modal">
    <h3>Additional Effects</h3>
    <div class="modal-columns">
      <div class="modal-col">
        <label><input type="checkbox" value="aegis_counter"> Aegis Counter</label>
        <label><input type="checkbox" value="fullbreak_counter"> Fullbreak Counter</label>
        <label><input type="checkbox" value="phys_job_break_counter"> Phys Job Break Counter</label>
        <label><input type="checkbox" value="mag_job_break_counter"> Mag Job Break Counter</label>
      </div>
      <div class="modal-col">
        <label><input type="checkbox" value="haste"> Haste</label>
        <label><input type="checkbox" value="protect"> Protect</label>
        <label><input type="checkbox" value="shell"> Shell</label>
      </div>
      <div class="modal-col">
        <label><input type="checkbox" value="last_stand"> Last Stand</label>
        <label><input type="checkbox" value="regen"> Regen</label>
        <label><input type="checkbox" value="regenga"> Regenga</label>
        <label><input type="checkbox" value="astra"> Astra</label>
      </div>
      <div class="modal-col">
        <label><input type="checkbox" value="crit_chance"> Critical Chance</label>
        <label><input type="checkbox" value="crit_damage"> Critical Damage</label>
        <label><input type="checkbox" value="sb_gauge"> SB Gauge</label>
        <label><input type="checkbox" value="deshell"> Deshell</label>
        <label><input type="checkbox" value="deprotect"> Deprotect</label>
      </div>
      <div class="modal-col">
        <label><input type="checkbox" value="dualcast"> Dualcast</label>
        <label><input type="checkbox" value="triplecast"> Triplecast</label>
        <label><input type="checkbox" value="instant_atb"> Instant ATB</label>
        <label><input type="checkbox" value="atb_speed"> ATB Speed</label>
      </div>
      <div class="modal-col">
        <label><input type="checkbox" value="weakness_boost"> Weakness Boost</label>
        <label><input type="checkbox" value="magical_boost"> Magical Boost</label>
        <label><input type="checkbox" value="phy_boost"> PHY Boost</label>
        <label><input type="checkbox" value="sorcery_boost"> Sorcery Damage Boost</label>
        <label><input type="checkbox" value="pentabreak_boost"> Pentabreak Damage Boost</label>
      </div>
    </div>
    <div class="modal-buttons">
      <button class="secondary" onclick="clearEffects()">Clear All</button>
      <button onclick="closeEffectsModal()">Done</button>
    </div>
  </div>
</div>
<div class="modal-overlay" id="schools-modal">
  <div class="modal">
    <h3>Job Requirements</h3>
    <div class="school-toggle">
      <span>Match:</span>
      <button id="school-mode-and" class="active" onclick="setSchoolMode('and')">ALL (AND)</button>
      <button id="school-mode-or" onclick="setSchoolMode('or')">ANY (OR)</button>
    </div>
    <div class="school-grid">
      <div class="school-row"><span>Black Magic</span><select data-school="Black Magic"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>White Magic</span><select data-school="White Magic"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Combat</span><select data-school="Combat"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Support</span><select data-school="Support"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Celerity</span><select data-school="Celerity"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Summoning</span><select data-school="Summoning"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Spellblade</span><select data-school="Spellblade"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Dragoon</span><select data-school="Dragoon"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Monk</span><select data-school="Monk"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Thief</span><select data-school="Thief"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Knight</span><select data-school="Knight"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Samurai</span><select data-school="Samurai"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Ninja</span><select data-school="Ninja"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Bard</span><select data-school="Bard"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Dancer</span><select data-school="Dancer"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Machinist</span><select data-school="Machinist"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Darkness</span><select data-school="Darkness"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Sharpshooter</span><select data-school="Sharpshooter"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Witch</span><select data-school="Witch"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
      <div class="school-row"><span>Heavy</span><select data-school="Heavy"><option value="">--</option><option value="1">1★</option><option value="2">2★</option><option value="3">3★</option><option value="4">4★</option><option value="5">5★</option><option value="6">6★</option></select></div>
    </div>
    <div class="modal-buttons">
      <button class="secondary" onclick="clearSchools()">Clear All</button>
      <button onclick="closeSchoolsModal()">Done</button>
    </div>
  </div>
</div>
<script>
fetch('/api/characters').then(r=>r.json()).then(names=>{
  var dl=document.getElementById('char-list');
  names.forEach(function(n){var o=document.createElement('option');o.value=n;dl.appendChild(o)});
});
fetch('/api/tiers').then(r=>r.json()).then(tiers=>{
  var sel=document.getElementById('tier-select');
  tiers.forEach(function(t){var o=document.createElement('option');o.value=t;o.textContent=t;sel.appendChild(o)});
  var p=new URLSearchParams(window.location.search);
  if(p.get('tier'))sel.value=p.get('tier');
});
function openEffectsModal(){document.getElementById('effects-modal').classList.add('visible')}
function closeEffectsModal(){document.getElementById('effects-modal').classList.remove('visible');updateBadge()}
function clearEffects(){
  document.querySelectorAll('#effects-modal input[type=checkbox]').forEach(function(cb){cb.checked=false});
  updateBadge();
}
function getCheckedEffects(){
  var eff=[];
  document.querySelectorAll('#effects-modal input[type=checkbox]:checked').forEach(function(cb){eff.push(cb.value)});
  return eff;
}
function updateBadge(){
  var eff=getCheckedEffects();
  var badge=document.getElementById('effects-badge');
  if(eff.length>0){badge.textContent=eff.length;badge.style.display='inline'}
  else{badge.style.display='none'}
}
document.getElementById('effects-modal').addEventListener('click',function(e){
  if(e.target===this)closeEffectsModal();
});
var schoolMode='and';
function openSchoolsModal(){document.getElementById('schools-modal').classList.add('visible')}
function closeSchoolsModal(){document.getElementById('schools-modal').classList.remove('visible');updateSchoolsBadge()}
function clearSchools(){
  document.querySelectorAll('#schools-modal select[data-school]').forEach(function(s){s.value=''});
  updateSchoolsBadge();
}
function setSchoolMode(mode){
  schoolMode=mode;
  document.getElementById('school-mode-and').classList.toggle('active',mode==='and');
  document.getElementById('school-mode-or').classList.toggle('active',mode==='or');
}
function getSchoolFilters(){
  var parts=[];
  document.querySelectorAll('#schools-modal select[data-school]').forEach(function(s){
    if(s.value){parts.push(s.getAttribute('data-school')+':'+s.value)}
  });
  return parts.join(',');
}
function updateSchoolsBadge(){
  var count=0;
  document.querySelectorAll('#schools-modal select[data-school]').forEach(function(s){if(s.value)count++});
  var badge=document.getElementById('schools-badge');
  if(count>0){badge.textContent=count;badge.style.display='inline'}
  else{badge.style.display='none'}
}
document.getElementById('schools-modal').addEventListener('click',function(e){
  if(e.target===this)closeSchoolsModal();
});
function doSearch(){
  var p=new URLSearchParams();
  var c=document.getElementById('char-input').value;if(c)p.set('character',c);
  var r=document.getElementById('realm-select').value;if(r)p.set('realm',r);
  var t=document.getElementById('tier-select').value;if(t)p.set('tier',t);
  var e=document.getElementById('en-element-select').value;if(e)p.set('element',e);
  var i=document.getElementById('imperil-select').value;if(i)p.set('imperil',i);
  var eff=getCheckedEffects();if(eff.length>0)p.set('effects',eff.join(','));
  var sch=getSchoolFilters();if(sch)p.set('schools',sch);
  if(schoolMode!=='and')p.set('schoolmode',schoolMode);
  if(document.getElementById('owned-only').checked)p.set('owned','1');
  window.location='/search?'+p.toString();
}
document.getElementById('char-input').addEventListener('keydown',function(e){if(e.key==='Enter')doSearch()});
document.getElementById('login-modal').addEventListener('click',function(e){if(e.target===this)closeAuthModals()});
document.getElementById('register-modal').addEventListener('click',function(e){if(e.target===this)closeAuthModals()});
document.getElementById('login-password').addEventListener('keydown',function(e){if(e.key==='Enter')doLogin()});
document.getElementById('register-password').addEventListener('keydown',function(e){if(e.key==='Enter')doRegister()});
function closeAuthModals(){document.getElementById('login-modal').classList.remove('visible');document.getElementById('register-modal').classList.remove('visible')}
function showLogin(){document.getElementById('login-error').style.display='none';document.getElementById('login-modal').classList.add('visible');document.getElementById('login-username').focus()}
function showRegister(){document.getElementById('register-error').style.display='none';document.getElementById('register-modal').classList.add('visible');document.getElementById('register-username').focus()}
function renderAuth(username){
  var area=document.getElementById('auth-area');
  if(username){area.innerHTML='<span class="welcome">Welcome, '+username+'</span><button onclick="doLogout()">Logout</button>';document.getElementById('owned-filter').style.display=''}
  else{area.innerHTML='<button onclick="showLogin()">Login</button><button onclick="showRegister()">Register</button>';document.getElementById('owned-filter').style.display='none';document.getElementById('owned-only').checked=false}
}
function doLogin(){
  var u=document.getElementById('login-username').value,p=document.getElementById('login-password').value;
  fetch('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:u,password:p})})
  .then(function(r){return r.json().then(function(d){return{ok:r.ok,data:d}})})
  .then(function(res){if(res.ok){closeAuthModals();renderAuth(res.data.username);window.__currentUser=res.data.username;if(typeof onAuthChange==='function')onAuthChange()}
    else{var e=document.getElementById('login-error');e.textContent=res.data.error||'Login failed';e.style.display='block'}});
}
function doRegister(){
  var u=document.getElementById('register-username').value,p=document.getElementById('register-password').value;
  fetch('/api/register',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:u,password:p})})
  .then(function(r){return r.json().then(function(d){return{ok:r.ok,data:d}})})
  .then(function(res){if(res.ok){closeAuthModals();renderAuth(res.data.username);window.__currentUser=res.data.username;if(typeof onAuthChange==='function')onAuthChange()}
    else{var e=document.getElementById('register-error');e.textContent=res.data.error||'Registration failed';e.style.display='block'}});
}
function doLogout(){
  fetch('/api/logout',{method:'POST'}).then(function(){renderAuth(null);window.__currentUser=null;if(typeof onAuthChange==='function')onAuthChange()});
}
fetch('/api/me').then(function(r){if(r.ok)return r.json();return null}).then(function(d){if(d&&d.username){renderAuth(d.username);window.__currentUser=d.username}else{renderAuth(null)}});
(function(){
  var p=new URLSearchParams(window.location.search);
  if(p.get('character'))document.getElementById('char-input').value=p.get('character');
  if(p.get('realm'))document.getElementById('realm-select').value=p.get('realm');
  if(p.get('element'))document.getElementById('en-element-select').value=p.get('element');
  if(p.get('imperil'))document.getElementById('imperil-select').value=p.get('imperil');
  if(p.get('effects')){
    var eff=p.get('effects').split(',');
    eff.forEach(function(v){
      var cb=document.querySelector('#effects-modal input[value="'+v+'"]');
      if(cb)cb.checked=true;
    });
    updateBadge();
  }
  if(p.get('owned')==='1')document.getElementById('owned-only').checked=true;
  if(p.get('schools')){
    p.get('schools').split(',').forEach(function(pair){
      var parts=pair.split(':');if(parts.length===2){
        var sel=document.querySelector('#schools-modal select[data-school="'+parts[0]+'"]');
        if(sel)sel.value=parts[1];
      }
    });
    updateSchoolsBadge();
  }
  if(p.get('schoolmode')==='or')setSchoolMode('or');
})();
</script>
`

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
       background: #1a1a2e; color: #e0e0e0; padding: 70px 20px 20px; }
` + searchBarCSS + `
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
` + searchBarHTML + `
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
       background: #1a1a2e; color: #e0e0e0; padding: 70px 20px 20px; max-width: 1200px; margin: 0 auto;
       font-size: 16px; }
` + searchBarCSS + `
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
td.sb-owned { width: 24px; padding: 4px; text-align: center; }
td.sb-owned input[type="checkbox"] { accent-color: #e94560; cursor: pointer; width: 16px; height: 16px; }
</style>
</head><body>
` + searchBarHTML + `
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
<tr>{{if $.LoggedIn}}<th></th>{{end}}<th></th><th></th><th>Name</th><th>Tier</th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
{{range .SoulBreaks}}
<tr class="sb-row{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .ArcaneDyad .BraveCommand}} expandable{{end}}" onclick="toggleDetail(this)">
  {{if $.LoggedIn}}<td class="sb-owned" onclick="event.stopPropagation()"><input type="checkbox" data-sbid="{{.ID}}" onchange="toggleOwned(this)"{{if index $.OwnedSoulbreaks .ID}} checked{{end}}></td>{{end}}
  <td class="sb-chevron">{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .ArcaneDyad .BraveCommand}}<span class="chevron">&#9654;</span>{{end}}</td>
  <td>{{if .Img}}<span class="sb-icon"><img src="{{.Img}}"></span>{{end}}</td>
  <td>{{.Name}}</td>
  <td>{{.Tier}}</td>
  <td>{{.Type}}</td>
  <td>{{.Element}}</td>
  <td>{{.Time}}</td>
  <td class="effects">{{.Effects}}</td>
</tr>
{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .ArcaneDyad .BraveCommand}}
<tr class="sb-detail">
  <td colspan="{{if $.LoggedIn}}9{{else}}8{{end}}">
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
    {{if .ArcaneDyad}}
    <div class="burst-commands">
      <div class="burst-title">Arcane Dyad Finisher</div>
      <table class="burst-table">
        <tr><th></th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        <tr class="bc-row{{if .ArcaneDyad.MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .ArcaneDyad.MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td>{{.ArcaneDyad.Type}}</td>
          <td>{{.ArcaneDyad.Element}}</td>
          <td>{{.ArcaneDyad.Time}}</td>
          <td class="effects">{{.ArcaneDyad.Effects}}</td>
        </tr>
        {{if .ArcaneDyad.MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="5">
            <ul class="effect-list">
            {{range .ArcaneDyad.MatchedEffects}}
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
function toggleOwned(cb){
  var id=cb.getAttribute('data-sbid'),owned=cb.checked;
  fetch('/api/user/soulbreaks',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({soulbreak_id:id,owned:owned})})
  .then(function(r){if(!r.ok){cb.checked=!owned}});
}
function onAuthChange(){window.location.reload()}
</script>
</body></html>`))

type SearchData struct {
	Results         []SearchResult
	Truncated       bool
	MaxResults      int
	LoggedIn        bool
	OwnedSoulbreaks map[string]bool
	Query           struct {
		Character  string
		Realm      string
		Tier       string
		Element    string
		Imperil    string
		Schools    string
		SchoolMode string
	}
}

var searchTmpl = template.Must(template.New("search").Funcs(funcMap).Parse(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>Search - FFRK</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
       background: #1a1a2e; color: #e0e0e0; padding: 70px 20px 20px; max-width: 1200px; margin: 0 auto;
       font-size: 16px; }
` + searchBarCSS + `
a { color: #e94560; }
h1 { color: #e94560; margin-bottom: 16px; }
h2 { color: #0f3460; background: #e94560; display: inline-block;
     padding: 4px 16px; border-radius: 4px; margin: 20px 0 12px; font-size: 1em; }
.char-header { display: flex; align-items: center; gap: 12px; margin: 24px 0 8px; }
.char-header img { width: 48px; height: 48px; object-fit: contain; }
.char-header .name { color: #e94560; font-size: 1.2em; font-weight: bold; }
.char-header .realm { color: #888; font-size: 0.9em; margin-left: 8px; }
.warning { background: #4a2020; border: 1px solid #e94560; border-radius: 4px;
           padding: 8px 16px; margin-bottom: 16px; color: #e0e0e0; }
.no-results { color: #888; font-size: 1.1em; margin-top: 24px; }
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
td.sb-owned { width: 24px; padding: 4px; text-align: center; }
td.sb-owned input[type="checkbox"] { accent-color: #e94560; cursor: pointer; width: 16px; height: 16px; }
</style>
</head><body>
` + searchBarHTML + `
<h1>Search Results</h1>
{{if .Truncated}}<div class="warning">Results truncated to {{.MaxResults}} items. Try narrowing your search.</div>{{end}}
{{if not .Results}}<div class="no-results">No results found.</div>{{end}}
{{range .Results}}
<div class="char-header">
  {{if .Character.Img}}<img src="{{.Character.Img}}" alt="{{.Character.Name}}">{{end}}
  <div><a href="/char/{{.Character.ID}}" class="name">{{.Character.Name}}</a><span class="realm">{{.Character.Realm}}</span></div>
</div>

{{if .HeroAbilities}}
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
<table>
<tr>{{if $.LoggedIn}}<th></th>{{end}}<th></th><th></th><th>Name</th><th>Tier</th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
{{range .SoulBreaks}}
<tr class="sb-row{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .ArcaneDyad .BraveCommand}} expandable{{end}}" onclick="toggleDetail(this)">
  {{if $.LoggedIn}}<td class="sb-owned" onclick="event.stopPropagation()"><input type="checkbox" data-sbid="{{.ID}}" onchange="toggleOwned(this)"{{if index $.OwnedSoulbreaks .ID}} checked{{end}}></td>{{end}}
  <td class="sb-chevron">{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .ArcaneDyad .BraveCommand}}<span class="chevron">&#9654;</span>{{end}}</td>
  <td>{{if .Img}}<span class="sb-icon"><img src="{{.Img}}"></span>{{end}}</td>
  <td>{{.Name}}</td>
  <td>{{.Tier}}</td>
  <td>{{.Type}}</td>
  <td>{{.Element}}</td>
  <td>{{.Time}}</td>
  <td class="effects">{{.Effects}}</td>
</tr>
{{if or .MatchedEffects .BurstCommands .SynchroAbilities .ZenithAbilities .DualShift .ArcaneDyad .BraveCommand}}
<tr class="sb-detail">
  <td colspan="{{if $.LoggedIn}}9{{else}}8{{end}}">
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
    {{if .ArcaneDyad}}
    <div class="burst-commands">
      <div class="burst-title">Arcane Dyad Finisher</div>
      <table class="burst-table">
        <tr><th></th><th>Type</th><th>Element</th><th>Time</th><th>Effects</th></tr>
        <tr class="bc-row{{if .ArcaneDyad.MatchedEffects}} expandable{{end}}" onclick="toggleBcDetail(this)">
          <td class="bc-chevron">{{if .ArcaneDyad.MatchedEffects}}<span class="chevron">&#9654;</span>{{end}}</td>
          <td>{{.ArcaneDyad.Type}}</td>
          <td>{{.ArcaneDyad.Element}}</td>
          <td>{{.ArcaneDyad.Time}}</td>
          <td class="effects">{{.ArcaneDyad.Effects}}</td>
        </tr>
        {{if .ArcaneDyad.MatchedEffects}}
        <tr class="bc-detail">
          <td colspan="5">
            <ul class="effect-list">
            {{range .ArcaneDyad.MatchedEffects}}
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
function toggleOwned(cb){
  var id=cb.getAttribute('data-sbid'),owned=cb.checked;
  fetch('/api/user/soulbreaks',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({soulbreak_id:id,owned:owned})})
  .then(function(r){if(!r.ok){cb.checked=!owned}});
}
function onAuthChange(){window.location.reload()}
</script>
</body></html>`))

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

// ---------- additional effect matching ----------

// containsCI performs a case-insensitive substring check.
func containsCI(text, substr string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(substr))
}

var critChanceRe = regexp.MustCompile(`(?i)\d+% critical[\]\s\d]`)
var critDamageRe = regexp.MustCompile(`(?i)critical damage \+\d+%`)
var sbGaugeRe = regexp.MustCompile(`(?i)soul break gauge \+`)
var atbSpeedRe = regexp.MustCompile(`(?i)\d+% atb`)
var aegisCounterRe = regexp.MustCompile(`(?i)def, res and mnd -\d+%`)
var fullbreakCounterRe = regexp.MustCompile(`(?i)atk, def, mag and res \+\d+%`)
var physJobBreakCounterRe = regexp.MustCompile(`(?i)atk and mnd \+\d+%, def and res \+\d+%`)
var magJobBreakCounterRe = regexp.MustCompile(`(?i)mag and mnd \+\d+%, def and res \+\d+%`)
var weaknessBoostRe = regexp.MustCompile(`(?i)weakness \+\d+%`)
var magicalBoostRe = regexp.MustCompile(`(?i)magical \+\d+%`)
var phyBoostRe = regexp.MustCompile(`(?i)phy \+\d+%`)
var sorceryBoostRe = regexp.MustCompile(`(?i)sorcery damage \+\d+%`)
var pentabreakBoostRe = regexp.MustCompile(`(?i)pentabreak damage boost`)

// effectCheckers maps effect filter keys to functions that check if a text matches.
var effectCheckers = map[string]func(string) bool{
	"aegis_counter": func(text string) bool {
		return aegisCounterRe.MatchString(text)
	},
	"fullbreak_counter": func(text string) bool {
		return fullbreakCounterRe.MatchString(text)
	},
	"phys_job_break_counter": func(text string) bool {
		return physJobBreakCounterRe.MatchString(text)
	},
	"mag_job_break_counter": func(text string) bool {
		return magJobBreakCounterRe.MatchString(text)
	},
	"haste": func(text string) bool {
		return containsCI(text, "[Haste]") || containsCI(text, "Haste]")
	},
	"protect": func(text string) bool {
		return containsCI(text, "[Protect]") || containsCI(text, "Protect]")
	},
	"shell": func(text string) bool {
		return containsCI(text, "[Shell]") || containsCI(text, "Shell]")
	},
	"last_stand": func(text string) bool {
		return containsCI(text, "Last Stand")
	},
	"regen": func(text string) bool {
		return containsCI(text, "[Regen]") || containsCI(text, "[High Regen]") ||
			containsCI(text, "Regen]") || containsCI(text, "High Regen")
	},
	"regenga": func(text string) bool {
		return containsCI(text, "Regenga")
	},
	"astra": func(text string) bool {
		return containsCI(text, "Astra")
	},
	"crit_chance": func(text string) bool {
		return critChanceRe.MatchString(text)
	},
	"crit_damage": func(text string) bool {
		return critDamageRe.MatchString(text)
	},
	"sb_gauge": func(text string) bool {
		return sbGaugeRe.MatchString(text)
	},
	"dualcast": func(text string) bool {
		return containsCI(text, "Dualcast")
	},
	"triplecast": func(text string) bool {
		return containsCI(text, "Triplecast")
	},
	"instant_atb": func(text string) bool {
		return containsCI(text, "Instant ATB")
	},
	"atb_speed": func(text string) bool {
		return atbSpeedRe.MatchString(text)
	},
	"weakness_boost": func(text string) bool {
		return weaknessBoostRe.MatchString(text)
	},
	"magical_boost": func(text string) bool {
		return magicalBoostRe.MatchString(text)
	},
	"phy_boost": func(text string) bool {
		return phyBoostRe.MatchString(text)
	},
	"sorcery_boost": func(text string) bool {
		return sorceryBoostRe.MatchString(text)
	},
	"pentabreak_boost": func(text string) bool {
		return pentabreakBoostRe.MatchString(text)
	},
	"deshell": func(text string) bool {
		return containsCI(text, "Deshell")
	},
	"deprotect": func(text string) bool {
		return containsCI(text, "Deprotect")
	},
}

// collectAllText gathers the SB's effects text, its sub-ability effects, and
// all matched status effect descriptions into a single combined string.
func collectSBTexts(sb SoulBreak) []string {
	texts := []string{sb.Effects}
	for _, se := range sb.MatchedEffects {
		texts = append(texts, se.Name, se.Description)
	}
	if sb.DualShift != nil {
		texts = append(texts, sb.DualShift.Effects)
		for _, se := range sb.DualShift.MatchedEffects {
			texts = append(texts, se.Name, se.Description)
		}
	}
	if sb.ArcaneDyad != nil {
		texts = append(texts, sb.ArcaneDyad.Effects)
		for _, se := range sb.ArcaneDyad.MatchedEffects {
			texts = append(texts, se.Name, se.Description)
		}
	}
	for _, bc := range sb.BurstCommands {
		texts = append(texts, bc.Effects)
		for _, se := range bc.MatchedEffects {
			texts = append(texts, se.Name, se.Description)
		}
	}
	for _, sa := range sb.SynchroAbilities {
		texts = append(texts, sa.Effects)
		for _, se := range sa.MatchedEffects {
			texts = append(texts, se.Name, se.Description)
		}
	}
	for _, za := range sb.ZenithAbilities {
		texts = append(texts, za.Effects)
		for _, se := range za.MatchedEffects {
			texts = append(texts, se.Name, se.Description)
		}
	}
	if sb.BraveCommand != nil {
		for _, bl := range sb.BraveCommand.Levels {
			texts = append(texts, bl.Effects)
			for _, se := range bl.MatchedEffects {
				texts = append(texts, se.Name, se.Description)
			}
		}
	}
	return texts
}

// sbMatchesAdditionalEffects checks if a soul break matches ALL of the given effect filters.
func sbMatchesAdditionalEffects(sb SoulBreak, effects []string) bool {
	texts := collectSBTexts(sb)
	for _, eff := range effects {
		checker, ok := effectCheckers[eff]
		if !ok {
			continue
		}
		found := false
		for _, t := range texts {
			if checker(t) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// haMatchesAdditionalEffects checks if a hero ability matches ALL of the given effect filters.
func haMatchesAdditionalEffects(ha HeroAbility, effects []string) bool {
	for _, eff := range effects {
		checker, ok := effectCheckers[eff]
		if !ok {
			continue
		}
		if !checker(ha.Effects) {
			return false
		}
	}
	return true
}

// textContainsAttach checks if text contains "Attach <element>" pattern
func textContainsAttach(text, element string) bool {
	return containsCI(text, "Attach "+element)
}

// textContainsImperil checks if text contains "Imperil <element>" or "Imperil Prismatic"
func textContainsImperil(text, element string) bool {
	if containsCI(text, "Imperil "+element) {
		return true
	}
	// "Imperil Prismatic" matches any specific element
	if element != "Prismatic" && containsCI(text, "Imperil Prismatic") {
		return true
	}
	return false
}

// sbMatchesElement checks if a soul break (including sub-abilities) matches en-element criteria
func sbMatchesElement(sb SoulBreak, element string) bool {
	if textContainsAttach(sb.Effects, element) {
		return true
	}
	if sb.DualShift != nil && textContainsAttach(sb.DualShift.Effects, element) {
		return true
	}
	if sb.ArcaneDyad != nil && textContainsAttach(sb.ArcaneDyad.Effects, element) {
		return true
	}
	for _, bc := range sb.BurstCommands {
		if textContainsAttach(bc.Effects, element) {
			return true
		}
	}
	for _, sa := range sb.SynchroAbilities {
		if textContainsAttach(sa.Effects, element) {
			return true
		}
	}
	for _, za := range sb.ZenithAbilities {
		if textContainsAttach(za.Effects, element) {
			return true
		}
	}
	return false
}

// sbMatchesImperil checks if a soul break (including sub-abilities) matches imperil criteria
func sbMatchesImperil(sb SoulBreak, element string) bool {
	if textContainsImperil(sb.Effects, element) {
		return true
	}
	if sb.DualShift != nil && textContainsImperil(sb.DualShift.Effects, element) {
		return true
	}
	if sb.ArcaneDyad != nil && textContainsImperil(sb.ArcaneDyad.Effects, element) {
		return true
	}
	for _, bc := range sb.BurstCommands {
		if textContainsImperil(bc.Effects, element) {
			return true
		}
	}
	for _, sa := range sb.SynchroAbilities {
		if textContainsImperil(sa.Effects, element) {
			return true
		}
	}
	for _, za := range sb.ZenithAbilities {
		if textContainsImperil(za.Effects, element) {
			return true
		}
	}
	return false
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	dataLock.RLock()
	defer dataLock.RUnlock()
	charFilter := r.URL.Query().Get("character")
	realmFilter := r.URL.Query().Get("realm")
	tierFilter := r.URL.Query().Get("tier")
	elementFilter := r.URL.Query().Get("element")
	imperilFilter := r.URL.Query().Get("imperil")
	effectsParam := r.URL.Query().Get("effects")
	schoolsParam := r.URL.Query().Get("schools")
	schoolModeParam := r.URL.Query().Get("schoolmode")
	if schoolModeParam == "" {
		schoolModeParam = "and"
	}
	ownedOnly := r.URL.Query().Get("owned") == "1"

	var additionalEffects []string
	if effectsParam != "" {
		for _, e := range strings.Split(effectsParam, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				additionalEffects = append(additionalEffects, e)
			}
		}
	}

	type schoolReq struct {
		School   string
		MinLevel int
	}
	var schoolReqs []schoolReq
	if schoolsParam != "" {
		for _, pair := range strings.Split(schoolsParam, ",") {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) == 2 {
				level, err := strconv.Atoi(parts[1])
				if err == nil && level >= 1 && level <= 6 {
					schoolReqs = append(schoolReqs, schoolReq{School: parts[0], MinLevel: level})
				}
			}
		}
	}

	hasEffectFilter := elementFilter != "" || imperilFilter != "" || len(additionalEffects) > 0
	hasSBFilter := tierFilter != "" || hasEffectFilter

	// Get owned soulbreaks for filtering
	u := getCurrentUser(r)
	var ownedSet map[string]bool
	if u != nil {
		userLock.RLock()
		ownedSet = userSoulbreaks[u.ID]
		userLock.RUnlock()
	}
	if ownedOnly && (u == nil || ownedSet == nil) {
		ownedOnly = false
	}

	// Filter characters
	var matchedChars []Character
	for _, c := range characters {
		if charFilter != "" && !strings.Contains(strings.ToLower(c.Name), strings.ToLower(charFilter)) {
			continue
		}
		if realmFilter != "" && c.Realm != realmFilter {
			continue
		}
		if len(schoolReqs) > 0 {
			match := schoolModeParam == "and"
			for _, req := range schoolReqs {
				has := c.Schools[req.School] >= req.MinLevel
				if schoolModeParam == "and" {
					if !has {
						match = false
						break
					}
				} else {
					if has {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}
		matchedChars = append(matchedChars, c)
	}

	sort.Slice(matchedChars, func(i, j int) bool {
		return matchedChars[i].Name < matchedChars[j].Name
	})

	var results []SearchResult
	totalCount := 0
	truncated := false
	const maxResults = 300

	for _, c := range matchedChars {
		if truncated {
			break
		}

		// Collect matching soul breaks
		var matchedSBs []SoulBreak
		for _, sb := range soulBreaks[c.Name] {
			if ownedOnly && !ownedSet[sb.ID] {
				continue
			}
			if tierFilter != "" && sb.Tier != tierFilter {
				continue
			}
			if elementFilter != "" && !sbMatchesElement(sb, elementFilter) {
				continue
			}
			if imperilFilter != "" && !sbMatchesImperil(sb, imperilFilter) {
				continue
			}
			if len(additionalEffects) > 0 && !sbMatchesAdditionalEffects(sb, additionalEffects) {
				continue
			}
			matchedSBs = append(matchedSBs, sb)
			totalCount++
			if totalCount >= maxResults {
				truncated = true
				break
			}
		}

		// Collect hero abilities:
		// - No SB filters: include all HAs for the character
		// - SB filters active: only include HAs that match effect filters,
		//   and only if the character also has matching soul breaks
		var matchedHAs []HeroAbility
		if !truncated && !hasSBFilter {
			for _, ha := range heroAbilities[c.Name] {
				matchedHAs = append(matchedHAs, ha)
				totalCount++
				if totalCount >= maxResults {
					truncated = true
					break
				}
			}
		} else if !truncated && len(matchedSBs) > 0 && hasEffectFilter {
			for _, ha := range heroAbilities[c.Name] {
				match := false
				if elementFilter != "" && textContainsAttach(ha.Effects, elementFilter) {
					match = true
				}
				if imperilFilter != "" && textContainsImperil(ha.Effects, imperilFilter) {
					match = true
				}
				if len(additionalEffects) > 0 && haMatchesAdditionalEffects(ha, additionalEffects) {
					match = true
				}
				if !match {
					continue
				}
				matchedHAs = append(matchedHAs, ha)
				totalCount++
				if totalCount >= maxResults {
					truncated = true
					break
				}
			}
		}

		if len(matchedSBs) > 0 || len(matchedHAs) > 0 {
			results = append(results, SearchResult{
				Character:     c,
				SoulBreaks:    matchedSBs,
				HeroAbilities: matchedHAs,
			})
		}
	}

	data := SearchData{
		Results:    results,
		Truncated:  truncated,
		MaxResults: maxResults,
	}
	data.Query.Character = charFilter
	data.Query.Realm = realmFilter
	data.Query.Tier = tierFilter
	data.Query.Element = elementFilter
	data.Query.Imperil = imperilFilter
	data.Query.Schools = schoolsParam
	data.Query.SchoolMode = schoolModeParam

	if u != nil {
		data.LoggedIn = true
		if ownedSet == nil {
			ownedSet = make(map[string]bool)
		}
		data.OwnedSoulbreaks = ownedSet
	} else {
		data.OwnedSoulbreaks = make(map[string]bool)
	}

	searchTmpl.Execute(w, data)
}

func charHandler(w http.ResponseWriter, r *http.Request) {
	dataLock.RLock()
	defer dataLock.RUnlock()
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

	u := getCurrentUser(r)
	if u != nil {
		detail.LoggedIn = true
		userLock.RLock()
		owned := userSoulbreaks[u.ID]
		if owned == nil {
			owned = make(map[string]bool)
		}
		detail.OwnedSoulbreaks = owned
		userLock.RUnlock()
	} else {
		detail.OwnedSoulbreaks = make(map[string]bool)
	}

	charTmpl.Execute(w, detail)
}

// ---------- main ----------

func main() {
	log.Println("Loading data...")
	reloadData()

	log.Println("Loading user data...")
	initUserData()

	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		for range ticker.C {
			log.Println("Auto-updating CSVs from Google Sheets...")
			updateCSVs()
		}
	}()

	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/char/", charHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/api/characters", characterAPIHandler)
	http.HandleFunc("/api/tiers", tierAPIHandler)
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/register", registerHandler)
	http.HandleFunc("/api/logout", logoutHandler)
	http.HandleFunc("/api/me", meHandler)
	http.HandleFunc("/api/user/soulbreaks", userSoulbreaksHandler)

	addr := "0.0.0.0:9090"
	fmt.Printf("Server running at http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
