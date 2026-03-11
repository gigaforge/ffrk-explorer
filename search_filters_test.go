package main

import "testing"

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
