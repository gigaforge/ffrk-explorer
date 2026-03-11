package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestMatchAndBracketEffectsInTextSkipsUnbracketedPoison(t *testing.T) {
	d := &AppData{
		StatusEffects: map[string]StatusEffect{
			"Poison": {
				Name:        "Poison",
				Description: "Generic poison.",
			},
			"Imperil Poison": {
				Name:        "Imperil Poison",
				Description: "Lowers poison resistance.",
			},
		},
		statusMatchTerms: []string{"Imperil Poison", "Poison"},
	}

	text, matched := d.matchAndBracketEffectsInText("Deals magic damage and causes Imperil Poison for 25 seconds.")
	if text != "Deals magic damage and causes [Imperil Poison] for 25 seconds." {
		t.Fatalf("unexpected bracketed text: %q", text)
	}

	want := []StatusEffect{{
		Name:        "Imperil Poison",
		Description: "Lowers poison resistance.",
		Duration:    "25 seconds",
	}}
	if !reflect.DeepEqual(matched, want) {
		t.Fatalf("unexpected matched effects: %#v", matched)
	}
}

func TestMatchAndBracketEffectsInTextKeepsBracketedPoison(t *testing.T) {
	d := &AppData{
		StatusEffects: map[string]StatusEffect{
			"Poison": {
				Name:        "Poison",
				Description: "Generic poison.",
			},
		},
		statusMatchTerms: []string{"Poison"},
	}

	text, matched := d.matchAndBracketEffectsInText("Deals magic damage and causes [Poison] for 8 seconds.")
	if text != "Deals magic damage and causes [Poison] for 8 seconds." {
		t.Fatalf("unexpected bracketed text: %q", text)
	}

	want := []StatusEffect{{
		Name:        "Poison",
		Description: "Generic poison.",
		Duration:    "8 seconds",
	}}

	sort.Slice(matched, func(i, j int) bool { return matched[i].Name < matched[j].Name })
	sort.Slice(want, func(i, j int) bool { return want[i].Name < want[j].Name })
	if !reflect.DeepEqual(matched, want) {
		t.Fatalf("unexpected matched effects: %#v", matched)
	}
}

func TestMatchAndBracketEffectsInTextMatchesPrismaticImperilAlias(t *testing.T) {
	d := &AppData{
		StatusEffects: map[string]StatusEffect{
			"Imperil Prismatic 10% (5s)": {
				Name:        "Imperil Prismatic 10% (5s)",
				Description: "Lowers all elemental resistance by 10%.",
				Duration:    "5 seconds",
			},
		},
		statusAliases: map[string]StatusEffect{
			"Prismatic Imperil": {
				Name:        "Imperil Prismatic",
				Description: "Lowers Fire, Ice, Lightning, Earth, Wind, Water, Holy, Dark, and Poison resistance.",
			},
			"Prismatic Imperil 10% (5s)": {
				Name:        "Imperil Prismatic 10% (5s)",
				Description: "Lowers all elemental resistance by 10%.",
				Duration:    "5 seconds",
			},
		},
		statusMatchTerms: []string{"Prismatic Imperil 10% (5s)", "Prismatic Imperil"},
	}

	text, matched := d.matchAndBracketEffectsInText("Chases with minor Prismatic Imperil for 5 seconds and later causes Prismatic Imperil 10% (5s).")
	if text != "Chases with minor [Prismatic Imperil] for 5 seconds and later causes [Prismatic Imperil 10% (5s)]." {
		t.Fatalf("unexpected bracketed text: %q", text)
	}

	want := []StatusEffect{
		{
			Name:        "Imperil Prismatic",
			Description: "Lowers Fire, Ice, Lightning, Earth, Wind, Water, Holy, Dark, and Poison resistance.",
			Duration:    "5 seconds",
		},
		{
			Name:        "Imperil Prismatic 10% (5s)",
			Description: "Lowers all elemental resistance by 10%.",
			Duration:    "5 seconds",
		},
	}
	if !reflect.DeepEqual(matched, want) {
		t.Fatalf("unexpected matched effects: %#v", matched)
	}
}
