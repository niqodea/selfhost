package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	listenPort   = "8080"
	scanInterval = time.Minute
)

// entry is a torrent importling has found in the downloads
type entry struct {
	Name  string
	Found time.Time
	// "waiting", "imported", "skipped", or "left" for one importling leaves
	// alone
	Status  string
	Form    Form
	LeftOut []LeftOut
	// Why importling leaves the torrent alone
	Unsupported []string `json:",omitempty"`
	Done        time.Time
}

type state struct {
	Torrents map[string]*entry
}

type importling struct {
	mu           sync.Mutex
	downloadsDir string
	// Where importling hands each torrent it imports on, as hard links named
	// by the show, the season and the episode
	importedDir string
	stateFile   string
	state
}

func main() {
	im := &importling{
		downloadsDir: requireEnv("DOWNLOADS_DIR"),
		importedDir:  requireEnv("IMPORTED_DIR"),
		stateFile:    requireEnv("STATE_FILE"),
		state:        state{Torrents: map[string]*entry{}},
	}
	data, err := os.ReadFile(im.stateFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		log.Fatal(err)
	default:
		if err := json.Unmarshal(data, &im.state); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.MkdirAll(im.importedDir, 0o770); err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			im.mu.Lock()
			im.scan()
			im.mu.Unlock()
			time.Sleep(scanInterval)
		}
	}()

	http.HandleFunc("GET /{$}", im.handlePage)
	http.HandleFunc("POST /answer", im.handleAnswer)
	http.HandleFunc("POST /skip", im.handleSkip)
	http.HandleFunc("POST /reopen", im.handleReopen)
	log.Fatal(http.ListenAndServe(":"+listenPort, nil))
}

func requireEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		log.Fatalf("%s is not set", name)
	}
	return value
}

// scan reads the torrents found since the last scan, and imports the ones
// with nothing in doubt. It takes every entry of the downloads as a finished
// torrent, since the downloader puts only those there
func (im *importling) scan() {
	entries, err := os.ReadDir(im.downloadsDir)
	if err != nil {
		log.Print(err)
		return
	}
	for _, e := range entries {
		if _, ok := im.Torrents[e.Name()]; ok {
			continue
		}
		files, err := im.listFiles(e.Name())
		if err != nil {
			log.Print(err)
			continue
		}
		torrent := Read(files)
		found := &entry{Name: e.Name(), Found: time.Now(), Status: "waiting", LeftOut: torrent.LeftOut}
		im.Torrents[e.Name()] = found
		for _, video := range torrent.Videos {
			if video.Unsupported != "" {
				found.Unsupported = append(found.Unsupported, video.Path+": "+video.Unsupported)
			}
		}
		if len(torrent.Videos) == 0 {
			found.Unsupported = []string{"Has no video"}
		}
		if len(found.Unsupported) > 0 {
			found.Status, found.Done = "left", time.Now()
			continue
		}
		found.Form = NewForm(torrent)
		im.importSettled(found)
	}
	im.save()
}

func (im *importling) listFiles(name string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(filepath.Join(im.downloadsDir, name), func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(im.downloadsDir, file)
		files = append(files, File{Path: filepath.ToSlash(relative), Size: info.Size()})
		return err
	})
	return files, err
}

// importSettled imports a settled torrent, linking every episode or none. An
// episode already handed on, by this torrent or by another, leaves the
// torrent waiting with a question on that episode, unless it is this very
// file, as when a crash came between linking and saving
func (im *importling) importSettled(e *entry) {
	if !e.Form.Settled() {
		return
	}
	links := e.Form.Links()
	for i, link := range links {
		from := filepath.Join(im.downloadsDir, filepath.FromSlash(link.From))
		to := filepath.Join(im.importedDir, filepath.FromSlash(link.To))
		err := os.MkdirAll(filepath.Dir(to), 0o770)
		if err == nil {
			err = os.Link(from, to)
		}
		if errors.Is(err, fs.ErrExist) && sameFile(from, to) {
			continue
		}
		if err == nil {
			continue
		}
		im.unlink(links[:i])
		row := slices.IndexFunc(e.Form.Rows, func(r Row) bool { return r.Path == link.From })
		if errors.Is(err, fs.ErrExist) {
			e.Form.Rows[row].Episode = e.Form.Rows[row].Episode.doubt("Already handed on as " + link.To)
		} else {
			e.Form.Rows[row].Episode = e.Form.Rows[row].Episode.doubt(err.Error())
		}
		return
	}
	e.Status, e.Done = "imported", time.Now()
}

func sameFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

// unlink removes links, and the directories they leave empty
func (im *importling) unlink(links []Link) {
	for _, link := range links {
		if err := os.Remove(filepath.Join(im.importedDir, filepath.FromSlash(link.To))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Print(err)
		}
		for dir := path.Dir(link.To); dir != "."; dir = path.Dir(dir) {
			if os.Remove(filepath.Join(im.importedDir, filepath.FromSlash(dir))) != nil {
				break
			}
		}
	}
}

// save writes the state to a new file and moves it in place, so a crash
// while writing leaves the old state whole
func (im *importling) save() {
	data, err := json.MarshalIndent(im.state, "", "  ")
	if err == nil {
		err = os.WriteFile(im.stateFile+".new", data, 0o640)
	}
	if err == nil {
		err = os.Rename(im.stateFile+".new", im.stateFile)
	}
	if err != nil {
		log.Print(err)
	}
}

func (im *importling) handlePage(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.scan()

	var waiting []card
	var finished []done
	for _, e := range im.Torrents {
		if e.Status == "waiting" {
			waiting = append(waiting, newCard(e))
		} else {
			finished = append(finished, newDone(e))
		}
	}
	slices.SortFunc(waiting, func(a, b card) int { return a.Found.Compare(b.Found) })
	slices.SortFunc(finished, func(a, b done) int { return b.Done.Compare(a.Done) })
	if err := page.Execute(w, map[string]any{"Waiting": waiting, "Done": finished}); err != nil {
		log.Print(err)
	}
}

func (im *importling) handleAnswer(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	if !ok || e.Status != "waiting" {
		http.Redirect(w, r, "./", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e.Form = e.Form.Answer(r.PostForm)
	im.importSettled(e)
	im.save()
	im.redirectAfter(w, r, e)
}

func (im *importling) handleSkip(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	if !ok || e.Status != "waiting" {
		http.Redirect(w, r, "./", http.StatusSeeOther)
		return
	}
	e.Status, e.Done = "skipped", time.Now()
	im.save()
	im.redirectAfter(w, r, e)
}

// handleReopen puts a skipped torrent back among the waiting
func (im *importling) handleReopen(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if e, ok := im.Torrents[r.FormValue("name")]; ok && e.Status == "skipped" {
		e.Status = "waiting"
		im.save()
	}
	http.Redirect(w, r, "./#"+cardID(r.FormValue("name")), http.StatusSeeOther)
}

// redirectAfter shows the page at the card the user is on, or at the next
// waiting one once this one is settled, so the user keeps their place
func (im *importling) redirectAfter(w http.ResponseWriter, r *http.Request, e *entry) {
	next := e
	if e.Status != "waiting" {
		next = nil
		for _, other := range im.Torrents {
			if other.Status == "waiting" && other.Found.Compare(e.Found) >= 0 && other != e && (next == nil || other.Found.Before(next.Found)) {
				next = other
			}
		}
	}
	if next == nil {
		http.Redirect(w, r, "./", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "./#"+cardID(next.Name), http.StatusSeeOther)
}
