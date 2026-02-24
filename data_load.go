package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func reloadData() error {
	next, err := buildAppData()
	if err != nil {
		return err
	}

	dataLock.Lock()
	appData = next
	dataLock.Unlock()

	log.Printf("Reloaded %d characters", len(next.Characters))
	return nil
}

func buildAppData() (*AppData, error) {
	return buildAppDataFromCSVDir(csvDir)
}

func buildAppDataFromCSVDir(dir string) (*AppData, error) {
	d := &AppData{csvDir: dir}
	if err := d.loadCharacters(); err != nil {
		return nil, fmt.Errorf("load characters: %w", err)
	}
	if err := d.applyRecordSphereUpgrades(); err != nil {
		return nil, fmt.Errorf("apply record sphere upgrades: %w", err)
	}
	if err := d.loadSoulBreaks(); err != nil {
		return nil, fmt.Errorf("load soul breaks: %w", err)
	}
	if err := d.loadHeroAbilities(); err != nil {
		return nil, fmt.Errorf("load hero abilities: %w", err)
	}
	if err := d.loadBurstCommands(); err != nil {
		return nil, fmt.Errorf("load burst commands: %w", err)
	}
	if err := d.loadSynchroAbilities(); err != nil {
		return nil, fmt.Errorf("load synchro abilities: %w", err)
	}
	if err := d.loadZenithAbilities(); err != nil {
		return nil, fmt.Errorf("load zenith abilities: %w", err)
	}
	if err := d.loadBraveCommands(); err != nil {
		return nil, fmt.Errorf("load brave commands: %w", err)
	}
	if err := d.loadStatuses(); err != nil {
		return nil, fmt.Errorf("load statuses: %w", err)
	}
	d.matchSoulBreakEffects()
	d.pairDualShifts()
	d.pairArcaneDyads()
	d.matchBurstCommands()
	d.matchSynchroAbilities()
	d.matchZenithAbilities()
	d.matchBraveCommands()
	d.cacheAllImages()
	d.buildRealmGroups()
	return d, nil
}

// ---------- CSV loading ----------

func readCSVResolved(resolved string) ([]map[string]string, error) {
	f, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", resolved, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", resolved, err)
	}
	if len(records) < 2 {
		return nil, nil
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
	return rows, nil
}

func readCSV(path string) ([]map[string]string, error) {
	return readCSVResolved(csvPath(path))
}

func (d *AppData) csvPath(name string) string {
	if d != nil && d.csvDir != "" {
		return filepath.Join(d.csvDir, name)
	}
	return csvPath(name)
}

func (d *AppData) readCSV(path string) ([]map[string]string, error) {
	return readCSVResolved(d.csvPath(path))
}

var schoolNames = []string{
	"Black Magic", "White Magic", "Combat", "Support", "Celerity",
	"Summoning", "Spellblade", "Dragoon", "Monk", "Thief",
	"Knight", "Samurai", "Ninja", "Bard", "Dancer",
	"Machinist", "Darkness", "Sharpshooter", "Witch", "Heavy",
}

func (d *AppData) loadCharacters() error {
	rows, err := d.readCSV("Characters.csv")
	if err != nil {
		return err
	}
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
		d.Characters = append(d.Characters, c)
	}
	return nil
}

var recordSphereUpgradeRE = regexp.MustCompile(`^(.+?)\s+(\d)★\s*->\s*(\d)★$`)

func (d *AppData) applyRecordSphereUpgrades() error {
	recordSpheresPath := d.csvPath("Record-Spheres.csv")
	if _, err := os.Stat(recordSpheresPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", recordSpheresPath, err)
	}
	rows, err := d.readCSV("Record-Spheres.csv")
	if err != nil {
		return err
	}

	// Build name -> index lookup for d.Characters
	charIndex := make(map[string]int, len(d.Characters))
	for i := range d.Characters {
		charIndex[d.Characters[i].Name] = i
	}

	lvCols := []string{"S. Lv 1", "S. Lv 2", "S. Lv 3", "S. Lv 4", "S. Lv 5"}
	upgrades := 0

	for _, row := range rows {
		name := row["Character"]
		ci, ok := charIndex[name]
		if !ok {
			continue
		}
		c := &d.Characters[ci]
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
	return nil
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

func (d *AppData) loadSoulBreaks() error {
	d.SoulBreaks = make(map[string][]SoulBreak)
	tierSet := make(map[string]bool)
	rows, err := d.readCSV("Soul-Breaks.csv")
	if err != nil {
		return err
	}
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
		d.SoulBreaks[sb.Character] = append(d.SoulBreaks[sb.Character], sb)
		if isValidTier(sb.Tier) {
			tierSet[sb.Tier] = true
		}
	}
	d.TierNames = nil
	for t := range tierSet {
		d.TierNames = append(d.TierNames, t)
	}
	sort.Strings(d.TierNames)
	return nil
}

func (d *AppData) loadHeroAbilities() error {
	d.HeroAbilities = make(map[string][]HeroAbility)
	rows, err := d.readCSV("Hero-Abilities.csv")
	if err != nil {
		return err
	}
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
			SB:        row["SB"],
			ID:        row["ID"],
		}
		d.HeroAbilities[ha.Character] = append(d.HeroAbilities[ha.Character], ha)
	}
	return nil
}

func sbGroupKey(character, source string) string {
	return character + "|" + source
}

func cloneWithMatchedEffects[T any](
	d *AppData,
	src []T,
	effects func(T) string,
	setEffects func(*T, string),
	setMatched func(*T, []StatusEffect),
) []T {
	matched := make([]T, len(src))
	copy(matched, src)
	for i := range matched {
		text, statuses := d.matchAndBracketEffectsInText(effects(matched[i]))
		setEffects(&matched[i], text)
		setMatched(&matched[i], statuses)
	}
	return matched
}

func attachTieredChildSlice[T any](
	d *AppData,
	tier string,
	groups map[string][]T,
	effects func(T) string,
	setEffects func(*T, string),
	setMatched func(*T, []StatusEffect),
	assign func(*SoulBreak, []T),
) {
	for name, sbList := range d.SoulBreaks {
		for i := range sbList {
			sb := &sbList[i]
			if sb.Tier != tier {
				continue
			}
			key := sbGroupKey(sb.Character, sb.Name)
			if children, ok := groups[key]; ok {
				assign(sb, cloneWithMatchedEffects(d, children, effects, setEffects, setMatched))
			}
		}
		d.SoulBreaks[name] = sbList
	}
}

func (d *AppData) cloneMatchedBraveCommand(src *BraveCommand) *BraveCommand {
	if src == nil {
		return nil
	}
	matched := *src
	matched.Levels = make([]BraveLevel, len(src.Levels))
	copy(matched.Levels, src.Levels)
	for i := range matched.Levels {
		matched.Levels[i].Effects, matched.Levels[i].MatchedEffects = d.matchAndBracketEffectsInText(matched.Levels[i].Effects)
	}
	return &matched
}

func (d *AppData) loadBurstCommands() error {
	d.BurstCommands = make(map[string][]BurstCommand)
	rows, err := d.readCSV("Burst.csv")
	if err != nil {
		return err
	}
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
		key := sbGroupKey(row["Character"], row["Source"])
		d.BurstCommands[key] = append(d.BurstCommands[key], bc)
	}
	// Sort each group so ID ending in '1' comes before '2'
	for key, cmds := range d.BurstCommands {
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].ID < cmds[j].ID
		})
		d.BurstCommands[key] = cmds
	}
	log.Printf("Loaded %d burst command groups", len(d.BurstCommands))
	return nil
}

func (d *AppData) matchBurstCommands() {
	attachTieredChildSlice(
		d,
		"BSB",
		d.BurstCommands,
		func(bc BurstCommand) string { return bc.Effects },
		func(bc *BurstCommand, effects string) { bc.Effects = effects },
		func(bc *BurstCommand, matched []StatusEffect) { bc.MatchedEffects = matched },
		func(sb *SoulBreak, matched []BurstCommand) { sb.BurstCommands = matched },
	)
}

func (d *AppData) loadSynchroAbilities() error {
	d.SynchroAbilities = make(map[string][]SynchroAbility)
	rows, err := d.readCSV("Synchro.csv")
	if err != nil {
		return err
	}
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
		key := sbGroupKey(row["Character"], row["Source"])
		d.SynchroAbilities[key] = append(d.SynchroAbilities[key], sa)
	}
	for key, abs := range d.SynchroAbilities {
		sort.Slice(abs, func(i, j int) bool {
			return abs[i].Slot < abs[j].Slot
		})
		d.SynchroAbilities[key] = abs
	}
	log.Printf("Loaded %d synchro ability groups", len(d.SynchroAbilities))
	return nil
}

func (d *AppData) matchSynchroAbilities() {
	attachTieredChildSlice(
		d,
		"SASB",
		d.SynchroAbilities,
		func(sa SynchroAbility) string { return sa.Effects },
		func(sa *SynchroAbility, effects string) { sa.Effects = effects },
		func(sa *SynchroAbility, matched []StatusEffect) { sa.MatchedEffects = matched },
		func(sb *SoulBreak, matched []SynchroAbility) { sb.SynchroAbilities = matched },
	)
}

func (d *AppData) loadZenithAbilities() error {
	d.ZenithAbilities = make(map[string][]ZenithAbility)
	rows, err := d.readCSV("Zenith-SB-Abilities.csv")
	if err != nil {
		return err
	}
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
		key := sbGroupKey(row["Character"], row["Source"])
		d.ZenithAbilities[key] = append(d.ZenithAbilities[key], za)
	}
	log.Printf("Loaded %d zenith ability groups", len(d.ZenithAbilities))
	return nil
}

func (d *AppData) matchZenithAbilities() {
	attachTieredChildSlice(
		d,
		"ZSB",
		d.ZenithAbilities,
		func(za ZenithAbility) string { return za.Effects },
		func(za *ZenithAbility, effects string) { za.Effects = effects },
		func(za *ZenithAbility, matched []StatusEffect) { za.MatchedEffects = matched },
		func(sb *SoulBreak, matched []ZenithAbility) { sb.ZenithAbilities = matched },
	)
}

func (d *AppData) loadBraveCommands() error {
	d.BraveCommands = make(map[string]*BraveCommand)
	rows, err := d.readCSV("Brave.csv")
	if err != nil {
		return err
	}
	for _, row := range rows {
		key := sbGroupKey(row["Character"], row["Source"])
		bc, ok := d.BraveCommands[key]
		if !ok {
			bc = &BraveCommand{
				Name:      row["Name"],
				School:    row["School"],
				Condition: row["Brave Condition"],
			}
			d.BraveCommands[key] = bc
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
	for _, bc := range d.BraveCommands {
		sort.Slice(bc.Levels, func(i, j int) bool {
			return bc.Levels[i].Level < bc.Levels[j].Level
		})
	}
	log.Printf("Loaded %d brave commands", len(d.BraveCommands))
	return nil
}

func (d *AppData) matchBraveCommands() {
	for name, sbList := range d.SoulBreaks {
		for i := range sbList {
			if !strings.Contains(sbList[i].Effects, "[Brave Mode]") {
				continue
			}
			key := sbGroupKey(sbList[i].Character, sbList[i].Name)
			if bc, ok := d.BraveCommands[key]; ok {
				sbList[i].BraveCommand = d.cloneMatchedBraveCommand(bc)
			}
		}
		d.SoulBreaks[name] = sbList
	}
}

func (d *AppData) loadStatuses() error {
	d.StatusEffects = make(map[string]StatusEffect)
	rows, err := d.readCSV("Status.csv")
	if err != nil {
		return err
	}
	for _, row := range rows {
		name := strings.TrimSpace(row["Common Name"])
		if name == "" {
			continue
		}
		d.StatusEffects[name] = StatusEffect{
			Name:        name,
			Description: strings.TrimSpace(row["Effects"]),
			Duration:    strings.TrimSpace(row["Default Duration"]),
		}
	}
	d.statusMatchTerms = make([]string, 0, len(d.StatusEffects))
	for name := range d.StatusEffects {
		d.statusMatchTerms = append(d.statusMatchTerms, name)
	}
	sort.Slice(d.statusMatchTerms, func(i, j int) bool {
		if len(d.statusMatchTerms[i]) != len(d.statusMatchTerms[j]) {
			return len(d.statusMatchTerms[i]) > len(d.statusMatchTerms[j])
		}
		return d.statusMatchTerms[i] < d.statusMatchTerms[j]
	})
	log.Printf("Loaded %d status effects", len(d.StatusEffects))
	return nil
}
