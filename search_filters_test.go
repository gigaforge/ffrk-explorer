package main

import "testing"

func TestHasPartyEffectByTarget(t *testing.T) {
	// Example: "Grants Deadly Strikes +20%, Instant ATB 1, Instant Cast 1, grants Crystal Mode to user"
	// Target: All allies → Instant ATB should be a party effect
	text := "Grants Deadly Strikes +20%, Instant ATB 1, Instant Cast 1, grants Crystal Mode to user"
	if !hasPartyEffectByTarget(text, "All allies", instantATBRe) {
		t.Fatal("expected Instant ATB to match as party effect when target is All allies")
	}

	// "Crystal Mode" is granted "to user", not party — but it's not an ATB effect so irrelevant.
	// What matters: effects after the second "grants...to user" should NOT match.
	// Test with a regex that would match "Crystal Mode" — it should not be considered party.
	crystalRe := critChanceRe // just need something that won't match Crystal Mode
	if hasPartyEffectByTarget(text, "All allies", crystalRe) {
		t.Fatal("should not match crit chance in this text")
	}

	// Target is not "All allies" → should not match
	if hasPartyEffectByTarget(text, "Single", instantATBRe) {
		t.Fatal("should not match when target is not All allies")
	}

	// No "Grants" in text → should not match
	noGrants := "Instant ATB 1, Instant Cast 1"
	if hasPartyEffectByTarget(noGrants, "All allies", instantATBRe) {
		t.Fatal("should not match when text has no Grants")
	}

	// Effect appears only after the second "grants...to user" boundary → should not match
	afterUser := "Grants Crystal Mode, grants Instant ATB 1 to user"
	if hasPartyEffectByTarget(afterUser, "All allies", instantATBRe) {
		t.Fatal("should not match when Instant ATB is in the user-only region")
	}

	// Single "grants" with "to user" → all effects go to user, not party (Gilgamesh MASB case)
	singleGrantsToUser := "Damage +5% to Samurais, grants Master Mode, Instant ATB 1 & Diffusion Barrier 1 to user"
	if hasPartyEffectByTarget(singleGrantsToUser, "All allies", instantATBRe) {
		t.Fatal("should not match when single grants clause ends with to user")
	}

	// Single grants with no "to user" → everything after grants is party
	allParty := "Grants Instant ATB 1, Instant Cast 1"
	if !hasPartyEffectByTarget(allParty, "All allies", instantATBRe) {
		t.Fatal("expected match when all effects are party-wide (no to user)")
	}
}

func TestTextContainsImperilMatchesPrismaticImperilForSpecificElements(t *testing.T) {
	if !textContainsImperil("Chases with minor Prismatic Imperil for 5 seconds", "Poison") {
		t.Fatal("expected Poison imperil filter to match Prismatic Imperil text")
	}
	if !textContainsImperil("Causes [Imperil Prismatic 10% (15s)]", "Poison") {
		t.Fatal("expected Poison imperil filter to match Imperil Prismatic text")
	}
	if textContainsImperil("Chases with minor Prismatic Imperil for 5 seconds", "Prismatic") {
		t.Fatal("expected Prismatic filter to require explicit Imperil Prismatic text")
	}
}
