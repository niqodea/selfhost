package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	listenPort   = "8080"
	scanInterval = time.Minute
)

// entry is a torrent the importer has found in the downloads
type entry struct {
	Name  string
	Found time.Time
	// "waiting", "imported", "editing" for an imported one the user reopened,
	// "skipped", or "left" for one the importer leaves alone
	Status  string
	Videos  []Reading
	LeftOut []LeftOut
	// The videos the user left out
	Excluded []string
	// About the torrent as a whole
	Doubts []Doubt
	Links  []Link
	Done   time.Time
}

type state struct {
	Torrents map[string]*entry
}

// answerLine is a line of the log of answers, which keeps what the importer
// read of a torrent and what the user answered, to improve the questions
type answerLine struct {
	Time    time.Time
	Torrent string
	// "answer" or "skip"
	Action string
	// As the importer had them when the user answered
	Read   []Reading
	Doubts []Doubt
	Answer []Reading `json:",omitempty"`
	// Why the importer refused the answer, if it did
	Refused []Doubt `json:",omitempty"`
	Status  string
}

type importer struct {
	mu sync.Mutex
	// Holds downloads, shows and movies, which a hard link needs on one mount
	mediaDir  string
	stateFile string
	state
}

func main() {
	im := &importer{
		mediaDir:  os.Getenv("MEDIA_DIR"),
		stateFile: os.Getenv("STATE_FILE"),
		state:     state{Torrents: map[string]*entry{}},
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
	http.HandleFunc("POST /edit", im.handleEdit)
	http.HandleFunc("POST /cancel", im.handleCancel)
	http.HandleFunc("POST /remove", im.handleRemove)
	log.Fatal(http.ListenAndServe(":"+listenPort, nil))
}

// scan reads the torrents found since the last scan, and imports the ones
// with nothing in doubt. Until the importer asks qBittorrent, it takes every
// entry of the downloads as a finished torrent
func (im *importer) scan() {
	entries, err := os.ReadDir(im.dir("downloads"))
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
		found := &entry{Name: e.Name(), Found: time.Now(), Videos: torrent.Videos, LeftOut: torrent.LeftOut}
		im.Torrents[e.Name()] = found
		shows, movies := im.library()
		im.apply(found, Resolve(torrent.Videos, shows, movies, false))
	}
	im.save()
}

func (im *importer) listFiles(name string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(filepath.Join(im.dir("downloads"), name), func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(im.dir("downloads"), file)
		files = append(files, File{Path: filepath.ToSlash(relative), Size: info.Size()})
		return err
	})
	return files, err
}

// apply links a torrent's plan, or leaves the torrent waiting for the user
// when the plan has no links, and reports whether it linked
func (im *importer) apply(e *entry, plan Plan) bool {
	e.Doubts, e.Links = plan.Doubts, nil
	e.Status = "waiting"
	for _, video := range e.Videos {
		if slices.ContainsFunc(video.Doubts, func(d Doubt) bool { return d.Field == "unsupported" }) {
			e.Status, e.Done = "left", time.Now()
			return false
		}
	}
	if len(plan.Links) == 0 {
		return false
	}
	if err := im.link(plan.Links); err != nil {
		e.Doubts = append(e.Doubts, Doubt{Field: "title", Question: err.Error()})
		return false
	}
	e.Status, e.Links, e.Done = "imported", plan.Links, time.Now()
	return true
}

// link makes every link, or none if one fails
func (im *importer) link(links []Link) error {
	for i, link := range links {
		to := im.dir(link.To)
		err := os.MkdirAll(filepath.Dir(to), 0o770)
		if err == nil {
			err = os.Link(im.dir(path.Join("downloads", link.From)), to)
		}
		if err != nil {
			im.unlink(links[:i])
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("Is this already in the library? It has %s", link.To)
			}
			return err
		}
	}
	return nil
}

// unlink removes links, and the directories they leave empty
func (im *importer) unlink(links []Link) {
	for _, link := range links {
		if err := os.Remove(im.dir(link.To)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Print(err)
		}
		// Up to the show's or movie's directory, which a non-empty directory
		// stops
		for dir := path.Dir(link.To); strings.Count(dir, "/") > 0; dir = path.Dir(dir) {
			if os.Remove(im.dir(dir)) != nil {
				break
			}
		}
	}
}

func (im *importer) library() (shows, movies []string) {
	return im.dirNames("shows"), im.dirNames("movies")
}

func (im *importer) dirNames(dir string) []string {
	entries, err := os.ReadDir(im.dir(dir))
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

func (im *importer) dir(relative string) string {
	return filepath.Join(im.mediaDir, filepath.FromSlash(relative))
}

// save writes the state to a new file and moves it in place, so a crash
// while writing leaves the old state whole
func (im *importer) save() {
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

func (im *importer) handlePage(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.scan()

	shows, movies := im.library()
	var waiting []card
	var finished []done
	for _, e := range im.Torrents {
		if e.Status == "waiting" || e.Status == "editing" {
			waiting = append(waiting, newCard(e, shows, movies))
		} else {
			finished = append(finished, newDone(e))
		}
	}
	// The ones the user reopened come first, since the user is on them
	slices.SortFunc(waiting, func(a, b card) int {
		if a.Editing != b.Editing {
			if a.Editing {
				return -1
			}
			return 1
		}
		return a.Found.Compare(b.Found)
	})
	slices.SortFunc(finished, func(a, b done) int { return b.Done.Compare(a.Done) })

	err := page.Execute(w, map[string]any{
		"Library": append(shows, movies...),
		"Waiting": waiting,
		"Done":    finished,
	})
	if err != nil {
		log.Print(err)
	}
}

func (im *importer) handleAnswer(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	if !ok || (e.Status != "waiting" && e.Status != "editing") {
		http.Redirect(w, r, "./", http.StatusSeeOther)
		return
	}

	var wrong []Doubt
	kind := map[string]string{"movie": "movie", "show": "episode"}[r.FormValue("kind")]
	if kind == "" {
		wrong = append(wrong, Doubt{Field: "kind", Question: "Is this a movie or a show?"})
	}
	// Each kind's tab has its own title and year, as the importer read them
	title := strings.TrimSpace(r.FormValue("title-" + r.FormValue("kind")))
	year, _ := strconv.Atoi(r.FormValue("year-" + r.FormValue("kind")))
	// Picking one of the library's titles files the torrent under it
	if place := r.FormValue("place"); place != "" && place != "new" {
		title, year = splitYear(place)
	}
	if title == "" {
		wrong = append(wrong, Doubt{Field: "title", Question: "What is the title?"})
	}
	edition := strings.TrimSpace(r.FormValue("edition"))

	var videos, answers []Reading
	var excluded []string
	for i, video := range e.Videos {
		field := func(name string) string { return strings.TrimSpace(r.FormValue(fmt.Sprintf("%s-%d", name, i))) }
		// A movie is one file, which the user picks among the torrent's
		included := field("include") != ""
		if kind == "movie" {
			included = r.FormValue("movie-file") == strconv.Itoa(i)
		}
		if !included {
			videos = append(videos, video)
			excluded = append(excluded, video.Path)
			continue
		}
		answer := Reading{Path: video.Path, Kind: kind, Title: title, Year: year, Group: video.Group}
		switch kind {
		case "movie":
			answer.Edition = edition
		case "episode":
			if field("special") != "" {
				if field("special-name") == "" {
					wrong = append(wrong, Doubt{Field: "episodes", Path: video.Path, Question: "What is the special's name?"})
				}
				answer.Season, answer.Name = 0, field("special-name")
				break
			}
			// A number the user left out or mistyped stays unknown, since a 0
			// would mean a special
			answer.Season = -1
			// The torrent's season, unless its files are of different seasons
			seasonText := r.FormValue("season")
			if field("season") != "" {
				seasonText = field("season")
			}
			if season, err := strconv.Atoi(seasonText); err == nil && season >= 0 {
				answer.Season = season
			} else {
				wrong = append(wrong, Doubt{Field: "season", Path: video.Path, Question: "Which season is this?"})
			}
			if episode, err := strconv.Atoi(field("episode")); err == nil && episode >= 0 {
				answer.Episodes = []int{episode}
			} else {
				wrong = append(wrong, Doubt{Field: "episodes", Path: video.Path, Question: "Which episode is this?"})
			}
		}
		videos = append(videos, answer)
		answers = append(answers, answer)
	}

	if kind == "movie" && len(answers) == 0 {
		wrong = append(wrong, Doubt{Field: "include", Question: "Which file is the movie?"})
	}

	line := answerLine{Time: time.Now(), Torrent: e.Name, Action: "answer", Read: e.Videos, Doubts: e.Doubts, Answer: answers, Refused: wrong}
	editing, imported := e.Status == "editing", e.Links
	e.Videos, e.Excluded = videos, excluded
	if len(wrong) > 0 {
		e.Doubts = wrong
	} else {
		// The import the user edits makes way for the new one, and comes back
		// if the new one fails
		im.unlink(imported)
		shows, movies := im.library()
		if !im.apply(e, Resolve(answers, shows, movies, true)) && editing {
			if err := im.link(imported); err != nil {
				log.Print(err)
			}
			e.Status, e.Links = "editing", imported
		}
	}
	if line.Refused == nil && e.Status != "imported" {
		line.Refused = e.Doubts
	}
	line.Status = e.Status
	im.logAnswer(line)
	im.save()
	im.redirectAfter(w, r, e)
}

// logAnswer appends a line to the log of answers, which lies next to the
// state
func (im *importer) logAnswer(line answerLine) {
	data, err := json.Marshal(line)
	if err != nil {
		log.Print(err)
		return
	}
	file, err := os.OpenFile(filepath.Join(filepath.Dir(im.stateFile), "answers.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		log.Print(err)
		return
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		log.Print(err)
	}
}

func (im *importer) handleSkip(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	if !ok || e.Status != "waiting" {
		http.Redirect(w, r, "./", http.StatusSeeOther)
		return
	}
	im.logAnswer(answerLine{Time: time.Now(), Torrent: e.Name, Action: "skip", Read: e.Videos, Doubts: e.Doubts, Status: "skipped"})
	e.Status, e.Done = "skipped", time.Now()
	im.save()
	im.redirectAfter(w, r, e)
}

// handleEdit puts a torrent back among the waiting, filled in as it was
// imported, and keeps its import until the user saves their changes
func (im *importer) handleEdit(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	switch {
	case !ok:
	case e.Status == "imported":
		e.Status, e.Doubts = "editing", nil
	case e.Status == "skipped":
		e.Status = "waiting"
	}
	im.save()
	http.Redirect(w, r, "./#"+cardID(r.FormValue("name")), http.StatusSeeOther)
}

func (im *importer) handleCancel(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	if ok && e.Status == "editing" {
		e.Status, e.Doubts = "imported", nil
		im.save()
	}
	http.Redirect(w, r, "./#"+cardID(r.FormValue("name")), http.StatusSeeOther)
}

// handleRemove takes an imported torrent out of the library, and keeps it
// among the done as skipped
func (im *importer) handleRemove(w http.ResponseWriter, r *http.Request) {
	im.mu.Lock()
	defer im.mu.Unlock()
	e, ok := im.Torrents[r.FormValue("name")]
	if ok && e.Status == "editing" {
		im.unlink(e.Links)
		e.Status, e.Links, e.Doubts, e.Done = "skipped", nil, nil, time.Now()
		im.save()
	}
	http.Redirect(w, r, "./#"+cardID(r.FormValue("name")), http.StatusSeeOther)
}

// redirectAfter shows the page at the card the user is on, or at the next
// waiting one once this one is settled, so the user keeps their place
func (im *importer) redirectAfter(w http.ResponseWriter, r *http.Request, e *entry) {
	next := e
	if e.Status != "waiting" && e.Status != "editing" {
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
