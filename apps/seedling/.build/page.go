package main

import (
	"cmp"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

const pagesTemplate = `
{{define "home"}}<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Seedling</title>
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
  body { margin: 0 auto; max-width: 48rem; min-height: 100vh; display: flex; flex-direction: column; padding: 10vh 1rem 1.5rem; background: var(--background); color: var(--text); }
  h1 { margin: 0 0 1rem; font-size: 1.25rem; font-weight: 600; }
  h2 { margin: 2.5rem 0 0.75rem; font-size: 1rem; font-weight: 600; color: var(--muted); }
  .muted { color: var(--muted); }
  small { display: block; font-size: 0.875rem; }
  footer { margin-top: auto; padding-top: 3rem; text-align: center; }
  form { display: flex; gap: 0.5rem; }
  input, button { font: inherit; padding: 0.5rem 0.875rem; border: 1px solid var(--border); border-radius: 0.5rem; background: var(--surface); color: var(--text); }
  input { flex: 1; min-width: 0; }
  input:focus { outline: 2px solid var(--accent); outline-offset: -1px; }
  button { cursor: pointer; background: var(--accent); border-color: var(--accent); color: #ffffff; }
  button:hover { filter: brightness(1.1); }
  ul { list-style: none; margin: 0; padding: 0; border: 1px solid var(--border); border-radius: 0.75rem; background: var(--surface); overflow: hidden; }
  li { padding: 0.75rem 1rem; overflow-wrap: anywhere; }
  li + li { border-top: 1px solid var(--border); }
  /* A dot before the status says how good the news is */
  li small::before { content: ""; display: inline-block; width: 0.5rem; height: 0.5rem; margin-right: 0.5rem; border-radius: 50%; background: var(--dot); }
  .ok { --dot: var(--ok); }
  .wait { --dot: var(--wait); }
  .none { --dot: var(--none); }
  .stopped { --dot: var(--muted); }
</style>
<h1>Seedling</h1>
<form action="/add">
  <input name="magnet" placeholder="Paste a magnet link" required autofocus>
  <button>Add</button>
</form>
{{if .}}<h2>Torrents</h2>
<ul>
  {{range .}}<li>{{.Name}}<small class="muted {{.State}}">{{.Status}}</small></li>{{end}}
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
`

type torrentRow struct {
	Name   string
	Status string
	// The page's stylesheet has a rule for each value: ok, wait, none, stopped
	State string
	added time.Time
}

// The page shows the torrents as they are when it loads, newest first
func (s *seedling) handleHome(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	var rows []torrentRow
	for infoHash, e := range s.entries {
		rows = append(rows, s.row(infoHash, e))
	}
	s.mu.Unlock()
	slices.SortFunc(rows, func(a, b torrentRow) int { return b.added.Compare(a.added) })
	if err := s.pages.ExecuteTemplate(w, "home", rows); err != nil {
		log.Print(err)
	}
}

// A GET, since the browser sends magnet links it hands to the page as one
func (s *seedling) handleAdd(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	err := s.addMagnet(r.FormValue("magnet"))
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *seedling) row(infoHash metainfo.Hash, e *entry) torrentRow {
	row := torrentRow{Name: e.Name, added: e.Added}
	if row.Name == "" {
		magnet, _ := metainfo.ParseMagnetUri(e.Magnet)
		row.Name = displayName(magnet)
	}
	t, running := s.torrents[infoHash]
	switch {
	case e.Stopped:
		row.Status, row.State = "Seeded its size, and stopped", "stopped"
	case !running:
		row.Status, row.State = "Not running, see the logs", "none"
	case t.Info() == nil:
		row.Status, row.State = fmt.Sprintf("Getting the file list · %s", peers(t.Stats().ActivePeers)), "wait"
	case !e.Done && t.Complete().Bool():
		row.Status, row.State = "Finished, but downloads already holds this name", "none"
	case !e.Done:
		percent := 100 * float64(t.BytesCompleted()) / float64(cmp.Or(t.Length(), 1))
		row.Status, row.State = fmt.Sprintf("Downloading · %.0f%% · %s", percent, peers(t.Stats().ActivePeers)), "wait"
	default:
		percent := 100 * float64(e.Uploaded) / float64(cmp.Or(t.Length(), 1)) / seedRatio
		row.Status, row.State = fmt.Sprintf("In downloads, seeding · %.0f%% seeded · %s", percent, peers(t.Stats().ActivePeers)), "ok"
	}
	return row
}

func peers(n int) string {
	if n == 1 {
		return "1 peer"
	}
	return fmt.Sprintf("%d peers", n)
}
