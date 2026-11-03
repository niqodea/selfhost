package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newTestImporter(t *testing.T, files ...string) *importer {
	t.Helper()
	media := t.TempDir()
	for _, dir := range []string{"downloads", "shows", "movies"} {
		if err := os.Mkdir(filepath.Join(media, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range files {
		path := filepath.Join(media, "downloads", file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	im := &importer{
		mediaDir:  media,
		stateFile: filepath.Join(t.TempDir(), "state.json"),
		state:     state{Torrents: map[string]*entry{}},
	}
	im.scan()
	return im
}

func answer(im *importer, form url.Values) {
	request := httptest.NewRequest(http.MethodPost, "/answer", nil)
	request.PostForm = form
	im.handleAnswer(httptest.NewRecorder(), request)
}

func TestAnswerWithoutSeasonImportsNothing(t *testing.T) {
	name := "Avatar.The.Last.Airbender.Book.1.Water"
	im := newTestImporter(t, name+"/01 The Boy in the Iceberg.avi")

	form := url.Values{"name": {name}, "kind": {"show"}, "title-show": {"Avatar"}, "include-0": {"on"}, "season": {""}, "episode-0": {"1"}}
	answer(im, form)
	e := im.Torrents[name]
	if e.Status != "waiting" || len(e.Links) > 0 {
		t.Fatalf("imported with no season: %s, %v", e.Status, e.Links)
	}
	// The season stays empty, and offers the first, never a 0
	c := newCard(e, nil, nil)
	if c.Season != "" || len(c.SuggestSeason) != 1 || c.SuggestSeason[0].Value != "1" {
		t.Errorf("season filled in as %q, suggestions %v", c.Season, c.SuggestSeason)
	}

	form.Set("season", "1")
	answer(im, form)
	if e.Status != "imported" || len(e.Links) != 1 || e.Links[0].To != "shows/Avatar/Season 01/01.avi" {
		t.Errorf("not imported as season 1: %s, %v", e.Status, e.Links)
	}
}

func TestMovieIsOneFile(t *testing.T) {
	name := "Avatar.The.Last.Airbender.Book.1.Water"
	im := newTestImporter(t, name+"/01 The Boy in the Iceberg.avi", name+"/02 The Avatar Returns.avi")

	form := url.Values{"name": {name}, "kind": {"movie"}, "title-movie": {"Avatar"}, "year-movie": {"2005"}, "include-0": {"on"}, "include-1": {"on"}}
	answer(im, form)
	e := im.Torrents[name]
	if e.Status != "waiting" {
		t.Fatalf("imported a movie with no file picked: %s, %v", e.Status, e.Links)
	}

	form.Set("movie-file", "1")
	answer(im, form)
	want := []Link{{name + "/02 The Avatar Returns.avi", "movies/Avatar (2005)/Avatar (2005).avi"}}
	if e.Status != "imported" || !slices.Equal(e.Links, want) {
		t.Errorf("got %s, %v, want %v", e.Status, e.Links, want)
	}

	// Both answers are in the log of answers
	data, err := os.ReadFile(filepath.Join(filepath.Dir(im.stateFile), "answers.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 2 {
		t.Errorf("%d lines in the log of answers, want 2", lines)
	}
}
