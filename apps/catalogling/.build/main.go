package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	listenPort   = "8080"
	scanInterval = time.Minute
)

// Filing is a move of a waiting show's files into the library
type Filing struct {
	// The waiting show's name, and the library's show it was filed under
	Name  string
	Show  string
	Files []string
	Time  time.Time
}

type state struct {
	// The shows each name the user filed under another show means, by the
	// key of the name
	Aliases map[string][]string
	Filings []Filing
}

type catalogling struct {
	mu sync.Mutex
	// Holds a directory per waiting show, named as the torrents call it, laid
	// out as the library is, as in The Office US/Season 02/03.mkv
	waitingDir string
	// Holds a directory per show of the library, named after it
	showsDir  string
	stateFile string
	state
}

func main() {
	c := &catalogling{
		waitingDir: requireEnv("WAITING_DIR"),
		showsDir:   requireEnv("SHOWS_DIR"),
		stateFile:  requireEnv("STATE_FILE"),
		state:      state{Aliases: map[string][]string{}},
	}
	data, err := os.ReadFile(c.stateFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		log.Fatal(err)
	default:
		if err := json.Unmarshal(data, &c.state); err != nil {
			log.Fatal(err)
		}
	}
	for _, dir := range []string{c.waitingDir, c.showsDir} {
		if err := os.MkdirAll(dir, 0o770); err != nil {
			log.Fatal(err)
		}
	}

	go func() {
		for {
			c.mu.Lock()
			c.scan()
			c.mu.Unlock()
			time.Sleep(scanInterval)
		}
	}()

	http.HandleFunc("GET /{$}", c.handlePage)
	http.HandleFunc("POST /file", c.handleFile)
	http.HandleFunc("POST /discard", c.handleDiscard)
	log.Fatal(http.ListenAndServe(":"+listenPort, nil))
}

func requireEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		log.Fatalf("%s is not set", name)
	}
	return value
}

// scan files each waiting show whose name means one show of the library
func (c *catalogling) scan() {
	catalog := c.catalog()
	for _, name := range c.waiting() {
		if found := catalog.Find(name); len(found) == 1 {
			c.file(name, found[0])
		}
	}
	c.save()
}

// waiting are the names of the waiting shows
func (c *catalogling) waiting() []string {
	return dirNames(c.waitingDir)
}

func (c *catalogling) catalog() Catalog {
	return Catalog{Shows: dirNames(c.showsDir), Aliases: c.Aliases}
}

func dirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Print(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// files are the paths of a waiting show's files, relative to its directory
func (c *catalogling) files(name string) []string {
	root := filepath.Join(c.waitingDir, name)
	var files []string
	err := filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, file)
		files = append(files, filepath.ToSlash(relative))
		return err
	})
	if err != nil {
		log.Print(err)
	}
	return files
}

// file moves a waiting show's files into the show of the library, and keeps
// the waiting name, when another, as an alias of the show. A file the
// library already has stays where it is, so the page shows it. Only
// catalogling writes the library, so nothing else can take a file's place
// between the check and the move
func (c *catalogling) file(name, show string) {
	var moved []string
	for _, file := range c.files(name) {
		to := filepath.Join(c.showsDir, show, filepath.FromSlash(file))
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		err := os.MkdirAll(filepath.Dir(to), 0o770)
		if err == nil {
			err = os.Rename(filepath.Join(c.waitingDir, name, filepath.FromSlash(file)), to)
		}
		if err != nil {
			log.Print(err)
			continue
		}
		moved = append(moved, file)
	}
	removeEmptyDirs(filepath.Join(c.waitingDir, name))
	if key(name) != key(show) && !slices.Contains(c.Aliases[key(name)], show) {
		c.Aliases[key(name)] = append(c.Aliases[key(name)], show)
	}
	if len(moved) > 0 {
		c.Filings = append(c.Filings, Filing{name, show, moved, time.Now()})
	}
}

// removeEmptyDirs removes the directories under root that hold no file, and
// root itself when it holds none
func removeEmptyDirs(root string) {
	var dirs []string
	filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, file)
		}
		return nil
	})
	// Deepest first, so a directory is empty once its own are removed
	for _, dir := range slices.Backward(dirs) {
		os.Remove(dir)
	}
}

// save writes the state to a new file and moves it in place, so a crash
// while writing leaves the old state whole
func (c *catalogling) save() {
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err == nil {
		err = os.WriteFile(c.stateFile+".new", data, 0o640)
	}
	if err == nil {
		err = os.Rename(c.stateFile+".new", c.stateFile)
	}
	if err != nil {
		log.Print(err)
	}
}

func (c *catalogling) handlePage(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scan()
	catalog := c.catalog()
	var waiting []card
	for _, name := range c.waiting() {
		waiting = append(waiting, c.newCard(name, catalog))
	}
	filings := slices.Clone(c.Filings)
	slices.Reverse(filings)
	slices.Sort(catalog.Shows)
	if err := page.Execute(w, map[string]any{"Shows": catalog.Shows, "Waiting": waiting, "Filings": filings}); err != nil {
		log.Print(err)
	}
}

// handleFile files a waiting show under the show the user named, as the
// library spells it when it has the show, or as a new show
func (c *catalogling) handleFile(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	name, show := r.FormValue("name"), strings.TrimSpace(r.FormValue("show"))
	if slices.Contains(c.waiting(), name) && show != "" && !strings.ContainsAny(show, `/\`) && show != "." && show != ".." {
		c.file(name, c.catalog().Spell(show))
		// The show or the name just filed may settle other waiting shows
		c.scan()
	}
	http.Redirect(w, r, "./", http.StatusSeeOther)
}

// handleDiscard removes a waiting show, which only removes its links, since
// the downloads keep their files
func (c *catalogling) handleDiscard(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if name := r.FormValue("name"); slices.Contains(c.waiting(), name) {
		if err := os.RemoveAll(filepath.Join(c.waitingDir, name)); err != nil {
			log.Print(err)
		}
	}
	http.Redirect(w, r, "./", http.StatusSeeOther)
}
