package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const maxSearchResults = 500

type searchSchoolReq struct {
	School   string
	MinLevel int
}

type searchRequest struct {
	Character         string
	characterLower    string
	Realm             string
	Tier              string
	Element           string
	Imperil           string
	SchoolsParam      string
	SchoolMode        string
	OwnedOnly         bool
	AdditionalEffects []string
	SchoolReqs        []searchSchoolReq
	HasEffectFilter   bool
	HasSBFilter       bool
}

func parseSearchRequest(r *http.Request) searchRequest {
	q := r.URL.Query()
	req := searchRequest{
		Character:    q.Get("character"),
		Realm:        q.Get("realm"),
		Tier:         q.Get("tier"),
		Element:      q.Get("element"),
		Imperil:      q.Get("imperil"),
		SchoolsParam: q.Get("schools"),
		SchoolMode:   q.Get("schoolmode"),
		OwnedOnly:    q.Get("owned") == "1",
	}
	if req.SchoolMode == "" {
		req.SchoolMode = "and"
	}
	req.characterLower = strings.ToLower(req.Character)

	effectsParam := q.Get("effects")
	if effectsParam != "" {
		for _, e := range strings.Split(effectsParam, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				req.AdditionalEffects = append(req.AdditionalEffects, e)
			}
		}
	}

	if req.SchoolsParam != "" {
		for _, pair := range strings.Split(req.SchoolsParam, ",") {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) != 2 {
				continue
			}
			level, err := strconv.Atoi(parts[1])
			if err != nil || level < 1 || level > 6 {
				continue
			}
			req.SchoolReqs = append(req.SchoolReqs, searchSchoolReq{
				School:   parts[0],
				MinLevel: level,
			})
		}
	}

	req.HasEffectFilter = req.Element != "" || req.Imperil != "" || len(req.AdditionalEffects) > 0
	req.HasSBFilter = req.Tier != "" || req.HasEffectFilter
	return req
}

func filterSearchCharacters(d *AppData, req searchRequest) []Character {
	var matched []Character
	for _, c := range d.Characters {
		if req.Character != "" && !strings.Contains(strings.ToLower(c.Name), req.characterLower) {
			continue
		}
		if req.Realm != "" && c.Realm != req.Realm {
			continue
		}
		if !characterMatchesSchoolReqs(c, req.SchoolReqs, req.SchoolMode) {
			continue
		}
		matched = append(matched, c)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})
	return matched
}

func characterMatchesSchoolReqs(c Character, reqs []searchSchoolReq, mode string) bool {
	if len(reqs) == 0 {
		return true
	}

	match := mode == "and"
	for _, req := range reqs {
		has := c.Schools[req.School] >= req.MinLevel
		if mode == "and" {
			if !has {
				match = false
				break
			}
			continue
		}
		if has {
			match = true
			break
		}
	}
	return match
}

func buildSearchResults(d *AppData, chars []Character, req searchRequest, ownedSet map[string]bool) ([]SearchResult, bool) {
	var results []SearchResult
	totalCount := 0
	truncated := false

	for _, c := range chars {
		if truncated {
			break
		}

		matchedSBs, nextCount, sbTruncated := collectMatchingSoulBreaks(d, c, req, ownedSet, totalCount)
		totalCount = nextCount
		truncated = sbTruncated

		matchedHAs, nextCount, haTruncated := collectMatchingHeroAbilities(d, c, req, matchedSBs, totalCount, truncated)
		totalCount = nextCount
		truncated = haTruncated

		matchedLMs, nextCount, lmTruncated := collectMatchingLegendMateria(d, c, req, ownedSet, totalCount, truncated)
		totalCount = nextCount
		truncated = lmTruncated

		if len(matchedSBs) > 0 || len(matchedHAs) > 0 || len(matchedLMs) > 0 {
			results = append(results, SearchResult{
				Character:     c,
				SoulBreaks:    matchedSBs,
				HeroAbilities: matchedHAs,
				LegendMateria: matchedLMs,
			})
		}
	}

	return results, truncated
}

func collectMatchingSoulBreaks(d *AppData, c Character, req searchRequest, ownedSet map[string]bool, totalCount int) ([]SoulBreak, int, bool) {
	var matchedSBs []SoulBreak
	truncated := false

	for _, sb := range d.SoulBreaks[c.Name] {
		if req.OwnedOnly && !ownedSet[sb.ID] {
			continue
		}
		if req.Tier != "" && sb.Tier != req.Tier {
			continue
		}
		if req.Element != "" && !sbMatchesElement(sb, req.Element) {
			continue
		}
		if req.Imperil != "" && !sbMatchesImperil(sb, req.Imperil) {
			continue
		}
		if len(req.AdditionalEffects) > 0 && !sbMatchesAdditionalEffects(sb, req.AdditionalEffects) {
			continue
		}

		matchedSBs = append(matchedSBs, sb)
		totalCount++
		if totalCount >= maxSearchResults {
			truncated = true
			break
		}
	}

	return matchedSBs, totalCount, truncated
}

func collectMatchingHeroAbilities(d *AppData, c Character, req searchRequest, matchedSBs []SoulBreak, totalCount int, alreadyTruncated bool) ([]HeroAbility, int, bool) {
	if alreadyTruncated {
		return nil, totalCount, true
	}

	var matchedHAs []HeroAbility
	truncated := false

	if !req.HasSBFilter {
		for _, ha := range d.HeroAbilities[c.Name] {
			matchedHAs = append(matchedHAs, ha)
			totalCount++
			if totalCount >= maxSearchResults {
				truncated = true
				break
			}
		}
		return matchedHAs, totalCount, truncated
	}

	if !req.HasEffectFilter {
		return nil, totalCount, false
	}

	for _, ha := range d.HeroAbilities[c.Name] {
		if req.Element != "" && !textContainsAttach(ha.Effects, req.Element) {
			continue
		}
		if req.Imperil != "" && !textContainsImperil(ha.Effects, req.Imperil) {
			continue
		}
		if len(req.AdditionalEffects) > 0 && !haMatchesAdditionalEffects(ha, req.AdditionalEffects) {
			continue
		}

		matchedHAs = append(matchedHAs, ha)
		totalCount++
		if totalCount >= maxSearchResults {
			truncated = true
			break
		}
	}

	return matchedHAs, totalCount, truncated
}

func isLMRTier(tier string) bool {
	return strings.Contains(tier, "LMR")
}

func collectMatchingLegendMateria(d *AppData, c Character, req searchRequest, ownedSet map[string]bool, totalCount int, alreadyTruncated bool) ([]LegendMateria, int, bool) {
	if alreadyTruncated {
		return nil, totalCount, true
	}

	var matchedLMs []LegendMateria
	truncated := false

	if req.Tier != "" {
		return nil, totalCount, false
	}

	if !req.HasSBFilter {
		for _, lm := range d.LegendMateria[c.Name] {
			if req.OwnedOnly && isLMRTier(lm.Tier) && !ownedSet[lm.ID] {
				continue
			}
			matchedLMs = append(matchedLMs, lm)
			totalCount++
			if totalCount >= maxSearchResults {
				truncated = true
				break
			}
		}
		return matchedLMs, totalCount, truncated
	}

	if !req.HasEffectFilter {
		return nil, totalCount, false
	}

	for _, lm := range d.LegendMateria[c.Name] {
		if req.OwnedOnly && isLMRTier(lm.Tier) && !ownedSet[lm.ID] {
			continue
		}
		if req.Element != "" && !textContainsAttach(lm.Effect, req.Element) {
			continue
		}
		if req.Imperil != "" && !textContainsImperil(lm.Effect, req.Imperil) {
			continue
		}
		if len(req.AdditionalEffects) > 0 && !lmMatchesAdditionalEffects(lm, req.AdditionalEffects) {
			continue
		}

		matchedLMs = append(matchedLMs, lm)
		totalCount++
		if totalCount >= maxSearchResults {
			truncated = true
			break
		}
	}

	return matchedLMs, totalCount, truncated
}

func buildSearchData(req searchRequest, results []SearchResult, truncated bool, u *User, ownedSet map[string]bool) SearchData {
	data := SearchData{
		Results:    results,
		Truncated:  truncated,
		MaxResults: maxSearchResults,
	}

	data.Query.Character = req.Character
	data.Query.Realm = req.Realm
	data.Query.Tier = req.Tier
	data.Query.Element = req.Element
	data.Query.Imperil = req.Imperil
	data.Query.Schools = req.SchoolsParam
	data.Query.SchoolMode = req.SchoolMode

	if u != nil {
		data.LoggedIn = true
		if ownedSet == nil {
			ownedSet = make(map[string]bool)
		}
		data.OwnedSoulbreaks = ownedSet
	} else {
		data.OwnedSoulbreaks = make(map[string]bool)
	}

	return data
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
		return
	}

	req := parseSearchRequest(r)

	u := getCurrentUser(r)
	var ownedSet map[string]bool
	if u != nil {
		ownedSet = snapshotOwnedSoulbreaks(u.ID)
	}
	if req.OwnedOnly && (u == nil || ownedSet == nil) {
		req.OwnedOnly = false
	}

	matchedChars := filterSearchCharacters(d, req)
	results, truncated := buildSearchResults(d, matchedChars, req, ownedSet)
	data := buildSearchData(req, results, truncated, u, ownedSet)

	searchTmpl.Execute(w, data)
}

func charHandler(w http.ResponseWriter, r *http.Request) {
	d, ok := requireAppData(w)
	if !ok {
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
		LegendMateria: d.LegendMateria[ch.Name],
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
