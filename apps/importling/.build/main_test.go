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

func newTestImportling(t *testing.T, files ...string) *importling {
	t.Helper()
	im := &importling{
		downloadsDir: t.TempDir(),
		importedDir:  t.TempDir(),
		stateFile:    filepath.Join(t.TempDir(), "state.json"),
		state:        state{Torrents: map[string]*entry{}},
	}
	for _, file := range files {
		download(t, im, file)
	}
	return im
}

func download(t *testing.T, im *importling, file string) {
	t.Helper()
	path := filepath.Join(im.downloadsDir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	im.scan()
}

func post(im *importling, handler http.HandlerFunc, form url.Values) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler(httptest.NewRecorder(), request)
}

// handedOn reports whether the download's file is handed on at to
func handedOn(im *importling, file, to string) bool {
	return sameFile(filepath.Join(im.downloadsDir, file), filepath.Join(im.importedDir, to))
}

func TestSettledTorrentIsImported(t *testing.T) {
	name := "The.Office.US.S02E03.1080p.WEB-DL.x264-GRP.mkv"
	im := newTestImportling(t, name)
	if e := im.Torrents[name]; e.Status != "imported" {
		t.Fatalf("not imported: %s, %v", e.Status, questions(e.Form))
	}
	if !handedOn(im, name, "The Office US/Season 02/03.mkv") {
		t.Error("not handed on")
	}
}

func TestAnswerImports(t *testing.T) {
	name := "Avatar.The.Last.Airbender.Book.1.Water"
	im := newTestImportling(t, name+"/01 The Boy in the Iceberg.avi")
	e := im.Torrents[name]
	form := url.Values{"name": {name}, "show": {"Avatar The Last Airbender"}, "0.include": {"on"}, "season": {""}, "0.episode": {"1"}}
	post(im, im.handleAnswer, form)
	if e.Status != "waiting" {
		t.Fatalf("imported with no season: %s", e.Status)
	}
	form.Set("season", "1")
	post(im, im.handleAnswer, form)
	if e.Status != "imported" || !handedOn(im, name+"/01 The Boy in the Iceberg.avi", "Avatar The Last Airbender/Season 01/01.avi") {
		t.Errorf("not imported as S01E01: %s", e.Status)
	}
}

func TestSkipAndReopen(t *testing.T) {
	name := "Fargo.1996.1080p.mkv"
	im := newTestImportling(t, name)
	e := im.Torrents[name]
	post(im, im.handleSkip, url.Values{"name": {name}})
	if e.Status != "skipped" {
		t.Fatalf("not skipped: %s", e.Status)
	}
	post(im, im.handleReopen, url.Values{"name": {name}})
	if e.Status != "waiting" {
		t.Errorf("not reopened: %s", e.Status)
	}
	if entries, _ := os.ReadDir(im.importedDir); len(entries) > 0 {
		t.Error("a skipped torrent is handed on")
	}
}

// An episode another torrent already handed on leaves the torrent waiting,
// and links none of its episodes
func TestEpisodeAlreadyHandedOn(t *testing.T) {
	im := newTestImportling(t, "Show.S01E01.mkv")
	download(t, im, "Show.S01E01.REPACK.mkv")
	e := im.Torrents["Show.S01E01.REPACK.mkv"]
	if e.Status != "waiting" || !slices.Contains(e.Form.Rows[0].Episode.Doubts, "Already handed on as Show/Season 01/01.mkv") {
		t.Errorf("got %s, %+v", e.Status, e.Form.Rows[0].Episode)
	}
	if !handedOn(im, "Show.S01E01.mkv", "Show/Season 01/01.mkv") {
		t.Error("the first torrent's episode is gone")
	}
}
