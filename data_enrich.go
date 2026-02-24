package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var bracketRe = regexp.MustCompile(`\[([^\]]+)\]`)
var pctRe = regexp.MustCompile(`([+-])\d+%`)
var forSecRe = regexp.MustCompile(`for (\d+(?:\.\d+)?) seconds`)

type statusTextMatch struct {
	start int
	end   int
	se    StatusEffect
	wrap  bool
}

func (d *AppData) lookupStatus(term string) (StatusEffect, bool) {
	// Try exact match first
	if se, ok := d.StatusEffects[term]; ok {
		return se, true
	}
	// Fallback: replace specific percentages (+30%, -50%) with +X%/-X%
	generic := pctRe.ReplaceAllString(term, "${1}X%")
	if generic != term {
		if se, ok := d.StatusEffects[generic]; ok {
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

func fillStatusDurationFromFollowing(text string, end int, durLocs [][]int, se StatusEffect) StatusEffect {
	if se.Duration != "" && se.Duration != "-" {
		return se
	}
	for _, dloc := range durLocs {
		if dloc[0] >= end {
			se.Duration = text[dloc[2]:dloc[3]] + " seconds"
			break
		}
	}
	return se
}

func markOccupied(occupied []bool, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(occupied) {
		end = len(occupied)
	}
	for i := start; i < end; i++ {
		occupied[i] = true
	}
}

func spanOccupied(occupied []bool, start, end int) bool {
	if start < 0 || end > len(occupied) || start >= end {
		return true
	}
	for i := start; i < end; i++ {
		if occupied[i] {
			return true
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func boundaryOK(text string, start, end int) bool {
	if start < 0 || end > len(text) || start >= end {
		return false
	}
	if start > 0 && isWordByte(text[start-1]) && isWordByte(text[start]) {
		return false
	}
	if end < len(text) && isWordByte(text[end-1]) && isWordByte(text[end]) {
		return false
	}
	return true
}

func dedupeStatusEffects(matches []statusTextMatch) []StatusEffect {
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]StatusEffect, 0, len(matches))
	for _, m := range matches {
		if seen[m.se.Name] {
			continue
		}
		seen[m.se.Name] = true
		out = append(out, m.se)
	}
	return out
}

func wrapMatchedEffectText(text string, matches []statusTextMatch) string {
	if len(matches) == 0 || len(text) == 0 {
		return text
	}
	var wraps []statusTextMatch
	for _, m := range matches {
		if m.wrap {
			wraps = append(wraps, m)
		}
	}
	if len(wraps) == 0 {
		return text
	}

	var b strings.Builder
	// Each wrapped span adds 2 chars.
	b.Grow(len(text) + len(wraps)*2)
	prev := 0
	for _, m := range wraps {
		if m.start < prev || m.end > len(text) || m.start >= m.end {
			continue
		}
		b.WriteString(text[prev:m.start])
		b.WriteByte('[')
		b.WriteString(text[m.start:m.end])
		b.WriteByte(']')
		prev = m.end
	}
	b.WriteString(text[prev:])
	return b.String()
}

// matchAndBracketEffectsInText extracts status effects from text, inferring
// duration from the next "for N seconds" that follows each match when the
// status has no default duration. It preserves existing brackets and adds
// brackets to newly full-text-matched effects outside bracketed regions.
func (d *AppData) matchAndBracketEffectsInText(text string) (string, []StatusEffect) {
	bracketLocs := bracketRe.FindAllStringSubmatchIndex(text, -1)
	durLocs := forSecRe.FindAllStringSubmatchIndex(text, -1)
	occupied := make([]bool, len(text))

	var matches []statusTextMatch
	for _, bloc := range bracketLocs {
		markOccupied(occupied, bloc[0], bloc[1]) // skip full-text matching inside already-bracketed text
		term := text[bloc[2]:bloc[3]]
		se, ok := d.lookupStatus(term)
		if !ok {
			continue
		}
		se = fillStatusDurationFromFollowing(text, bloc[1], durLocs, se)
		matches = append(matches, statusTextMatch{
			start: bloc[0],
			end:   bloc[1],
			se:    se,
		})
	}

	if len(d.statusMatchTerms) == 0 || len(text) == 0 {
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].start != matches[j].start {
				return matches[i].start < matches[j].start
			}
			if matches[i].end != matches[j].end {
				return matches[i].end < matches[j].end
			}
			return matches[i].se.Name < matches[j].se.Name
		})
		return text, dedupeStatusEffects(matches)
	}

	for _, term := range d.statusMatchTerms {
		if term == "" {
			continue
		}
		for off := 0; off < len(text); {
			idx := strings.Index(text[off:], term)
			if idx < 0 {
				break
			}
			start := off + idx
			end := start + len(term)
			if boundaryOK(text, start, end) && !spanOccupied(occupied, start, end) {
				se, ok := d.lookupStatus(term)
				if ok {
					se = fillStatusDurationFromFollowing(text, end, durLocs, se)
					matches = append(matches, statusTextMatch{
						start: start,
						end:   end,
						se:    se,
						wrap:  true,
					})
					markOccupied(occupied, start, end)
				}
			}
			off = start + 1
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		if matches[i].end != matches[j].end {
			return matches[i].end < matches[j].end
		}
		return matches[i].se.Name < matches[j].se.Name
	})
	return wrapMatchedEffectText(text, matches), dedupeStatusEffects(matches)
}

// matchEffectsInText extracts status effects from text and returns a deduped
// list for expanded status breakdowns.
func (d *AppData) matchEffectsInText(text string) []StatusEffect {
	_, matched := d.matchAndBracketEffectsInText(text)
	return matched
}

func (d *AppData) rewriteSoulBreakLists(rewrite func([]SoulBreak) []SoulBreak) {
	for name, sbList := range d.SoulBreaks {
		d.SoulBreaks[name] = rewrite(sbList)
	}
}

func filterSoulBreaksByRemovedIndex(sbList []SoulBreak, remove map[int]bool) []SoulBreak {
	if len(remove) == 0 {
		return sbList
	}
	filtered := make([]SoulBreak, 0, len(sbList)-len(remove))
	for i, sb := range sbList {
		if !remove[i] {
			filtered = append(filtered, sb)
		}
	}
	return filtered
}

func (d *AppData) matchSoulBreakEffects() {
	d.rewriteSoulBreakLists(func(sbList []SoulBreak) []SoulBreak {
		for i := range sbList {
			sbList[i].Effects, sbList[i].MatchedEffects = d.matchAndBracketEffectsInText(sbList[i].Effects)
		}
		return sbList
	})
}

func (d *AppData) pairDualShifts() {
	d.rewriteSoulBreakLists(func(sbList []SoulBreak) []SoulBreak {
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
		return filterSoulBreaksByRemovedIndex(sbList, remove)
	})
}

func (d *AppData) pairArcaneDyads() {
	d.rewriteSoulBreakLists(func(sbList []SoulBreak) []SoulBreak {
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
		return filterSoulBreaksByRemovedIndex(sbList, remove)
	})
}

// Preferred realm display order (roughly follows mainline game numbering)
var realmOrder = map[string]int{
	"I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6,
	"VII": 7, "VIII": 8, "IX": 9, "X": 10, "XI": 11, "XII": 12,
	"XIII": 13, "XIV": 14, "XV": 15, "XVI": 16,
	"FFT": 17, "Type-0": 18, "KH": 19, "Beyond": 20, "Core": 21, "DB Only": 22,
}

func (d *AppData) buildRealmGroups() {
	grouped := make(map[string][]Character)
	for _, c := range d.Characters {
		grouped[c.Realm] = append(grouped[c.Realm], c)
	}
	for realm, chars := range grouped {
		sort.Slice(chars, func(i, j int) bool {
			return chars[i].Name < chars[j].Name
		})
		grouped[realm] = chars
		d.RealmGroups = append(d.RealmGroups, RealmGroup{Realm: realm, Characters: chars})
	}
	sort.Slice(d.RealmGroups, func(i, j int) bool {
		oi, oj := realmOrder[d.RealmGroups[i].Realm], realmOrder[d.RealmGroups[j].Realm]
		if oi != oj {
			return oi < oj
		}
		return d.RealmGroups[i].Realm < d.RealmGroups[j].Realm
	})
	d.CharByID = make(map[string]*Character)
	for i := range d.Characters {
		d.CharByID[d.Characters[i].ID] = &d.Characters[i]
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

func (d *AppData) cacheAllImages() {
	dirs := []string{"images/characters", "images/abilities", "images/hero_abilities", "images/soulbreaks", "images/burst", "images/synchro", "images/zenith", "images/brave", "images/trigger"}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0o755)
	}

	// Cache static brave images
	ensureCachedFile("images/brave/BraveBase.png", "https://dff.sp.mbga.jp/dff/static/lang/image/ability/30151001/30151001_128.png")

	// Cache static trigger ability image
	ensureCachedFile("images/trigger/default_attack.png", "https://dff.sp.mbga.jp/dff/static/lang/image/ability/30151001/30151001_128.png")
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

	for i := range d.Characters {
		jobs = append(jobs, imageJob{"images/characters", "images/characters", charFmt, d.Characters[i].ID, &d.Characters[i].Img})
	}

	for name := range d.HeroAbilities {
		haList := d.HeroAbilities[name]
		for i := range haList {
			if haList[i].ID != "" {
				jobs = append(jobs, imageJob{"images/hero_abilities", "images/hero_abilities", abilityFmt, haList[i].ID, &haList[i].Img})
			}
		}
		d.HeroAbilities[name] = haList
	}

	for name := range d.SoulBreaks {
		sbList := d.SoulBreaks[name]
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
		d.SoulBreaks[name] = sbList
	}

	// Set default image for trigger abilities on all soul breaks
	for name := range d.SoulBreaks {
		sbList := d.SoulBreaks[name]
		for i := range sbList {
			for j := range sbList[i].TriggerAbilities {
				sbList[i].TriggerAbilities[j].Img = "/images/trigger/default_attack.png"
			}
		}
		d.SoulBreaks[name] = sbList
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
