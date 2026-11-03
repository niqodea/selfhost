package main

import (
	"io"
	"testing"
	"time"
)

func TestPageRenders(t *testing.T) {
	names := []string{
		"Superman 2.mkv",
		"Doctor.Who.S01E01.mkv",
		"Daily.Show.2024.03.12.mkv",
		"Show.Name.S01/Show.Name.Special.720p.mkv",
	}
	var waiting []card
	for _, name := range names {
		torrent := Read([]File{{Path: name}})
		e := &entry{Name: name, Status: "waiting", Videos: torrent.Videos, LeftOut: torrent.LeftOut}
		e.Doubts = Resolve(e.Videos, []string{"Doctor Who (2005)"}, nil, false).Doubts
		waiting = append(waiting, newCard(e, []string{"Doctor Who (2005)"}, nil))
	}
	imported := &entry{
		Name: "Show.S01", Status: "imported", Done: time.Now(),
		Links:   []Link{{From: "Show.S01/Show.S01E01.mkv", To: "shows/Show/Season 01/S01E01.mkv"}},
		LeftOut: []LeftOut{{Path: "Show.S01/Sample/sample.mkv", Reason: "named as a sample or trailer"}},
	}
	err := page.Execute(io.Discard, map[string]any{
		"Library": []string{"Doctor Who (2005)"},
		"Waiting": waiting,
		"Done":    []done{newDone(imported)},
	})
	if err != nil {
		t.Fatal(err)
	}
}
