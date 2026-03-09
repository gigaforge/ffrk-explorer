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

func hasPartyCritPattern(text string, re *regexp.Regexp) bool {
	lowerText := strings.ToLower(text)
	for _, idx := range re.FindAllStringIndex(text, -1) {
		tail := lowerText[idx[1]:]
		if len(tail) == 0 {
			continue
		}

		searchFrom := 0
		for {
			alliesIdx := strings.Index(tail[searchFrom:], "to all allies")
			if alliesIdx < 0 {
				break
			}
			alliesPos := searchFrom + alliesIdx
			between := tail[:alliesPos]
			if !strings.Contains(between, "to the user") {
				return true
			}
			searchFrom = alliesPos + len("to all allies")
		}
	}
	return false
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
		return hasPartyCritPattern(text, critChanceRe)
	},
	"crit_damage": func(text string) bool {
		return critDamageRe.MatchString(text)
	},
	"party_crit_damage": func(text string) bool {
		return hasPartyCritPattern(text, critDamageRe)
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

// sbMatchesAdditionalEffects checks if a soul break matches the given effect filters using the specified mode.
func sbMatchesAdditionalEffects(sb SoulBreak, effects []string, mode string) bool {
	if mode == "or" {
		for _, eff := range effects {
			checker, ok := effectCheckers[eff]
			if !ok {
				continue
			}
			if walkSBTexts(sb, sbTextWalkOptions{IncludeStatuses: true, IncludeBrave: true}, checker) {
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
		if !walkSBTexts(sb, sbTextWalkOptions{IncludeStatuses: true, IncludeBrave: true}, checker) {
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
