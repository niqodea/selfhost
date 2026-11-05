package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// A torrent goes from the inbox to downloads, seeds from there to another
// peer, and stops once it has uploaded its size
func TestSeedling(t *testing.T) {
	source := t.TempDir()
	files := map[string][]byte{
		"Show.S01/Show.S01E01.mkv": bytes.Repeat([]byte("a"), 100<<10),
		"Show.S01/Show.S01E02.mkv": bytes.Repeat([]byte("b"), 70<<10),
	}
	for name, data := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(source, name)), 0o755)
		if err := os.WriteFile(filepath.Join(source, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	info := metainfo.Info{PieceLength: 16 << 10}
	if err := info.BuildFromFilePath(filepath.Join(source, "Show.S01")); err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info)}
	infoHash := mi.HashInfoBytes()

	seeder := newClient(t, source)
	if _, err := seeder.AddTorrent(&mi); err != nil {
		t.Fatal(err)
	}

	inbox, media, state := t.TempDir(), t.TempDir(), t.TempDir()
	s, err := newSeedling(testConfig(t), inbox, media, state)
	if err != nil {
		t.Fatal(err)
	}
	defer s.client.Close()
	magnet := mi.Magnet(&infoHash, &info).String()
	if err := os.WriteFile(filepath.Join(inbox, "show.magnet"), []byte(magnet+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.scan()
	if _, err := os.Stat(filepath.Join(inbox, "show.magnet")); !os.IsNotExist(err) {
		t.Fatalf("the magnet link is still in the inbox: %v", err)
	}
	s.torrents[infoHash].AddClientPeer(seeder)
	scanUntil(t, s, func() bool { return s.entries[infoHash].Done })

	for name, data := range files {
		got, err := os.ReadFile(filepath.Join(media, "downloads", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("%s differs from the source", name)
		}
	}
	if _, err := os.Stat(filepath.Join(media, "incoming", infoHash.HexString())); !os.IsNotExist(err) {
		t.Fatalf("the torrent is still in incoming: %v", err)
	}

	leecher := newClient(t, t.TempDir())
	lt, err := leecher.AddTorrent(&mi)
	if err != nil {
		t.Fatal(err)
	}
	lt.DownloadAll()
	lt.AddClientPeer(s.client)
	scanUntil(t, s, func() bool { return s.entries[infoHash].Stopped })
	if !lt.Complete().Bool() {
		t.Fatal("seedling stopped before the other peer had the whole torrent")
	}

	// Starting again leaves a stopped torrent stopped
	s.client.Close()
	s, err = newSeedling(testConfig(t), inbox, media, state)
	if err != nil {
		t.Fatal(err)
	}
	defer s.client.Close()
	if len(s.client.Torrents()) != 0 {
		t.Fatal("a stopped torrent seeds again")
	}
}

// The testing config refuses requests of a whole block, to test the library
// itself
func testConfig(t *testing.T) *torrent.ClientConfig {
	config := torrent.TestingConfig(t)
	config.MaxAllocPeerRequestDataPerConn = torrent.NewDefaultClientConfig().MaxAllocPeerRequestDataPerConn
	return config
}

func newClient(t *testing.T, dir string) *torrent.Client {
	config := testConfig(t)
	config.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{ClientBaseDir: dir})
	config.Seed = true
	client, err := torrent.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func scanUntil(t *testing.T, s *seedling, done func() bool) {
	for start := time.Now(); !done(); s.scan() {
		if time.Since(start) > 30*time.Second {
			t.Fatal("timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// A magnet link added on the page shows in the list, and a broken one is
// refused
func TestPage(t *testing.T) {
	s, err := newSeedling(testConfig(t), t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.client.Close()
	magnet := "magnet:?xt=urn:btih:" + strings.Repeat("ab", 20) + "&dn=Some.Movie.2001"

	add := httptest.NewRecorder()
	s.handleAdd(add, httptest.NewRequest("GET", "/add?magnet="+url.QueryEscape(magnet), nil))
	if add.Code != http.StatusSeeOther {
		t.Fatalf("adding returned %d: %s", add.Code, add.Body)
	}
	home := httptest.NewRecorder()
	s.handleHome(home, httptest.NewRequest("GET", "/", nil))
	for _, want := range []string{"Some.Movie.2001", "Getting the file list"} {
		if !strings.Contains(home.Body.String(), want) {
			t.Fatalf("the page lacks %q:\n%s", want, home.Body)
		}
	}

	broken := httptest.NewRecorder()
	s.handleAdd(broken, httptest.NewRequest("GET", "/add?magnet=nonsense", nil))
	if broken.Code != http.StatusBadRequest {
		t.Fatalf("adding a broken link returned %d", broken.Code)
	}
}
