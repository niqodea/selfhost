package main

import (
	"io"
	"testing"
	"time"
)

func TestPageRenders(t *testing.T) {
	var waiting []card
	for _, files := range [][]File{
		{{Path: "Fargo.1996.mkv"}},
		{{Path: "Show/Season 1/a.mkv"}, {Path: "Show/Season 2/b.mkv"}},
		{{Path: "Breaking Bad Season 2/03.mkv"}},
	} {
		e := &entry{Name: files[0].Path, Status: "waiting", Form: NewForm(Read(files))}
		waiting = append(waiting, newCard(e))
	}
	imported := &entry{
		Name: "Show.S01", Status: "imported", Done: time.Now(),
		Form:    NewForm(Read([]File{{Path: "Show.S01/Show.S01E01.mkv"}})),
		LeftOut: []LeftOut{{Path: "Show.S01/Sample/sample.mkv", Reason: "named as a sample or trailer"}},
	}
	left := &entry{Name: "Show.S01E01E02.mkv", Status: "left", Done: time.Now(), Unsupported: []string{"Holds more than one episode"}}
	skipped := &entry{Name: "Fargo.mkv", Status: "skipped", Done: time.Now()}
	err := page.Execute(io.Discard, map[string]any{
		"Waiting": waiting,
		"Done":    []done{newDone(imported), newDone(left), newDone(skipped)},
	})
	if err != nil {
		t.Fatal(err)
	}
}
