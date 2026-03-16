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

func TestNormalizeStatShorthand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "phys job break counter shorthand",
			input: "ATK/MND +25% & DEF/RES +30% to party",
			want:  "ATK and MND +25%, DEF and RES +30% to party",
		},
		{
			name:  "mag job break counter shorthand",
			input: "MAG/MND +25% & DEF/RES +30% to party",
			want:  "MAG and MND +25%, DEF and RES +30% to party",
		},
		{
			name:  "full break counter shorthand with reorder",
			input: "ATK/MAG/DEF/RES +30% to party",
			want:  "ATK, DEF, MAG and RES +30% to party",
		},
		{
			name:  "aegis counter shorthand",
			input: "DEF/RES/MND -70% to target",
			want:  "DEF, RES and MND -70% to target",
		},
		{
			name:  "does not touch DEF/RES Pierce",
			input: "25% DEF/RES Pierce",
			want:  "25% DEF/RES Pierce",
		},
		{
			name:  "does not touch non-stat ampersand",
			input: "Cap Break Level 1 & Dual Awoken Mode",
			want:  "Cap Break Level 1 & Dual Awoken Mode",
		},
		{
			name:  "bracketed shorthand",
			input: "[MAG/MND +25% & DEF/RES +30%] for 8 seconds",
			want:  "[MAG and MND +25%, DEF and RES +30%] for 8 seconds",
		},
		{
			name:  "mixed shorthand and normal text",
			input: "7 attacks, ATK/MND +25% & DEF/RES +30% to party, Cap Break Level 1 & Dual Awoken Mode to user",
			want:  "7 attacks, ATK and MND +25%, DEF and RES +30% to party, Cap Break Level 1 & Dual Awoken Mode to user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeStatShorthand(tt.input)
			if got != tt.want {
				t.Errorf("normalizeStatShorthand(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
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
