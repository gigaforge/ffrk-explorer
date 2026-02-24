package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func searchHandler(w http.ResponseWriter, r *http.Request) {
	d := getAppDataSnapshot()
	if d == nil {
		http.Error(w, "data not loaded", http.StatusServiceUnavailable)
		return
	}
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
		ownedSet = snapshotOwnedSoulbreaks(u.ID)
	}
	if ownedOnly && (u == nil || ownedSet == nil) {
		ownedOnly = false
	}

	// Filter characters
	var matchedChars []Character
	for _, c := range d.Characters {
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
	const maxResults = 500

	for _, c := range matchedChars {
		if truncated {
			break
		}

		// Collect matching soul breaks
		var matchedSBs []SoulBreak
		for _, sb := range d.SoulBreaks[c.Name] {
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
			for _, ha := range d.HeroAbilities[c.Name] {
				matchedHAs = append(matchedHAs, ha)
				totalCount++
				if totalCount >= maxResults {
					truncated = true
					break
				}
			}
		} else if !truncated && len(matchedSBs) > 0 && hasEffectFilter {
			for _, ha := range d.HeroAbilities[c.Name] {
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
	d := getAppDataSnapshot()
	if d == nil {
		http.Error(w, "data not loaded", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/char/")
	ch, ok := d.CharByID[id]
	if !ok {
		http.NotFound(w, r)
		return
	}

	detail := CharDetail{
		Character:     *ch,
		SoulBreaks:    d.SoulBreaks[ch.Name],
		HeroAbilities: d.HeroAbilities[ch.Name],
	}

	u := getCurrentUser(r)
	if u != nil {
		detail.LoggedIn = true
		detail.OwnedSoulbreaks = snapshotOwnedSoulbreaks(u.ID)
	} else {
		detail.OwnedSoulbreaks = make(map[string]bool)
	}

	charTmpl.Execute(w, detail)
}
