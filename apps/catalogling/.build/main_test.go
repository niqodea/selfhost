package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestCatalogling has a library of the shows given
func newTestCatalogling(t *testing.T, shows ...string) *catalogling {
	t.Helper()
	media := t.TempDir()
	c := &catalogling{
		waitingDir: filepath.Join(media, "imported"),
		showsDir:   filepath.Join(media, "shows"),
		stateFile:  filepath.Join(t.TempDir(), "state.json"),
		state:      state{Aliases: map[string][]string{}},
	}
	for _, dir := range append([]string{c.waitingDir, c.showsDir}, shows...) {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(c.showsDir, dir)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// handOn puts an episode where importling hands it on, as in The Office
// US/Season 02/03.mkv
func handOn(t *testing.T, c *catalogling, episode string) {
	t.Helper()
	path := filepath.Join(c.waitingDir, filepath.FromSlash(episode))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(episode), 0o644); err != nil {
		t.Fatal(err)
	}
	c.scan()
}

func post(c *catalogling, handler http.HandlerFunc, form url.Values) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler(httptest.NewRecorder(), request)
}

func exists(dir, file string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file)))
	return err == nil
}

func TestKnownShowIsFiled(t *testing.T) {
	c := newTestCatalogling(t, "The Office US (2005)")
	handOn(t, c, "The Office US/Season 02/03.mkv")
	if !exists(c.showsDir, "The Office US (2005)/Season 02/03.mkv") || exists(c.waitingDir, "The Office US") {
		t.Error("not moved into the library")
	}
}

// A new show waits, and once filed, its next episodes are filed on their own
func TestNewShow(t *testing.T) {
	c := newTestCatalogling(t)
	handOn(t, c, "Severance/Season 01/01.mkv")
	handOn(t, c, "Severance/Season 01/02.mkv")
	k := c.newCard("Severance", c.catalog())
	if len(k.Files) != 2 || len(k.Offers) != 1 || k.Offers[0] != (Offer{"Severance", true}) {
		t.Fatalf("got %+v", k)
	}
	post(c, c.handleFile, url.Values{"name": {"Severance"}, "show": {"Severance"}})
	if !exists(c.showsDir, "Severance/Season 01/01.mkv") || !exists(c.showsDir, "Severance/Season 01/02.mkv") {
		t.Fatal("not filed")
	}
	handOn(t, c, "Severance/Season 01/03.mkv")
	if !exists(c.showsDir, "Severance/Season 01/03.mkv") {
		t.Error("the next episode is not filed")
	}
}

// A name close to a show's offers the show, and filing under it makes the
// name an alias, which files the next episodes on their own
func TestSimilarShowTeachesAnAlias(t *testing.T) {
	c := newTestCatalogling(t, "The Floor (2024)")
	handOn(t, c, "The Floor US/Season 06/06.mkv")
	if k := c.newCard("The Floor US", c.catalog()); len(k.Offers) != 2 || k.Offers[0] != (Offer{Show: "The Floor (2024)"}) {
		t.Fatalf("got %+v", k)
	}
	post(c, c.handleFile, url.Values{"name": {"The Floor US"}, "show": {"the floor (2024)"}})
	handOn(t, c, "The Floor US/Season 06/07.mkv")
	if !exists(c.showsDir, "The Floor (2024)/Season 06/06.mkv") || !exists(c.showsDir, "The Floor (2024)/Season 06/07.mkv") {
		t.Error("not filed under the show")
	}
}

// An episode the library already has stays waiting, saying so, and replaces
// nothing
func TestEpisodeInTheLibrary(t *testing.T) {
	c := newTestCatalogling(t, "Show")
	handOn(t, c, "Show/Season 01/01.mkv")
	handOn(t, c, "Show/Season 01/01.mkv")
	if k := c.newCard("Show", c.catalog()); k.InLibrary != "Show" || len(k.Files) != 1 {
		t.Errorf("got %+v", k)
	}
	post(c, c.handleDiscard, url.Values{"name": {"Show"}})
	if exists(c.waitingDir, "Show") || !exists(c.showsDir, "Show/Season 01/01.mkv") {
		t.Error("discarding touched the library or kept the waiting show")
	}
}
