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
var instantATBRe = regexp.MustCompile(`(?i)instant atb`)
var aegisCounterRe = regexp.MustCompile(`(?i)def, res and mnd -\d+%`)
var fullbreakCounterRe = regexp.MustCompile(`(?i)atk, def, mag and res \+\d+%`)
var physJobBreakCounterRe = regexp.MustCompile(`(?i)atk and mnd \+\d+%, def and res \+\d+%`)
var magJobBreakCounterRe = regexp.MustCompile(`(?i)mag and mnd \+\d+%, def and res \+\d+%`)
var weaknessBoostRe = regexp.MustCompile(`(?i)weakness \+\d+%`)
var magicalBoostRe = regexp.MustCompile(`(?i)magical \+\d+%`)
var phyBoostRe = regexp.MustCompile(`(?i)phy \+\d+%`)
var sorceryBoostRe = regexp.MustCompile(`(?i)sorcery damage \+\d+%`)
var pentabreakBoostRe = regexp.MustCompile(`(?i)pentabreak damage boost`)

func hasPartyEffectPattern(text string, re *regexp.Regexp) bool {
	lowerText := strings.ToLower(text)
	for _, idx := range re.FindAllStringIndex(text, -1) {
		tail := lowerText[idx[1]:]
		if len(tail) == 0 {
			continue
		}

		partyPhrases := []string{"to all allies", "to party"}
		for _, phrase := range partyPhrases {
			searchFrom := 0
			for {
				phraseIdx := strings.Index(tail[searchFrom:], phrase)
				if phraseIdx < 0 {
					break
				}
				phrasePos := searchFrom + phraseIdx
				between := tail[:phrasePos]
				if !strings.Contains(between, "to the user") && !strings.Contains(between, "to user") && !strings.Contains(between, "grants") {
					return true
				}
				searchFrom = phrasePos + len(phrase)
			}
		}
	}
	return false
}

// hasPartyEffectByTarget checks if an effect applies to the party based on the
// soul break's target being "All allies" and the effect appearing after "Grants"
// but before any second "Grants...to user" boundary.
func hasPartyEffectByTarget(text string, target string, re *regexp.Regexp) bool {
	if !strings.EqualFold(target, "All allies") {
		return false
	}
	lowerText := strings.ToLower(text)

	// Find the first occurrence of "grants"
	grantsIdx := strings.Index(lowerText, "grants")
	if grantsIdx < 0 {
		return false
	}
	afterFirstGrants := grantsIdx + len("grants")

	// Find the party region end: either a second "grants" that leads to "to user"/"to the user", or end of text.
	// If there is only one "grants" and "to user" appears after it, the effects go to the user, not the party.
	afterGrants := lowerText[afterFirstGrants:]
	partyRegionEnd := len(lowerText)
	secondGrants := strings.Index(afterGrants, "grants")
	if secondGrants >= 0 {
		// Second "grants" found — if it leads to "to user", party region ends there
		secondGrantsPos := afterFirstGrants + secondGrants
		afterSecond := lowerText[secondGrantsPos:]
		if strings.Contains(afterSecond, "to user") || strings.Contains(afterSecond, "to the user") {
			partyRegionEnd = secondGrantsPos
		}
	} else {
		// No second "grants" — if "to user" appears, all effects go to user, not party
		if strings.Contains(afterGrants, "to user") || strings.Contains(afterGrants, "to the user") {
			return false
		}
	}

	// Check if the regex matches within the party region
	partyRegion := text[afterFirstGrants:partyRegionEnd]
	return re.MatchString(partyRegion)
}

// sbPartyEffectCheckers maps effect filter keys to functions that check if a soul break
// provides a party-wide effect, considering both text patterns and the SB's target field.
var sbPartyEffectCheckers = map[string]func(SoulBreak) bool{
	"party_crit_chance": func(sb SoulBreak) bool {
		return hasPartyEffectByTarget(sb.Effects, sb.Target, critChanceRe)
	},
	"party_crit_damage": func(sb SoulBreak) bool {
		return hasPartyEffectByTarget(sb.Effects, sb.Target, critDamageRe)
	},
	"party_instant_atb": func(sb SoulBreak) bool {
		return hasPartyEffectByTarget(sb.Effects, sb.Target, instantATBRe)
	},
}

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
	"proshellga": func(text string) bool {
		hasHaste := containsCI(text, "[Haste]") || containsCI(text, "Haste]")
		hasProtect := containsCI(text, "[Protect]") || containsCI(text, "Protect]")
		hasShell := containsCI(text, "[Shell]") || containsCI(text, "Shell]")
		return hasHaste && hasProtect && hasShell
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
	"party_crit_chance": func(text string) bool {
		return hasPartyEffectPattern(text, critChanceRe)
	},
	"crit_damage": func(text string) bool {
		return critDamageRe.MatchString(text)
	},
	"party_crit_damage": func(text string) bool {
		return hasPartyEffectPattern(text, critDamageRe)
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
	"party_instant_atb": func(text string) bool {
		return hasPartyEffectPattern(text, instantATBRe)
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

type sbTextWalkOptions struct {
	IncludeStatuses bool
	IncludeBrave    bool
}

func walkStatusEffectTexts(statuses []StatusEffect, visit func(string) bool) bool {
	for _, se := range statuses {
		if visit(se.Name) || visit(se.Description) {
			return true
		}
	}
	return false
}

func walkSBTexts(sb SoulBreak, opts sbTextWalkOptions, visit func(string) bool) bool {
	if visit(sb.Effects) {
		return true
	}
	if opts.IncludeStatuses && walkStatusEffectTexts(sb.MatchedEffects, visit) {
		return true
	}

	if sb.DualShift != nil {
		if visit(sb.DualShift.Effects) {
			return true
		}
		if opts.IncludeStatuses && walkStatusEffectTexts(sb.DualShift.MatchedEffects, visit) {
			return true
		}
	}

	if sb.ArcaneDyad != nil {
		if visit(sb.ArcaneDyad.Effects) {
			return true
		}
		if opts.IncludeStatuses && walkStatusEffectTexts(sb.ArcaneDyad.MatchedEffects, visit) {
			return true
		}
	}

	for _, bc := range sb.BurstCommands {
		if visit(bc.Effects) {
			return true
		}
		if opts.IncludeStatuses && walkStatusEffectTexts(bc.MatchedEffects, visit) {
			return true
		}
	}

	for _, sa := range sb.SynchroAbilities {
		if visit(sa.Effects) {
			return true
		}
		if opts.IncludeStatuses && walkStatusEffectTexts(sa.MatchedEffects, visit) {
			return true
		}
	}

	for _, za := range sb.ZenithAbilities {
		if visit(za.Effects) {
			return true
		}
		if opts.IncludeStatuses && walkStatusEffectTexts(za.MatchedEffects, visit) {
			return true
		}
	}

	for _, ta := range sb.TriggerAbilities {
		if visit(ta.Effects) {
			return true
		}
		if opts.IncludeStatuses && walkStatusEffectTexts(ta.MatchedEffects, visit) {
			return true
		}
	}

	if opts.IncludeBrave && sb.BraveCommand != nil {
		for _, bl := range sb.BraveCommand.Levels {
			if visit(bl.Effects) {
				return true
			}
			if opts.IncludeStatuses && walkStatusEffectTexts(bl.MatchedEffects, visit) {
				return true
			}
		}
	}

	return false
}

// sbMatchesProshellga checks if a soul break provides all three of Haste, Protect, and Shell
// across any of its text fields.
func sbMatchesProshellga(sb SoulBreak) bool {
	opts := sbTextWalkOptions{IncludeStatuses: true, IncludeBrave: true}
	return walkSBTexts(sb, opts, effectCheckers["haste"]) &&
		walkSBTexts(sb, opts, effectCheckers["protect"]) &&
		walkSBTexts(sb, opts, effectCheckers["shell"])
}

// sbMatchesEffect checks if a soul break matches a single effect filter key.
func sbMatchesEffect(sb SoulBreak, eff string) bool {
	if eff == "proshellga" {
		return sbMatchesProshellga(sb)
	}
	// Check SB-level party effect checkers (target-based) first
	if sbChecker, ok := sbPartyEffectCheckers[eff]; ok {
		if sbChecker(sb) {
			return true
		}
	}
	checker, ok := effectCheckers[eff]
	if !ok {
		return false
	}
	return walkSBTexts(sb, sbTextWalkOptions{IncludeStatuses: true, IncludeBrave: true}, checker)
}

// sbMatchesAdditionalEffects checks if a soul break matches the given effect filters using the specified mode.
func sbMatchesAdditionalEffects(sb SoulBreak, effects []string, mode string) bool {
	if mode == "or" {
		for _, eff := range effects {
			if sbMatchesEffect(sb, eff) {
				return true
			}
		}
		return false
	}
	for _, eff := range effects {
		if !sbMatchesEffect(sb, eff) {
			return false
		}
	}
	return true
}

// lmMatchesAdditionalEffects checks if a legend materia matches the given effect filters using the specified mode.
func lmMatchesAdditionalEffects(lm LegendMateria, effects []string, mode string) bool {
	if mode == "or" {
		for _, eff := range effects {
			checker, ok := effectCheckers[eff]
			if !ok {
				continue
			}
			if checker(lm.Effect) {
				return true
			}
		}
		return false
	}
	for _, eff := range effects {
		checker, ok := effectCheckers[eff]
		if !ok {
			continue
		}
		if !checker(lm.Effect) {
			return false
		}
	}
	return true
}

// haMatchesAdditionalEffects checks if a hero ability matches the given effect filters using the specified mode.
func haMatchesAdditionalEffects(ha HeroAbility, effects []string, mode string) bool {
	if mode == "or" {
		for _, eff := range effects {
			checker, ok := effectCheckers[eff]
			if !ok {
				continue
			}
			if checker(ha.Effects) {
				return true
			}
		}
		return false
	}
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

// textContainsAttach checks if text contains "Attach <element>" or "En-<element>" pattern
func textContainsAttach(text, element string) bool {
	return containsCI(text, "Attach "+element) || containsCI(text, "En-"+element)
}

// textContainsImperil checks if text contains "Imperil <element>" or either prismatic phrasing.
func textContainsImperil(text, element string) bool {
	if containsCI(text, "Imperil "+element) {
		return true
	}
	// Prismatic imperil matches any specific element filter, including Poison.
	if element != "Prismatic" && (containsCI(text, "Imperil Prismatic") || containsCI(text, "Prismatic Imperil")) {
		return true
	}
	return false
}

// sbMatchesElement checks if a soul break (including sub-abilities) matches en-element criteria
func sbMatchesElement(sb SoulBreak, element string) bool {
	return walkSBTexts(sb, sbTextWalkOptions{}, func(text string) bool {
		return textContainsAttach(text, element)
	})
}

// sbMatchesImperil checks if a soul break (including sub-abilities) matches imperil criteria
func sbMatchesImperil(sb SoulBreak, element string) bool {
	return walkSBTexts(sb, sbTextWalkOptions{}, func(text string) bool {
		return textContainsImperil(text, element)
	})
}
