package main

import (
	"regexp"
	"strings"
)

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
