package main

import (
	"encoding/csv"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func reloadData() {
	dataLock.Lock()
	defer dataLock.Unlock()

	characters = nil
	realmGroups = nil

	loadCharacters()
	applyRecordSphereUpgrades()
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
	resolved := csvPath(path)
	f, err := os.Open(resolved)
	if err != nil {
		log.Fatalf("open %s: %v", resolved, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("read %s: %v", resolved, err)
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
					if n == 5 {
						n = 6
					}
					c.Schools[s] = n
				}
			}
		}
		characters = append(characters, c)
	}
}

var recordSphereUpgradeRE = regexp.MustCompile(`^(.+?)\s+(\d)★\s*->\s*(\d)★$`)

func applyRecordSphereUpgrades() {
	if _, err := os.Stat(csvPath("Record-Spheres.csv")); err != nil {
		return
	}
	rows := mustReadCSV("Record-Spheres.csv")

	// Build name -> index lookup for characters
	charIndex := make(map[string]int, len(characters))
	for i := range characters {
		charIndex[characters[i].Name] = i
	}

	lvCols := []string{"S. Lv 1", "S. Lv 2", "S. Lv 3", "S. Lv 4", "S. Lv 5"}
	upgrades := 0

	for _, row := range rows {
		name := row["Character"]
		ci, ok := charIndex[name]
		if !ok {
			continue
		}
		c := &characters[ci]
		for _, col := range lvCols {
			val := strings.TrimSpace(row[col])
			m := recordSphereUpgradeRE.FindStringSubmatch(val)
			if m == nil {
				continue
			}
			school := m[1]
			newLevel, _ := strconv.Atoi(m[3])
			if newLevel == 5 {
				newLevel = 6
			}
			if c.Schools[school] < newLevel {
				c.Schools[school] = newLevel
				upgrades++
			}
		}
	}
	log.Printf("Applied %d record sphere school upgrades", upgrades)
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
			Name:        row["Name"],
			Slot:        row["Synchro Ability Slot"],
			Condition:   row["Synchro Condition"],
			Type:        row["Type"],
			Target:      row["Target"],
			Element:     row["Element"],
			Time:        row["Time"],
			Effects:     row["Effects"],
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
