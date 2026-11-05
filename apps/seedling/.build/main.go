package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/sys/unix"
)

const (
	listenPort   = "8080"
	scanInterval = 10 * time.Second
	// A torrent stops seeding once it has uploaded this many times its size
	seedRatio = 1.0
)

// entry is a torrent added from the page or the inbox
type entry struct {
	Magnet string
	Added  time.Time
	// The torrent's top file or directory, once peers sent its metainfo
	Name string `json:",omitempty"`
	// Moved from incoming to downloads
	Done bool `json:",omitempty"`
	// Bytes sent to peers, over every run
	Uploaded int64 `json:",omitempty"`
	Stopped  bool  `json:",omitempty"`
}

type seedling struct {
	client   *torrent.Client
	pages    *template.Template
	inboxDir string
	// Holds a directory per unfinished torrent, named after its info hash, so
	// downloads only ever holds finished torrents
	incomingDir  string
	downloadsDir string
	stateDir     string
	entries      map[metainfo.Hash]*entry
	torrents     map[metainfo.Hash]*torrent.Torrent
	// What each torrent had uploaded when it was last added, since the client
	// counts from zero
	uploadedBefore map[metainfo.Hash]int64
	// Logged once, instead of at every scan
	reported map[string]bool

	mu sync.Mutex
}

func main() {
	config := torrent.NewDefaultClientConfig()
	s, err := newSeedling(config, requireEnv("INBOX_DIR"), requireEnv("MEDIA_DIR"), requireEnv("STATE_DIR"))
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			s.mu.Lock()
			s.scan()
			s.mu.Unlock()
			time.Sleep(scanInterval)
		}
	}()

	http.HandleFunc("GET /{$}", s.handleHome)
	http.HandleFunc("GET /add", s.handleAdd)
	log.Fatal(http.ListenAndServe(":"+listenPort, nil))
}

func requireEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		log.Fatalf("%s is not set", name)
	}
	return value
}

// The media directory holds incoming and downloads, which a rename needs on
// one mount
func newSeedling(config *torrent.ClientConfig, inboxDir, mediaDir, stateDir string) (*seedling, error) {
	pages, err := template.New("").Parse(pagesTemplate)
	if err != nil {
		return nil, err
	}
	s := &seedling{
		pages:          pages,
		inboxDir:       inboxDir,
		incomingDir:    filepath.Join(mediaDir, "incoming"),
		downloadsDir:   filepath.Join(mediaDir, "downloads"),
		stateDir:       stateDir,
		entries:        map[metainfo.Hash]*entry{},
		torrents:       map[metainfo.Hash]*torrent.Torrent{},
		uploadedBefore: map[metainfo.Hash]int64{},
		reported:       map[string]bool{},
	}
	for _, dir := range []string{s.incomingDir, s.downloadsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(s.statePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.entries); err != nil {
			return nil, err
		}
	}

	// A torrent with a directory in incoming downloads there, and any other
	// seeds from downloads. The library tells finished pieces from the files'
	// names and sizes, so a moved torrent seeds again without checking them
	config.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir: s.downloadsDir,
		TorrentDirMaker: func(baseDir string, info *metainfo.Info, infoHash metainfo.Hash) string {
			if _, err := os.Stat(s.incomingPath(infoHash)); err == nil {
				return s.incomingPath(infoHash)
			}
			return baseDir
		},
	})
	config.Seed = true
	s.client, err = torrent.NewClient(config)
	if err != nil {
		return nil, err
	}

	for infoHash, e := range s.entries {
		if e.Done {
			if err := s.finishMove(infoHash); err != nil {
				return nil, err
			}
		}
		if !e.Stopped {
			if err := s.add(infoHash); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}

// scan takes the magnet links in the inbox, and moves each torrent along:
// to downloads once finished, and to stopped once it has seeded enough
func (s *seedling) scan() {
	s.readInbox()
	changed := false
	for infoHash, t := range s.torrents {
		e := s.entries[infoHash]
		stats := t.Stats()
		if uploaded := s.uploadedBefore[infoHash] + stats.BytesWrittenData.Int64(); uploaded != e.Uploaded {
			e.Uploaded, changed = uploaded, true
		}
		if t.Info() == nil {
			continue
		}
		if e.Name == "" {
			if err := s.saveMetainfo(t); err != nil {
				log.Print(err)
				continue
			}
			log.Printf("%s: got the list of its %d files", t.Name(), len(t.Files()))
			e.Name, changed = t.Info().BestName(), true
			t.DownloadAll()
		}
		switch {
		case !e.Done && t.Complete().Bool():
			if err := s.move(infoHash); err != nil {
				log.Print(err)
			}
		case e.Done && float64(e.Uploaded) >= seedRatio*float64(t.Length()):
			log.Printf("%s: stopped, after uploading %d bytes", e.Name, e.Uploaded)
			t.Drop()
			delete(s.torrents, infoHash)
			e.Stopped, changed = true, true
		}
	}
	if changed {
		if err := s.saveState(); err != nil {
			log.Print(err)
		}
	}
}

// addMagnet saves the magnet link's torrent and starts it, unless it was
// added before
func (s *seedling) addMagnet(link string) error {
	magnet, err := metainfo.ParseMagnetUri(link)
	if err != nil {
		return err
	}
	if _, ok := s.entries[magnet.InfoHash]; ok {
		log.Printf("%s: already added", displayName(magnet))
		return nil
	}
	s.entries[magnet.InfoHash] = &entry{Magnet: link, Added: time.Now()}
	if err := s.saveState(); err != nil {
		delete(s.entries, magnet.InfoHash)
		return err
	}
	if err := s.add(magnet.InfoHash); err != nil {
		return err
	}
	log.Printf("%s: added", displayName(magnet))
	return nil
}

func displayName(magnet metainfo.Magnet) string {
	if magnet.DisplayName != "" {
		return magnet.DisplayName
	}
	return magnet.InfoHash.HexString()
}

// readInbox adds the torrent of each magnet link in the inbox, and removes the
// link once the torrent is saved
func (s *seedling) readInbox() {
	paths, err := filepath.Glob(filepath.Join(s.inboxDir, "*.magnet"))
	if err != nil {
		log.Print(err)
		return
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Print(err)
			continue
		}
		if err := s.addMagnet(strings.TrimSpace(string(data))); err != nil {
			if !s.reported[path] {
				log.Printf("%s: %v", path, err)
				s.reported[path] = true
			}
			continue
		}
		if err := os.Remove(path); err != nil {
			log.Print(err)
		}
	}
}

// add starts the torrent in the client, from its metainfo when peers sent it,
// which spares asking them again
func (s *seedling) add(infoHash metainfo.Hash) error {
	e := s.entries[infoHash]
	if !e.Done {
		if err := os.MkdirAll(s.incomingPath(infoHash), 0o755); err != nil {
			return err
		}
	}
	var t *torrent.Torrent
	mi, err := metainfo.LoadFromFile(s.metainfoPath(infoHash))
	switch {
	case err == nil:
		t, err = s.client.AddTorrent(mi)
	case errors.Is(err, fs.ErrNotExist):
		t, err = s.client.AddMagnet(e.Magnet)
	}
	if err != nil {
		return err
	}
	if e.Name != "" {
		t.DownloadAll()
	}
	s.torrents[infoHash] = t
	s.uploadedBefore[infoHash] = e.Uploaded
	return nil
}

// move puts the finished torrent in downloads and seeds it from there. It
// marks the torrent done first, so a restart midway finishes the move
func (s *seedling) move(infoHash metainfo.Hash) error {
	e := s.entries[infoHash]
	// A file or directory of the same name may be another torrent's, which
	// seeding from there would overwrite
	if _, err := os.Lstat(filepath.Join(s.downloadsDir, e.Name)); err == nil {
		if !s.reported[e.Name] {
			log.Printf("%s: finished, but downloads already holds that name", e.Name)
			s.reported[e.Name] = true
		}
		return nil
	}
	e.Done = true
	if err := s.saveState(); err != nil {
		e.Done = false
		return err
	}
	s.torrents[infoHash].Drop()
	delete(s.torrents, infoHash)
	if err := s.finishMove(infoHash); err != nil {
		return err
	}
	log.Printf("%s: finished, and moved to downloads", e.Name)
	return s.add(infoHash)
}

func (s *seedling) finishMove(infoHash metainfo.Hash) error {
	from := filepath.Join(s.incomingPath(infoHash), s.entries[infoHash].Name)
	to := filepath.Join(s.downloadsDir, s.entries[infoHash].Name)
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.RemoveAll(s.incomingPath(infoHash))
}

func (s *seedling) saveMetainfo(t *torrent.Torrent) error {
	var data bytes.Buffer
	mi := t.Metainfo()
	if err := mi.Write(&data); err != nil {
		return err
	}
	return writeFile(s.metainfoPath(t.InfoHash()), data.Bytes())
}

func (s *seedling) saveState() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(s.statePath(), data)
}

// writeFile replaces the file whole, so a crash never leaves half of it
func writeFile(name string, data []byte) error {
	temp := name + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, name)
}

func (s *seedling) incomingPath(infoHash metainfo.Hash) string {
	return filepath.Join(s.incomingDir, infoHash.HexString())
}

func (s *seedling) metainfoPath(infoHash metainfo.Hash) string {
	return filepath.Join(s.stateDir, infoHash.HexString()+".torrent")
}

func (s *seedling) statePath() string {
	return filepath.Join(s.stateDir, "seedling.json")
}
