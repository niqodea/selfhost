package main

import (
	"io"
	"testing"
	"time"
)

func TestPageRenders(t *testing.T) {
	waiting := []card{
		{Name: "The Floor US", Files: []string{"Season 06/06.mkv"}, Question: "No show of the library goes by this name, but some have a similar one",
			Offers: []Offer{{Show: "The Floor (2024)"}, {"The Floor US", true}}},
		{Name: "Show", Files: []string{"Season 01/01.mkv"}, InLibrary: "Show"},
	}
	filings := []Filing{{"Show", "Show", []string{"Season 01/01.mkv"}, time.Now()}}
	if err := page.Execute(io.Discard, map[string]any{"Shows": []string{"The Floor (2024)"}, "Waiting": waiting, "Filings": filings}); err != nil {
		t.Fatal(err)
	}
}
