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
