package main

import (
	"slices"
	"testing"
)

func TestFind(t *testing.T) {
	c := Catalog{Shows: []string{"The Office US", "Game of Thrones"}, Aliases: map[string][]string{key("Juego de Tronos"): {"Game of Thrones"}}}
	for name, want := range map[string][]string{
		"the.office.us":        {"The Office US"},
		"Game of Thrones 2011": {"Game of Thrones"},
		"Juego de Tronos":      {"Game of Thrones"},
		"The Office":           nil,
	} {
		if got := c.Find(name); !slices.Equal(got, want) {
			t.Errorf("Find(%q) = %v, want %v", name, got, want)
		}
	}
	// An alias of a show the library no longer has means nothing
	c.Shows = []string{"The Office US"}
	if got := c.Find("Juego de Tronos"); got != nil {
		t.Errorf("found %v", got)
	}
}

func TestSimilar(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Frieren", "Frieren Beyond Journey's End", true},
		{"Agents of SHIELD", "Marvel's Agents of S.H.I.E.L.D.", true},
		{"Star Trek TNG", "Star Trek The Next Generation", true},
		{"Law and Order SVU", "Law & Order: Special Victims Unit", true},
		{"Hawaii Five-0", "Hawaii Five-O", true},
		{"The Office", "The Office (US)", true},
		{"Beavis and Butt-Head", "Mike Judge's Beavis and Butt-Head", true},
		{"The Floor US", "The Floor (2024)", true},
		{"Lost", "Host", false},
		{"The Boys", "The Bear", false},
		{"Severance", "Breaking Bad", false},
		{"The", "The Office", false},
		{"Re", "Robinson Efter Oe-Raadet", false},
		{"The Witch", "The Nature of Things with David Suzuki", false},
		{"Deutschland", "Das 1 Prozent Quiz - Wie clever ist Deutschland", false},
	}
	for _, test := range tests {
		if _, got := similar(test.a, test.b); got != test.want {
			t.Errorf("similar(%q, %q) = %v, want %v", test.a, test.b, got, test.want)
		}
	}
	if key("It's Always Sunny") != key("Its Always Sunny") || key("Law & Order") != key("Law and Order") || key("Chicago P.D.") != key("Chicago PD") {
		t.Error("names that differ in apostrophes, & or dots have different keys")
	}
}
