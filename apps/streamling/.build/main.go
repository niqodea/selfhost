package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"maps"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

const (
	listenPort   = "8080"
	keepFor      = 30 * 24 * time.Hour
	warnFor      = 7 * 24 * time.Hour
	rateInterval = 3 * time.Second
	// Each measurement averages with the one before, so the rate of a download
	// that stopped halves every interval and never reaches zero
	minArrivingBytesPerSecond = 1000
	// Speed swings the most as a download starts, so its first readings, about
	// 10 seconds of them, give no estimate of how long it takes
	estimateWarmupSamples = 3
	// How much each reading moves the speed behind an estimate, which makes it
	// average about the last 10 readings
	// Ref: https://en.wikipedia.org/wiki/Exponential_smoothing
	estimateSmoothing = 2.0 / (10 + 1)
)

const pagesTemplate = `
{{define "head"}}<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  :root {
    color-scheme: light dark;
    --background: light-dark(#fafafa, #18181b);
    --surface: light-dark(#ffffff, #27272a);
    --border: light-dark(#e4e4e7, #3f3f46);
    --text: light-dark(#18181b, #f4f4f5);
    --muted: light-dark(#71717a, #a1a1aa);
    --accent: light-dark(#2563eb, #3b82f6);
    --ok: light-dark(#15803d, #4ade80);
    --wait: light-dark(#b45309, #fbbf24);
    --none: light-dark(#b91c1c, #f87171);
    font: 16px/1.5 system-ui, sans-serif;
  }
  * { box-sizing: border-box; }
  body { margin: 0 auto; max-width: 48rem; padding: 1.5rem 1rem; background: var(--background); color: var(--text); }
  h2 { margin: 2.5rem 0 0.75rem; font-size: 1rem; font-weight: 600; color: var(--muted); }
  h1 { margin: 0 0 1rem; font-size: 1.25rem; font-weight: 600; overflow-wrap: anywhere; }
  nav { margin-bottom: 1.5rem; }
  nav a { color: var(--muted); text-decoration: none; }
  nav a:hover { color: var(--text); }
  .muted { color: var(--muted); }
  /* Text that the page updates in place goes in a .live slot, whose size stays
     the same whatever the text: one line, cut with an ellipsis when too long,
     with digits of equal width. Inside a slot, the parts that change most go
     last, so they move nothing. In a row, an item of fixed size gets
     flex: none, and text of varying length gets flex: 1 and min-width: 0, so
     the text is cut instead of pushing the item out of view */
  .live { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-variant-numeric: tabular-nums; }
  /* The status stays in view, and a line fades in under it as the page scrolls,
     where the browser supports scroll-driven animations */
  #status {
    position: sticky; top: 0; z-index: 1; display: flex; align-items: center; gap: 1rem;
    margin: -0.5rem 0 1rem; padding: 0.5rem 0; background: var(--background);
    border-bottom: 1px solid transparent;
    animation: stuck linear both; animation-timeline: scroll(); animation-range: 0 5rem;
  }
  @keyframes stuck { to { border-bottom-color: var(--border); } }
  #status .lines { flex: 1; min-width: 0; }
  #status form { flex: none; }
  /* One line for the whole torrent and one for the prioritized file. They sum
     up the files below them, so they stay quiet but for the dot */
  #status p { min-height: 1.5em; margin: 0; color: var(--muted); }
  #status p::before { content: ""; display: inline-block; width: 0.5rem; height: 0.5rem; margin-right: 0.5rem; border-radius: 50%; background: var(--dot); }
  #status p:empty::before { content: none; }
  #status .ok { --dot: var(--ok); }
  #status .wait { --dot: var(--wait); }
  #status .none { --dot: var(--none); }
  #status .paused { --dot: var(--muted); }
  small { display: block; font-size: 0.875rem; }
  footer { margin-top: auto; padding-top: 3rem; text-align: center; }
  input, .button, button {
    font: inherit; padding: 0.5rem 0.875rem; border: 1px solid var(--border); border-radius: 0.5rem;
    background: var(--surface); color: var(--text);
  }
  .button, button { display: inline-block; text-decoration: none; white-space: nowrap; cursor: pointer; }
  .button:hover, button:hover { border-color: var(--muted); }
  .primary { background: var(--accent); border-color: var(--accent); color: #ffffff; }
  .primary:hover { border-color: var(--accent); filter: brightness(1.1); }
  .icon { padding: 0.5rem; border: none; background: none; color: var(--muted); line-height: 0; }
  .icon:hover { color: var(--text); }
  .icon.delete:hover { color: var(--none); }
  button:disabled { opacity: 0.6; cursor: wait; }
  button[aria-pressed="true"] { color: var(--accent); border-color: var(--accent); }
  form { display: flex; gap: 0.5rem; }
  input { flex: 1; min-width: 0; }
  input:focus { outline: 2px solid var(--accent); outline-offset: -1px; }
  ul { list-style: none; margin: 0; padding: 0; border: 1px solid var(--border); border-radius: 0.75rem; background: var(--surface); overflow: hidden; }
  li { position: relative; display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem 1rem; padding: 0.75rem 1rem; }
  li + li { border-top: 1px solid var(--border); }
  /* A button looks and acts the same whatever the status, which changes on its
     own and could otherwise change a click as it lands */
  .go { display: inline-flex; align-items: center; gap: 0.375rem; padding: 0.5rem; line-height: 0; color: var(--muted); border-color: currentColor; }
  .go:hover { border-color: currentColor; filter: brightness(1.2); }
  /* Selecting a video's row prioritizes it, and the selected row stands out.
     The other files only go along with the videos, so they stay in the
     background */
  li[data-prioritize] { cursor: pointer; }
  li:not([data-prioritize]) .name { color: var(--muted); }
  li:is(.wait, .ready) { background: color-mix(in srgb, var(--accent) 10%, transparent); }
  li:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  @media (hover: hover) { li[data-prioritize]:not(.wait, .ready):hover { background: color-mix(in srgb, var(--muted) 10%, transparent); } }
  .go .copied, .go.copying .act { display: none; }
  .go.copying .copied { display: block; }
  /* The bar's color says where the file stands: prioritized and a wait,
     prioritized and ready to play, or done. It sits over a faint track of the
     same color, and moving stripes show that data arrives for the file */
  .bar {
    position: absolute; inset: auto 0 0; height: 4px; --color: var(--muted);
    --fill: linear-gradient(to right, var(--color) var(--progress), color-mix(in srgb, var(--color) 20%, transparent) 0);
    background: var(--fill);
  }
  .bar.wait { --color: var(--wait); }
  .bar.ready { --color: var(--ok); }
  .bar.done { --color: var(--accent); }
  .bar.arriving {
    background: repeating-linear-gradient(-45deg, transparent 0 4px, color-mix(in srgb, var(--surface) 40%, transparent) 4px 8px), var(--fill);
    animation: stripes 1s linear infinite;
  }
  /* One period of the stripes, 8px across at 45 degrees, is 8√2px wide */
  @keyframes stripes { to { background-position: 11.31px 0, 0 0; } }
  @media (prefers-reduced-motion: reduce) { .bar.arriving { animation: none; } }
  /* The stripes stop when the page no longer knows whether data arrives */
  .disconnected .bar.arriving { animation: none; }
  li a.name { color: inherit; text-decoration: none; }
  li a.name:hover { text-decoration: underline; }
  li .name { flex: 1; min-width: 12rem; overflow-wrap: anywhere; }
  li .actions { display: flex; flex-wrap: wrap; gap: 0.5rem; }
</style>
{{end}}
{{define "home"}}{{template "head"}}
<title>Streamling</title>
<body style="padding-top: 10vh; min-height: 100vh; display: flex; flex-direction: column">
<h1>Streamling</h1>
<form action="/add">
  <input name="magnet" placeholder="Paste a magnet link" required autofocus>
  <button class="primary">Open</button>
</form>
{{if .}}<h2>Recently opened</h2>
<ul>
  {{range .}}<li>
    <a class="name" href="{{.URL}}">{{.Name}}<small class="muted">{{if .Size}}{{.Size}} · {{.Progress}} · {{end}}{{.Age}}</small></a>
    <form method="post" action="{{.URL}}delete" onsubmit="return confirm('Delete this torrent and its files?')">
      <button class="icon delete" aria-label="Delete" title="Delete">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
      </button>
    </form>
  </li>{{end}}
</ul>
{{end}}
<footer>
  <small><a class="muted" id="handler" href="">Register to handle magnet links</a></small>
  <small class="muted" id="handler-note" style="visibility: hidden">No prompt? Your browser can't do this, or it's already set up.</small>
</footer>
<script>
  // The browser asks before sending magnet links from any site to /add. Some
  // browsers lack the function, some silently ignore it, and one may not ask
  // again once set up, so the link can only say what a missing prompt means
  // Ref: https://developer.mozilla.org/en-US/docs/Web/API/Navigator/registerProtocolHandler
  const handler = document.querySelector("#handler")
  handler.addEventListener("click", (event) => {
    event.preventDefault()
    // The note always takes its space, so showing it moves nothing
    document.querySelector("#handler-note").style.visibility = "visible"
    navigator.registerProtocolHandler?.("magnet", "/add?magnet=%s")
  })
</script>
{{end}}
{{define "files"}}{{template "head"}}
<title>{{.Name}}</title>
<nav><a href="/">← Streamling</a></nav>
<h1>{{.Name}}</h1>
<div id="status">
  <div class="lines">
    <p id="torrent-status" class="live {{.Status.State}}">{{.Status.Text}}</p>
    <p id="prioritized-status" class="live {{.PrioritizedStatus.State}}">{{.PrioritizedStatus.Text}}</p>
  </div>
  {{if .OffersAll}}<form method="post" action="all"><button aria-pressed="{{.DownloadAll}}">Download all</button></form>{{end}}
</div>
{{if .Files}}<ul>
  {{range $index, $file := .Files}}<li class="{{.Status.State}}"{{if .Video}} tabindex="0" data-index="{{$index}}" data-prioritize="{{.PrioritizeURL}}"{{end}}>
    <span class="name">{{.Path}}<small class="muted live">{{.Size}}<span data-note="{{$index}}">{{if .Status.Note}} · {{.Status.Note}}{{end}}</span></small></span>
    <span class="actions">
      {{if .Video}}
        <button class="go"{{if $.Android}} data-open="{{.AppURL}}" aria-label="Play" title="Play"{{else}} data-copy="{{.FileURL}}" aria-label="Copy link" title="Copy link"{{end}}>
          {{if $.Android}}<svg class="act" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3l14 9-14 9z"/></svg>
          {{else}}<svg class="act" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
          <svg class="copied" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>
          {{end}}
        </button>
      {{end}}
      <a class="button icon" href="{{.FileURL}}" download aria-label="Save" title="Save">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/></svg>
      </a>
    </span>
    <span class="bar {{.Status.State}}{{if .Status.Arriving}} arriving{{end}}" data-bar="{{$index}}" style="--progress: {{.Status.ProgressPercent}}%"></span>
  </li>{{end}}
</ul>{{end}}
<script>
  // A finished file has nothing to download, so the row only shows that the
  // click arrived
  function select(row) {
    if (row.matches(".done")) {
      if (!matchMedia("(prefers-reduced-motion: reduce)").matches) {
        row.animate({ backgroundColor: ["color-mix(in srgb, var(--accent) 25%, transparent)", "transparent"] }, 600)
      }
      return
    }
    const selected = row.matches(".wait, .ready")
    change(selected ? "deprioritize" : row.dataset.prioritize, row, selected ? "idle" : "wait")
  }
  // Shows the change at once, puts the rows back if it fails, and otherwise lets
  // the server have the last word
  const rows = [...document.querySelectorAll("li[data-prioritize]")]
  let changesInFlight = 0
  let changeCount = 0
  async function change(url, row, state) {
    const before = rows.map((other) => other.className)
    for (const other of rows) {
      if (other.matches(".wait, .ready")) other.className = "idle"
    }
    row.className = state
    changesInFlight++
    let succeeded = false
    try {
      succeeded = (await fetch(url, { method: "POST" })).ok
    } catch {}
    changesInFlight--
    changeCount++
    if (!succeeded) rows.forEach((other, index) => { other.className = before[index] })
    refresh()
  }
  for (const row of rows) {
    row.addEventListener("click", (event) => {
      // Selecting the file's name to copy it is not selecting the file
      if (!event.target.closest("a, button") && getSelection().isCollapsed) select(row)
    })
    row.addEventListener("keydown", (event) => {
      if (event.target === row && event.key === "Enter") select(row)
    })
  }
  // Playing or copying a file also prioritizes it, since it is what the user
  // watches next
  for (const button of document.querySelectorAll(".go")) {
    button.addEventListener("click", async () => {
      const row = button.closest("li")
      // The server ignores picking a finished file
      if (!row.matches(".done")) change(row.dataset.prioritize, row, row.matches(".ready") ? "ready" : "wait")
      if ("open" in button.dataset) {
        location.href = button.dataset.open
        return
      }
      // A player on any system can open the link, pasted into its Open URL
      const link = new URL(button.dataset.copy, location.href).href
      try {
        await navigator.clipboard.writeText(link)
      } catch {
        // Some browsers refuse the clipboard, and a prompt lets the user copy
        // the link by hand
        prompt("Copy this link", link)
        return
      }
      button.classList.add("copying")
      setTimeout(() => button.classList.remove("copying"), 1500)
    })
  }
  // Speed and peers change all the time, so the page keeps asking for them
  let failedRefreshes = 0
  async function refresh() {
    const changeCountBefore = changeCount
    let report
    try {
      const response = await fetch("status")
      if (!response.ok) throw new Error(response.statusText)
      report = await response.json()
    } catch {
      failedRefreshes++
      // A brief blip of the network changes nothing on the page
      if (failedRefreshes >= 2) showDisconnected()
      return
    }
    failedRefreshes = 0
    document.body.classList.remove("disconnected")
    // A status the server sent before a change finished would undo the change on
    // the page. The change refreshes the status again once it finishes
    if (changesInFlight > 0 || changeCount !== changeCountBefore) return
    const { torrent, prioritized, files } = report
    // The files come from peers, and the page shows them once they do
    if ({{not .Files}} && files.length) location.reload()
    for (const [id, status] of [["#torrent-status", torrent], ["#prioritized-status", prioritized]]) {
      const line = document.querySelector(id)
      line.className = "live " + status.state
      line.textContent = status.text
    }
    for (const row of rows) {
      row.className = files[row.dataset.index].state
    }
    for (const note of document.querySelectorAll("[data-note]")) {
      const text = files[note.dataset.note].note
      note.textContent = text && " · " + text
    }
    for (const bar of document.querySelectorAll("[data-bar]")) {
      const file = files[bar.dataset.bar]
      bar.className = "bar " + file.state + (file.arriving ? " arriving" : "")
      bar.style.setProperty("--progress", file.progress + "%")
    }
  }
  function showDisconnected() {
    document.body.classList.add("disconnected")
    const line = document.querySelector("#torrent-status")
    line.className = "live none"
    line.textContent = "Can't reach the server. Status may be out of date."
    const prioritizedLine = document.querySelector("#prioritized-status")
    prioritizedLine.className = "live"
    prioritizedLine.textContent = ""
  }
  setInterval(refresh, 3000)
</script>
{{end}}
`

type streamling struct {
	client *torrent.Client
	pages  *template.Template
	// Holds each torrent's magnet link, whose modification time is when the
	// torrent was last opened, its metainfo once peers send it, an empty file
	// when every one of its files downloads, and the path of the file the user
	// selected, empty when they selected none
	torrentsDir string
	// Holds a directory per torrent, named after its info hash
	downloadsDir string

	mu       sync.Mutex
	torrents map[metainfo.Hash]*torrentState
}

type torrentState struct {
	// To tell a search for peers that is still going from one that found no one
	addedAt time.Time
	// The video last picked, or nil when the user deselected it. Starts as the one
	// saved, or as the first video
	pickedFile   *torrent.File
	pickRestored bool
	downloadsAll bool
	// The file the torrent prioritizes and how many peers it has, to log when
	// they change
	prioritizedFile *torrent.File
	hasPrioritized  bool
	peerCount       int
	rate            rateMeter
	fileRates       map[*torrent.File]rateMeter
	// Seconds, once ffprobe was started on the file. Nil while it reads, and
	// when the file gives no length
	durations map[*torrent.File]*float64
}

type rateMeter struct {
	// Follows the speed closely, for telling whether data arrives
	bytesPerSecond float64
	// Stays steady, for estimating how long a download takes
	smoothedBytesPerSecond float64
	// How many readings went into the smoothed speed, since data began arriving
	arrivingSamples int
	completedBytes  int64
	started         bool
}

type torrentLink struct {
	URL        string
	Name       string
	Size       string
	Progress   string
	LastOpened time.Time
	// When it was last opened, or when it gets deleted if that is soon
	Age string
}

type filesPage struct {
	Name              string
	Status            status
	PrioritizedStatus status
	Files             []fileLinks
	// Unless the torrent is one video, whose other files are extras like samples
	OffersAll   bool
	DownloadAll bool
	// Only Android opens the app link, and any player elsewhere takes the
	// copied one
	Android bool
}

type fileLinks struct {
	Path    string
	Size    string
	FileURL string
	// An intent link, which Android opens in an installed player. Marked safe,
	// since html/template drops a link whose scheme is not http, https or mailto
	// Ref: https://developer.chrome.com/docs/android/intents
	AppURL        template.URL
	PrioritizeURL string
	Video         bool
	Status        fileStatus
}

type statusReport struct {
	Torrent     status `json:"torrent"`
	Prioritized status `json:"prioritized"`
	// In name order. Always a JSON array, since the page reads its length
	Files []fileStatus `json:"files"`
}

// A line for the user, and how good the news is
type status struct {
	Text  string      `json:"text"`
	State statusState `json:"state"`
}

// The page's stylesheet has a rule for each value
type statusState string

const (
	statusOk   statusState = "ok"
	statusWait statusState = "wait"
	statusNone statusState = "none"
	// Nothing is set to download, and nothing is being read
	statusPaused statusState = "paused"
)

type fileStatus struct {
	State fileState `json:"state"`
	// Says in words what the state's color shows, when there is more to say
	Note            string  `json:"note"`
	ProgressPercent float64 `json:"progress"`
	Arriving        bool    `json:"arriving"`
}

// The page's stylesheet and script name each value
type fileState string

const (
	// Prioritized, and it would download slower than it plays, or at an unknown rate
	fileWait fileState = "wait"
	// Prioritized, and it would download faster than it plays
	fileReady fileState = "ready"
	// Not prioritized, while every file downloads
	fileDownloading fileState = "downloading"
	// Not prioritized, and not downloading
	fileIdle fileState = "idle"
	fileDone fileState = "done"
)

func main() {
	// Go's built-in table of file types has no entry for these
	mime.AddExtensionType(".mkv", "video/x-matroska")
	mime.AddExtensionType(".avi", "video/x-msvideo")
	mime.AddExtensionType(".mov", "video/quicktime")

	torrentsDir := requireEnv("TORRENTS_DIR")
	downloadsDir := requireEnv("DOWNLOADS_DIR")
	pages, err := template.New("").Parse(pagesTemplate)
	if err != nil {
		log.Fatal(err)
	}
	config := torrent.NewDefaultClientConfig()
	// Each torrent downloads into a directory of its own, so it can be deleted
	// whole once it expires
	config.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir: downloadsDir,
		TorrentDirMaker: func(baseDir string, info *metainfo.Info, infoHash metainfo.Hash) string {
			return filepath.Join(baseDir, infoHash.HexString())
		},
	})
	config.Seed = true
	client, err := torrent.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	s := &streamling{
		client:       client,
		pages:        pages,
		torrentsDir:  torrentsDir,
		downloadsDir: downloadsDir,
		torrents:     map[metainfo.Hash]*torrentState{},
	}

	if err := s.restoreTorrents(); err != nil {
		log.Fatal(err)
	}
	go s.deleteExpired()
	go s.followDownloads()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /add", s.handleAdd)
	mux.HandleFunc("GET /{infohash}/{$}", s.handleFiles)
	mux.HandleFunc("POST /{infohash}/delete", s.handleDelete)
	mux.HandleFunc("GET /{infohash}/status", s.handleStatus)
	mux.HandleFunc("POST /{infohash}/all", s.handleAll)
	mux.HandleFunc("POST /{infohash}/deprioritize", s.handleDeprioritize)
	mux.HandleFunc("POST /{infohash}/prioritize/{path...}", s.handlePrioritize)
	mux.HandleFunc("GET /{infohash}/file/{path...}", s.handleFile)
	log.Fatal(http.ListenAndServe(":"+listenPort, mux))
}

func requireEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		log.Fatalf("%s is not set", name)
	}
	return value
}

// The torrents added before a restart carry on where they were
func (s *streamling) restoreTorrents() error {
	magnetPaths, err := filepath.Glob(filepath.Join(s.torrentsDir, "*.magnet"))
	if err != nil {
		return err
	}
	for _, magnetPath := range magnetPaths {
		var infoHash metainfo.Hash
		if err := infoHash.FromHexString(strings.TrimSuffix(filepath.Base(magnetPath), ".magnet")); err != nil {
			return err
		}
		t, err := s.addSavedTorrent(infoHash)
		if err != nil {
			return err
		}
		downloadsAll, err := fileExists(s.allPath(infoHash))
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.getState(infoHash).downloadsAll = downloadsAll
		s.mu.Unlock()
		s.follow(t)
	}
	return nil
}

// Prefers the metainfo when peers sent it, which spares asking them again
func (s *streamling) addSavedTorrent(infoHash metainfo.Hash) (*torrent.Torrent, error) {
	mi, err := metainfo.LoadFromFile(s.torrentPath(infoHash))
	if err == nil {
		return s.client.AddTorrent(mi)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	magnet, err := os.ReadFile(s.magnetPath(infoHash))
	if err != nil {
		return nil, err
	}
	return s.client.AddMagnet(string(magnet))
}

func (s *streamling) handleHome(w http.ResponseWriter, r *http.Request) {
	var torrents []torrentLink
	for _, t := range s.client.Torrents() {
		info, err := os.Stat(s.magnetPath(t.InfoHash()))
		// The magnet link of a torrent being added is not written yet
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		link := torrentLink{
			URL:        torrentURL(t.InfoHash()),
			Name:       t.Name(),
			LastOpened: info.ModTime(),
			Age:        formatAge(info.ModTime()),
		}
		// A torrent still finding its peers has no size yet
		if t.Info() != nil {
			link.Size = formatSize(t.Length())
			link.Progress = formatPercent(t)
		}
		torrents = append(torrents, link)
	}
	slices.SortFunc(torrents, func(a, b torrentLink) int {
		return b.LastOpened.Compare(a.LastOpened)
	})
	s.render(w, "home", torrents)
}

func (s *streamling) handleAdd(w http.ResponseWriter, r *http.Request) {
	magnet := r.FormValue("magnet")
	t, err := s.client.AddMagnet(magnet)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("%s: opened", t.Name())
	s.mu.Lock()
	s.getState(t.InfoHash())
	s.mu.Unlock()
	// Kept even if no peer ever shows up, so the torrent survives a restart and
	// can be retried
	saved, err := fileExists(s.magnetPath(t.InfoHash()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !saved {
		if err := writeFile(s.magnetPath(t.InfoHash()), []byte(magnet)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.follow(t)
	http.Redirect(w, r, torrentURL(t.InfoHash()), http.StatusSeeOther)
}

func (s *streamling) handleFiles(w http.ResponseWriter, r *http.Request) {
	t := s.openTorrent(w, r)
	if t == nil {
		return
	}
	report := s.getStatusReport(t)
	root := torrentURL(t.InfoHash())
	videoCount := 0
	var files []fileLinks
	for i, file := range getSortedFiles(t) {
		escaped := escapePath(file.DisplayPath())
		fileURL := root + "file/" + escaped
		video := isVideo(file.DisplayPath())
		files = append(files, fileLinks{
			Path:          file.DisplayPath(),
			Size:          formatSize(file.Length()),
			FileURL:       fileURL,
			AppURL:        template.URL("intent://" + r.Host + fileURL + "#Intent;scheme=https;type=video/*;end"),
			PrioritizeURL: root + "prioritize/" + escaped,
			Video:         video,
			Status:        report.Files[i],
		})
		if video {
			videoCount++
		}
	}
	s.mu.Lock()
	downloadsAll := s.getState(t.InfoHash()).downloadsAll
	s.mu.Unlock()
	s.render(w, "files", filesPage{
		Name:              t.Name(),
		Status:            report.Torrent,
		PrioritizedStatus: report.Prioritized,
		Files:             files,
		OffersAll:         videoCount != 1,
		DownloadAll:       downloadsAll,
		Android:           isAndroid(r),
	})
}

func (s *streamling) handleDelete(w http.ResponseWriter, r *http.Request) {
	t := s.openTorrent(w, r)
	if t == nil {
		return
	}
	if err := s.deleteTorrent(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *streamling) handleStatus(w http.ResponseWriter, r *http.Request) {
	t := s.openTorrent(w, r)
	if t == nil {
		return
	}
	report := s.getStatusReport(t)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		log.Print(err)
	}
}

func (s *streamling) handleAll(w http.ResponseWriter, r *http.Request) {
	t := s.openTorrent(w, r)
	if t == nil {
		return
	}
	s.mu.Lock()
	state := s.getState(t.InfoHash())
	downloadsAll := !state.downloadsAll
	s.mu.Unlock()
	// The file goes first, so a failure leaves the choice as it was
	if err := s.saveDownloadsAll(t.InfoHash(), downloadsAll); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	state.downloadsAll = downloadsAll
	s.mu.Unlock()
	s.prioritize(t)
	http.Redirect(w, r, torrentURL(t.InfoHash()), http.StatusSeeOther)
}

func (s *streamling) saveDownloadsAll(infoHash metainfo.Hash, downloadsAll bool) error {
	if downloadsAll {
		return writeFile(s.allPath(infoHash), nil)
	}
	return os.Remove(s.allPath(infoHash))
}

func (s *streamling) handlePrioritize(w http.ResponseWriter, r *http.Request) {
	file := s.openFile(w, r)
	if file == nil {
		return
	}
	if !isVideo(file.DisplayPath()) {
		http.Error(w, "Only a video can be prioritized", http.StatusBadRequest)
		return
	}
	s.pick(file.Torrent(), file)
	w.WriteHeader(http.StatusNoContent)
}

func (s *streamling) handleDeprioritize(w http.ResponseWriter, r *http.Request) {
	t := s.openTorrent(w, r)
	if t == nil {
		return
	}
	s.pick(t, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *streamling) handleFile(w http.ResponseWriter, r *http.Request) {
	file := s.openFile(w, r)
	if file == nil {
		return
	}
	// Reading a file leaves the prioritized video as it is, since only the user
	// picks it on the page. The reader asks peers for the pieces right after
	// where it reads before any other
	reader := file.NewReader()
	defer reader.Close()
	reader.SetContext(r.Context())
	reader.SetResponsive()
	http.ServeContent(w, r, file.DisplayPath(), time.Time{}, reader)
}

// Writes the page in one go, so a failure can still answer with an error
func (s *streamling) render(w http.ResponseWriter, name string, data any) {
	var page bytes.Buffer
	if err := s.pages.ExecuteTemplate(&page, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := page.WriteTo(w); err != nil {
		log.Print(err)
	}
}

// Marks the file's torrent as opened now. Writes the error itself and returns
// nil when there is no such file
func (s *streamling) openFile(w http.ResponseWriter, r *http.Request) *torrent.File {
	t := s.openTorrent(w, r)
	if t == nil {
		return nil
	}
	for _, file := range getSortedFiles(t) {
		if file.DisplayPath() == r.PathValue("path") {
			return file
		}
	}
	http.NotFound(w, r)
	return nil
}

// Marks the torrent as opened now. Writes the error itself and returns nil when
// there is no such torrent
func (s *streamling) openTorrent(w http.ResponseWriter, r *http.Request) *torrent.Torrent {
	var infoHash metainfo.Hash
	if err := infoHash.FromHexString(r.PathValue("infohash")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	t, ok := s.client.Torrent(infoHash)
	if !ok {
		http.Error(w, "No such torrent, add it again from the home page", http.StatusNotFound)
		return nil
	}
	// A zero time leaves the access time as it is. A failure only makes the
	// torrent expire earlier, so the request carries on
	if err := os.Chtimes(s.magnetPath(infoHash), time.Time{}, time.Now()); err != nil {
		log.Print(err)
	}
	return t
}

// The file list comes from peers, which may take a while or never happen, so
// the torrent's page shows how the search goes meanwhile. Once it comes, the
// metainfo is saved next to the magnet link
func (s *streamling) follow(t *torrent.Torrent) {
	go func() {
		select {
		case <-t.GotInfo():
		case <-t.Closed():
			return
		}
		s.prioritize(t)
		saved, err := fileExists(s.torrentPath(t.InfoHash()))
		if err != nil {
			log.Print(err)
			return
		}
		if saved {
			return
		}
		log.Printf("%s: got the list of its %d files", t.Name(), len(t.Files()))
		var data bytes.Buffer
		torrentFile := t.Metainfo()
		if err := torrentFile.Write(&data); err != nil {
			log.Print(err)
			return
		}
		if err := writeFile(s.torrentPath(t.InfoHash()), data.Bytes()); err != nil {
			log.Print(err)
		}
	}()
}

// Deletes each torrent once it has gone unopened for keepFor
func (s *streamling) deleteExpired() {
	for {
		for _, t := range s.client.Torrents() {
			info, err := os.Stat(s.magnetPath(t.InfoHash()))
			// The magnet link of a torrent being added is not written yet
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				log.Print(err)
				continue
			}
			if time.Since(info.ModTime()) < keepFor {
				continue
			}
			if err := s.deleteTorrent(t); err != nil {
				log.Print(err)
			}
		}
		time.Sleep(time.Hour)
	}
}

func (s *streamling) deleteTorrent(t *torrent.Torrent) error {
	log.Printf("%s: deleted", t.Name())
	t.Drop()
	s.mu.Lock()
	delete(s.torrents, t.InfoHash())
	s.mu.Unlock()
	// The magnet link goes first, so a crash midway never brings back a torrent
	// whose files are partly gone
	if err := os.Remove(s.magnetPath(t.InfoHash())); err != nil {
		return err
	}
	for _, name := range []string{s.torrentPath(t.InfoHash()), s.allPath(t.InfoHash()), s.pickedPath(t.InfoHash())} {
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return os.RemoveAll(filepath.Join(s.downloadsDir, t.InfoHash().HexString()))
}

// Says whether the torrent downloads at all, how the prioritized file goes, and
// for each file how much of it is here and whether it downloads
func (s *streamling) getStatusReport(t *torrent.Torrent) statusReport {
	activePeers := t.Stats().ActivePeers
	s.mu.Lock()
	state := s.getState(t.InfoHash())
	addedAt := state.addedAt
	downloadsAll := state.downloadsAll
	bytesPerSecond := state.rate.bytesPerSecond
	fileRates := maps.Clone(state.fileRates)
	s.mu.Unlock()
	if t.Info() == nil {
		return statusReport{Torrent: getSearchStatus(activePeers, addedAt), Files: []fileStatus{}}
	}
	prioritized := s.getPrioritizedFile(t)
	// The rate fades out over a few seconds once the last peer leaves
	arriving := activePeers > 0 && bytesPerSecond >= minArrivingBytesPerSecond
	durationSeconds := s.getPrioritizedDuration(prioritized)
	files := getSortedFiles(t)
	fileStatuses := make([]fileStatus, len(files))
	for i, file := range files {
		meter := fileRates[file]
		smoothedBytesPerSecond := meter.smoothedBytesPerSecond
		fileStatuses[i] = fileStatus{
			ProgressPercent: float64(file.BytesCompleted()) * 100 / float64(max(file.Length(), 1)),
			Arriving:        arriving && meter.bytesPerSecond >= minArrivingBytesPerSecond && file.BytesCompleted() < file.Length(),
		}
		remainingBytes := float64(file.Length() - file.BytesCompleted())
		switch {
		case remainingBytes == 0:
			fileStatuses[i].State = fileDone
			fileStatuses[i].Note = "Downloaded"
		case file != prioritized && downloadsAll:
			fileStatuses[i].State = fileDownloading
		case file != prioritized:
			fileStatuses[i].State = fileIdle
		// Until data has arrived for the file for a while there is no rate to
		// guess from
		case !arriving || meter.bytesPerSecond < minArrivingBytesPerSecond || meter.arrivingSamples < estimateWarmupSamples:
			fileStatuses[i].State = fileWait
		// Without its length, the wait is how long the download takes
		case durationSeconds == nil:
			fileStatuses[i].State = fileWait
			fileStatuses[i].Note = "Finishes downloading in about " + formatWait(remainingBytes/smoothedBytesPerSecond)
		case remainingBytes/smoothedBytesPerSecond <= *durationSeconds:
			fileStatuses[i].State = fileReady
			fileStatuses[i].Note = "Ready to play"
		default:
			fileStatuses[i].State = fileWait
			fileStatuses[i].Note = "Plays smoothly in about " + formatWait(remainingBytes/smoothedBytesPerSecond-*durationSeconds)
		}
	}
	return statusReport{
		Torrent:     getDownloadStatus(t, activePeers, arriving, bytesPerSecond, prioritized == nil && !downloadsAll),
		Prioritized: getPrioritizedStatus(t, prioritized, arriving, fileRates[prioritized].bytesPerSecond),
		Files:       fileStatuses,
	}
}

func getSearchStatus(activePeers int, addedAt time.Time) status {
	switch {
	case activePeers > 0:
		return status{"Getting the file list from " + formatPeers(activePeers), statusWait}
	case time.Since(addedAt) < 30*time.Second:
		return status{"Looking for peers…", statusWait}
	default:
		return status{"No one is sharing this right now", statusNone}
	}
}

// Sums up the file rows, which say how each file goes
func getDownloadStatus(t *torrent.Torrent, activePeers int, arriving bool, bytesPerSecond float64, paused bool) status {
	peers := formatPeers(activePeers)
	progress := formatProgress(t)
	switch {
	case t.BytesCompleted() == t.Length():
		return status{"Downloaded", statusOk}
	// A file being read still downloads when nothing is set to
	case paused && !arriving:
		return status{"Paused · " + progress, statusPaused}
	case activePeers == 0:
		return status{"No one is sharing this right now", statusNone}
	case !arriving:
		return status{"Connected to " + peers + ", but nothing is arriving yet", statusWait}
	default:
		return status{progress + " · " + peers + " · " + formatSize(int64(bytesPerSecond)) + "/s", statusOk}
	}
}

func getPrioritizedStatus(t *torrent.Torrent, prioritized *torrent.File, arriving bool, bytesPerSecond float64) status {
	switch {
	case t.BytesCompleted() == t.Length():
		return status{}
	// Only a video can be prioritized
	case prioritized == nil && !slices.ContainsFunc(t.Files(), func(file *torrent.File) bool {
		return isVideo(file.DisplayPath())
	}):
		return status{}
	// Says what to do, so it takes no dot
	case prioritized == nil:
		return status{Text: "Select a video to prioritize it"}
	case arriving && bytesPerSecond >= minArrivingBytesPerSecond:
		return status{"Downloading " + prioritized.DisplayPath() + " · " + formatSize(int64(bytesPerSecond)) + "/s", statusOk}
	default:
		return status{"Waiting for " + prioritized.DisplayPath(), statusWait}
	}
}

// Makes the file the prioritized one, or with nil, leaves none prioritized
func (s *streamling) pick(t *torrent.Torrent, file *torrent.File) {
	// A finished file has nothing to download, and picking it would stop the file
	// downloading now
	if file != nil && file.BytesCompleted() == file.Length() {
		return
	}
	s.mu.Lock()
	state := s.getState(t.InfoHash())
	changed := file != state.pickedFile || !state.pickRestored
	state.pickedFile = file
	state.pickRestored = true
	s.mu.Unlock()
	// A player asks for a file many times over as it plays
	if changed {
		s.savePick(t, file)
	}
	s.prioritize(t)
}

// A failure only loses the choice at the next restart, so the request carries on
func (s *streamling) savePick(t *torrent.Torrent, file *torrent.File) {
	var filePath string
	if file == nil {
		log.Printf("%s: picked nothing", t.Name())
	} else {
		filePath = file.DisplayPath()
		log.Printf("%s: picked %s", t.Name(), filePath)
	}
	if err := writeFile(s.pickedPath(t.InfoHash()), []byte(filePath)); err != nil {
		log.Print(err)
	}
}

// Sets the picked video once the file list is known: the one saved, or for a
// torrent where the user chose nothing yet, the first one. Does nothing after
// the first time
func (s *streamling) restorePick(t *torrent.Torrent, videos []*torrent.File) {
	s.mu.Lock()
	restored := s.getState(t.InfoHash()).pickRestored
	s.mu.Unlock()
	if restored {
		return
	}
	var picked *torrent.File
	data, err := os.ReadFile(s.pickedPath(t.InfoHash()))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if len(videos) > 0 {
			picked = videos[0]
		}
	case err != nil:
		log.Print(err)
	default:
		// No video matches an empty path, which is how nothing gets saved
		if index := slices.IndexFunc(videos, func(file *torrent.File) bool {
			return file.DisplayPath() == string(data)
		}); index >= 0 {
			picked = videos[index]
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.getState(t.InfoHash())
	if !state.pickRestored {
		state.pickedFile = picked
		state.pickRestored = true
	}
}

// Downloads the prioritized file before the others, which download only when
// the user asks for every file. A reader gets the pieces it reads before either
func (s *streamling) prioritize(t *torrent.Torrent) {
	prioritized := s.getPrioritizedFile(t)
	s.mu.Lock()
	state := s.getState(t.InfoHash())
	downloadsAll := state.downloadsAll
	before := state.prioritizedFile
	hadPrioritized := state.hasPrioritized
	state.prioritizedFile = prioritized
	state.hasPrioritized = true
	s.mu.Unlock()
	for _, file := range getSortedFiles(t) {
		switch {
		case file == prioritized:
			file.SetPriority(torrent.PiecePriorityHigh)
		case downloadsAll:
			file.SetPriority(torrent.PiecePriorityNormal)
		default:
			file.SetPriority(torrent.PiecePriorityNone)
		}
	}
	if hadPrioritized && prioritized == before {
		return
	}
	if before != nil && before.BytesCompleted() == before.Length() {
		log.Printf("%s: finished %s", t.Name(), before.DisplayPath())
	}
	if prioritized == nil {
		log.Printf("%s: prioritizing nothing", t.Name())
		return
	}
	log.Printf("%s: prioritizing %s", t.Name(), prioritized.DisplayPath())
}

// Returns the video last picked until it is complete. When every file
// downloads, the next unfinished video in name order takes its place, wrapping
// around to the ones before it. Returns nil when the user deselected the video,
// or no video is left to prioritize
func (s *streamling) getPrioritizedFile(t *torrent.Torrent) *torrent.File {
	videos := slices.DeleteFunc(getSortedFiles(t), func(file *torrent.File) bool {
		return !isVideo(file.DisplayPath())
	})
	if len(videos) == 0 {
		return nil
	}
	s.restorePick(t, videos)
	s.mu.Lock()
	state := s.getState(t.InfoHash())
	picked := state.pickedFile
	downloadsAll := state.downloadsAll
	s.mu.Unlock()
	start := slices.Index(videos, picked)
	// Nothing is picked
	if start < 0 {
		return nil
	}
	queue := slices.Concat(videos[start:], videos[:start])
	next := slices.IndexFunc(queue, func(file *torrent.File) bool {
		return file.BytesCompleted() < file.Length()
	})
	if next < 0 || next > 0 && !downloadsAll {
		return nil
	}
	return queue[next]
}

// Returns the length in seconds of the prioritized file, or nil when there is
// none yet. Reading a file's length fetches its pieces first, so only the
// prioritized file gets read
func (s *streamling) getPrioritizedDuration(prioritized *torrent.File) *float64 {
	if prioritized == nil || !isVideo(prioritized.DisplayPath()) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.getState(prioritized.Torrent().InfoHash())
	durationSeconds, ok := state.durations[prioritized]
	if ok {
		return durationSeconds
	}
	state.durations[prioritized] = nil
	go func() {
		fileURL := "http://127.0.0.1:" + listenPort + torrentURL(prioritized.Torrent().InfoHash()) + "file/" + escapePath(prioritized.DisplayPath()) + "?probe"
		seconds, err := readDurationSeconds(fileURL)
		if err != nil {
			log.Printf("Could not read the length of %s: %v", fileURL, err)
			return
		}
		log.Printf("Read the length of %s: %.0f s", fileURL, seconds)
		s.mu.Lock()
		state.durations[prioritized] = &seconds
		s.mu.Unlock()
	}()
	return nil
}

// Ffprobe reads the video through Streamling itself, which fetches the pieces
// it asks for first
func readDurationSeconds(fileURL string) (float64, error) {
	// Ffprobe waits on pieces no peer may have, such as an index at the end of
	// the file
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", fileURL).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
}

// Every few seconds, measures how fast each torrent and each of its files
// download, and moves the priority on once a file finishes
func (s *streamling) followDownloads() {
	for {
		time.Sleep(rateInterval)
		for _, t := range s.client.Torrents() {
			activePeers := t.Stats().ActivePeers
			s.mu.Lock()
			state := s.getState(t.InfoHash())
			peerCount := state.peerCount
			state.peerCount = activePeers
			s.mu.Unlock()
			if activePeers != peerCount {
				log.Printf("%s: peer count %d", t.Name(), activePeers)
			}
			if t.Info() == nil {
				continue
			}
			s.prioritize(t)
			s.mu.Lock()
			state.rate = state.rate.measure(t.BytesCompleted())
			for _, file := range t.Files() {
				state.fileRates[file] = state.fileRates[file].measure(file.BytesCompleted())
			}
			s.mu.Unlock()
		}
	}
}

// Returns the rate from how much more is complete than a few seconds before
func (m rateMeter) measure(completedBytes int64) rateMeter {
	if !m.started {
		return rateMeter{completedBytes: completedBytes, started: true}
	}
	// Completed bytes go down when a piece fails its check
	bytesPerSecond := float64(max(completedBytes-m.completedBytes, 0)) / rateInterval.Seconds()
	next := rateMeter{completedBytes: completedBytes, started: true}
	// Averaging with the rate before smooths out bursts
	next.bytesPerSecond = (m.bytesPerSecond + bytesPerSecond) / 2
	switch {
	// Data that arrives again later starts a new estimate
	case next.bytesPerSecond < minArrivingBytesPerSecond:
	// Starting from the first reading spares averaging it with a rate of zero
	case m.arrivingSamples == 0:
		next.smoothedBytesPerSecond = bytesPerSecond
		next.arrivingSamples = 1
	default:
		next.smoothedBytesPerSecond = m.smoothedBytesPerSecond + estimateSmoothing*(bytesPerSecond-m.smoothedBytesPerSecond)
		next.arrivingSamples = m.arrivingSamples + 1
	}
	return next
}

// Returns the torrent's state, new if the torrent has none yet. The caller holds
// the mutex
func (s *streamling) getState(infoHash metainfo.Hash) *torrentState {
	state, ok := s.torrents[infoHash]
	if ok {
		return state
	}
	state = &torrentState{
		addedAt:   time.Now(),
		fileRates: map[*torrent.File]rateMeter{},
		durations: map[*torrent.File]*float64{},
	}
	s.torrents[infoHash] = state
	return state
}

func (s *streamling) magnetPath(infoHash metainfo.Hash) string {
	return filepath.Join(s.torrentsDir, infoHash.HexString()+".magnet")
}

func (s *streamling) allPath(infoHash metainfo.Hash) string {
	return filepath.Join(s.torrentsDir, infoHash.HexString()+".all")
}

func (s *streamling) pickedPath(infoHash metainfo.Hash) string {
	return filepath.Join(s.torrentsDir, infoHash.HexString()+".picked")
}

func (s *streamling) torrentPath(infoHash metainfo.Hash) string {
	return filepath.Join(s.torrentsDir, infoHash.HexString()+".torrent")
}

func fileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Writes to a temporary file first, so a crash never leaves a partial file
// behind
func writeFile(name string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(name), ".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), name)
}

// Chromium browsers also name the system in a header of its own, which they
// send even when the user agent says less
// Ref: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Sec-CH-UA-Platform
func isAndroid(r *http.Request) bool {
	return r.Header.Get("Sec-CH-UA-Platform") == `"Android"` || strings.Contains(r.UserAgent(), "Android")
}

func isVideo(filePath string) bool {
	return strings.HasPrefix(mime.TypeByExtension(path.Ext(filePath)), "video/")
}

// Returns the torrent's files in name order, or none while it waits for its
// file list
func getSortedFiles(t *torrent.Torrent) []*torrent.File {
	if t.Info() == nil {
		return nil
	}
	files := slices.Clone(t.Files())
	slices.SortFunc(files, func(a, b *torrent.File) int {
		return compareNames(a.DisplayPath(), b.DisplayPath())
	})
	return files
}

// Compares names the way file managers do, ignoring case and with numbers by
// value, so "Episode 2" comes before "Episode 10"
func compareNames(a, b string) int {
	aRest, bRest := strings.ToLower(a), strings.ToLower(b)
	for aRest != "" && bRest != "" {
		aDigits, bDigits := getLeadingDigits(aRest), getLeadingDigits(bRest)
		if aDigits == "" || bDigits == "" {
			if aRest[0] != bRest[0] {
				return cmp.Compare(aRest[0], bRest[0])
			}
			aRest, bRest = aRest[1:], bRest[1:]
			continue
		}
		// Without leading zeros, a longer number is a larger one
		aNumber, bNumber := strings.TrimLeft(aDigits, "0"), strings.TrimLeft(bDigits, "0")
		if order := cmp.Or(cmp.Compare(len(aNumber), len(bNumber)), strings.Compare(aNumber, bNumber)); order != 0 {
			return order
		}
		aRest, bRest = aRest[len(aDigits):], bRest[len(bDigits):]
	}
	return cmp.Or(cmp.Compare(len(aRest), len(bRest)), strings.Compare(a, b))
}

func getLeadingDigits(s string) string {
	end := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		return s
	}
	return s[:end]
}

func torrentURL(infoHash metainfo.Hash) string {
	return "/" + infoHash.HexString() + "/"
}

func escapePath(filePath string) string {
	return (&url.URL{Path: filePath}).EscapedPath()
}

func formatAge(lastOpened time.Time) string {
	left := keepFor - time.Since(lastOpened)
	if left >= warnFor {
		return lastOpened.Format("Jan 2")
	}
	days := max(int(math.Ceil(left.Hours()/24)), 1)
	if days == 1 {
		return "Deleted in 1 day"
	}
	return fmt.Sprintf("Deleted in %d days", days)
}

func formatPeers(count int) string {
	if count == 1 {
		return "1 peer"
	}
	return fmt.Sprintf("%d peers", count)
}

func formatPercent(t *torrent.Torrent) string {
	return fmt.Sprintf("%d%%", t.BytesCompleted()*100/max(t.Length(), 1))
}

func formatProgress(t *torrent.Torrent) string {
	files := t.Files()
	if len(files) <= 1 {
		return formatPercent(t)
	}
	doneCount := 0
	for _, file := range files {
		if file.BytesCompleted() == file.Length() {
			doneCount++
		}
	}
	return fmt.Sprintf("%d of %d files", doneCount, len(files))
}

// Rounds up to steps coarse enough for an estimate to hold still, as in 3m or
// 1h 20m
func formatWait(seconds float64) string {
	minutes := int(math.Ceil(seconds / 60))
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	minutes = (minutes + 4) / 5 * 5
	return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
}

func formatSize(byteCount int64) string {
	units := []string{"B", "kB", "MB", "GB", "TB"}
	size := float64(byteCount)
	unit := 0
	for size >= 1000 && unit < len(units)-1 {
		size /= 1000
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", byteCount)
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}
