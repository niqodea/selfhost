package main

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"path"
	"slices"
	"strconv"
	"strings"
)

// The page follows these rules:
//   - Nothing moves when the user picks something. A field that does not
//     apply to the kind picked keeps its place, hidden, and the alternatives
//     of a cell, like a date in place of a season and an episode, share its
//     space
//   - A field in doubt starts empty, so that importing takes the user's
//     answer and never the importer's guess, which the field offers to fill
//     in with one click instead
//   - Every field has a label of its own, since a placeholder goes away as
//     the user types
//     Ref: https://www.nngroup.com/articles/form-design-placeholders/
//   - A path that does not fit loses the middle of its directories and keeps
//     its file's name, which tells it apart from the others
//     Ref: https://vercel.com/geist/middle-truncate
const pageTemplate = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Importer</title>
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
    --ask: light-dark(#b45309, #fbbf24);
    --danger: light-dark(#b91c1c, #f87171);
    --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font: 16px/1.5 system-ui, sans-serif;
  }
  * { box-sizing: border-box; }
  body { margin: 0 auto; max-width: 60rem; padding: 1.5rem 1rem 4rem; background: var(--background); color: var(--text); }
  h1 { margin: 0 0 1rem; font-size: 1.25rem; font-weight: 600; }
  h2 { margin: 2.5rem 0 0.75rem; font-size: 1rem; font-weight: 600; color: var(--muted); }
  .muted { color: var(--muted); }
  .mono { font-family: var(--mono); font-size: 0.875rem; }
  input, button { font: inherit; color: var(--text); }
  input:not([type=checkbox], [type=radio]) {
    width: 100%; padding: 0.375rem 0.5rem; border: 1px solid var(--border); border-radius: 0.375rem; background: var(--surface);
  }
  input:focus-visible { outline: 2px solid var(--accent); outline-offset: -1px; }
  button { padding: 0.5rem 1rem; border: 1px solid var(--border); border-radius: 0.5rem; background: var(--surface); cursor: pointer; white-space: nowrap; }
  button:hover { border-color: var(--muted); }
  .primary { background: var(--accent); border-color: var(--accent); color: #ffffff; font-weight: 600; }
  .primary:hover { border-color: var(--accent); filter: brightness(1.1); }
  .quiet { border-color: transparent; background: none; color: var(--muted); }
  .quiet:hover { border-color: transparent; color: var(--text); text-decoration: underline; }
  .quiet.danger:hover { color: var(--danger); }

  .card { margin-bottom: 1rem; padding: 1rem 1.25rem; border: 1px solid var(--border); border-radius: 0.75rem; background: var(--surface); scroll-margin-top: 1rem; }
  .card header { display: flex; gap: 1rem; align-items: baseline; margin-bottom: 1rem; }
  .card header .mono { flex: 1; min-width: 0; overflow-wrap: anywhere; font-weight: 600; }
  .count, .editing { flex: none; font-size: 0.875rem; font-weight: 600; }
  .count { color: var(--ask); }
  .editing { color: var(--accent); }

  /* What the user has to settle: the field, a mark next to its label, and the
     question under it */
  .ask { border-color: var(--ask) !important; box-shadow: 0 0 0 1px var(--ask); background: color-mix(in srgb, var(--ask) 8%, var(--surface)) !important; }
  .mark { display: inline-grid; place-items: center; width: 1.1rem; height: 1.1rem; margin-left: 0.25rem; border-radius: 50%; background: var(--ask); color: var(--surface); font-size: 0.75rem; font-weight: 700; vertical-align: 0.1em; }
  .questions { margin: 0.25rem 0 0; padding: 0; list-style: none; color: var(--ask); font-size: 0.875rem; }
  .suggest { margin: 0.25rem 0.25rem 0 0; padding: 0.125rem 0.5rem; border: 1px dashed var(--ask); border-radius: 1rem; background: none; color: var(--text); font-size: 0.8125rem; }
  .suggest:hover { border-style: solid; }

  .tabs { display: inline-flex; border: 1px solid var(--border); border-radius: 0.5rem; overflow: hidden; }
  .tabs label { position: relative; padding: 0.375rem 1rem; cursor: pointer; }
  .tabs label + label { border-left: 1px solid var(--border); }
  .tabs input { position: absolute; opacity: 0; }
  .tabs label:has(:checked) { background: var(--accent); color: #ffffff; }
  .tabs label:has(:focus-visible) { outline: 2px solid var(--accent); outline-offset: -2px; }
  .kind { margin-bottom: 1rem; }

  /* The fields of the kind picked. Until the user picks one, the panel keeps
     its space and says to pick */
  .panel { position: relative; }
  .pick { display: none; position: absolute; inset: 0; padding-top: 1rem; text-align: center; color: var(--muted); }
  form:not(:has([name=kind]:checked)) .panel > :not(.pick) { visibility: hidden; }
  form:not(:has([name=kind]:checked)) .pick { display: block; }
  .off { visibility: hidden; }

  .fields { display: grid; grid-template-columns: minmax(12rem, 1fr) 5.5rem 10rem; gap: 1rem; align-items: start; margin-bottom: 1rem; }
  .field > span { display: block; margin-bottom: 0.25rem; font-size: 0.875rem; color: var(--muted); }
  .check { display: inline-flex; gap: 0.375rem; align-items: center; cursor: pointer; }
  .places { display: flex; flex-direction: column; gap: 0.25rem; margin-bottom: 1rem; padding: 0.5rem 0.75rem; border: 1px solid var(--border); border-radius: 0.5rem; }

  table { width: 100%; border-collapse: collapse; table-layout: fixed; margin-bottom: 0.5rem; }
  th { padding: 0 0.5rem 0.25rem; font-size: 0.875rem; font-weight: 400; color: var(--muted); text-align: left; }
  td { padding: 0.5rem; border-top: 1px solid var(--border); vertical-align: top; }
  col.include { width: 3.5rem; }
  col.special { width: 4.5rem; }
  col.numbers { width: 9rem; }
  .episode { display: flex; gap: 0.5rem; align-items: center; }
  td.include, td.special, th.include, th.special { text-align: center; }
  td.include, td.special { padding-top: 0.75rem; }
  td.file { overflow-wrap: anywhere; }
  form:has([name=kind][value=show]:checked) tr:has([name^=include]:not(:checked)) td.file .mono,
  form:has([name=kind][value=movie]:checked) tr:has([name=movie-file]:not(:checked)) td.file .mono { color: var(--muted); text-decoration: line-through; }
  td.include .stack { justify-items: center; }
  /* A cell's alternatives lie on top of each other, so the cell keeps the
     size of the largest */
  .stack { display: grid; }
  .stack > * { grid-area: 1 / 1; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; }
  .left-out { margin: 0 0 1rem; padding: 0; list-style: none; font-size: 0.875rem; color: var(--muted); }

  .actions { display: flex; gap: 0.5rem; align-items: center; }
  .actions .end { margin-left: auto; }

  .done header { margin-bottom: 0.5rem; }
  .status { flex: none; font-size: 0.875rem; }
  .imported { color: var(--ok); }
  details { margin-top: 0.5rem; }
  summary { cursor: pointer; color: var(--muted); font-size: 0.875rem; }
  .group { margin: 0.5rem 0 0 1rem; }
  .group > div { font-weight: 600; font-size: 0.875rem; }
  .links { margin: 0.25rem 0 0; padding: 0; list-style: none; }
  .links li { padding: 0.25rem 0; }
  /* One line, which loses the middle of the directories when too long */
  .path { display: flex; min-width: 0; white-space: nowrap; }
  .path .head { overflow: hidden; text-overflow: ellipsis; }
  .path .tail { flex: none; }
  .from { color: var(--muted); font-size: 0.8125rem; }

  @media (max-width: 40rem) {
    .fields { grid-template-columns: 1fr 5.5rem; }
    .fields .edition-field { grid-column: 1 / -1; }
    thead { display: none; }
    table, tbody, tr, td { display: block; }
    tr { display: grid; grid-template-columns: 2.5rem 1fr; gap: 0.25rem 0.5rem; padding: 0.5rem 0; border-top: 1px solid var(--border); }
    td { border: none; padding: 0; }
    td.include { grid-row: 1 / 4; }
    td.special, td.numbers { grid-column: 2; text-align: left; }
    td.special::after { content: " Special"; }
    /* The column's header, which a phone has no room for */
    td.numbers::before { display: block; font-size: 0.875rem; color: var(--muted); content: "Episode"; }
    tr:has(input.special:checked) td.numbers::before { content: "The special's name"; }
    form:has([name=kind][value=movie]:checked) td.numbers::before { content: none; }
  }
</style>
<body>
<h1>Importer</h1>
<datalist id="library">{{range .Library}}<option value="{{.}}">{{end}}</datalist>

<h2>Waiting for you</h2>
{{range .Waiting}}<form class="card" id="{{.ID}}" method="post" action="answer" autocomplete="off">
  <input type="hidden" name="name" value="{{.Name}}">
  <header>
    <span class="mono">{{.Name}}</span>
    {{if .Editing}}<span class="editing">Imported · your changes replace it</span>{{end}}
    {{if .Count}}<span class="count">{{.Count}} to settle</span>{{end}}
  </header>

  <div class="kind">
    <span class="tabs{{if .KindAsk}} ask{{end}}" role="radiogroup" aria-label="Kind">
      <label><input type="radio" name="kind" value="movie" required{{if eq .Kind "movie"}} checked{{end}}>Movie</label>
      <label><input type="radio" name="kind" value="show"{{if eq .Kind "show"}} checked{{end}}>Show</label>
    </span>
    {{if .KindAsk}}<i class="mark" aria-label="To settle">?</i>{{end}}
    {{template "questions" .KindAsk}}
  </div>

  <div class="panel">
    <div class="pick">Pick Movie or Show first.</div>
    <div class="fields">
      <div class="stack">{{template "title" .Movie}}{{template "title" .Show}}</div>
      <div class="stack">
        <label class="field" data-for="movie"><span>Year</span><input name="year-movie" value="{{.Movie.Year}}" inputmode="numeric" pattern="[0-9]{4}"></label>
        <label class="field" data-for="show"><span>Year</span><input name="year-show" value="{{.Show.Year}}" inputmode="numeric" pattern="[0-9]{4}"></label>
      </div>
      <div class="stack">
        <label class="field" data-for="movie">
          <span>Edition{{if .Movie.Ask.edition}}<i class="mark" aria-label="To settle">?</i>{{end}}</span>
          <input name="edition" value="{{.Edition}}" placeholder="like Extended"{{if .Movie.Ask.edition}} class="ask"{{end}}>
          {{template "questions" .Movie.Ask.edition}}
        </label>
        {{if .MultiSeason}}<span data-for="show"></span>{{else}}<label class="field" data-for="show">
          <span>Season{{if .Show.Ask.season}}<i class="mark" aria-label="To settle">?</i>{{end}}</span>
          <input name="season" value="{{.Season}}" inputmode="numeric"{{if .Show.Ask.season}} class="ask"{{end}}>
          {{template "suggestions" .SuggestSeason}}
        </label>{{end}}
      </div>
    </div>
    <div class="stack">
      <span data-for="movie">{{template "questions" .Movie.Ask.include}}</span>
      <span data-for="show">{{template "questions" .Show.Ask.season}}{{template "questions" .Show.Ask.episodes}}</span>
    </div>

    {{template "places" (places "show" .NearShows)}}{{template "places" (places "movie" .NearMovies)}}

    <table>
      <colgroup><col class="include"><col><col class="special"><col class="numbers"></colgroup>
      <thead><tr>
        <th class="include">Import</th>
        <th>File</th>
        <th class="special" data-for="show">Special</th>
        <th><span data-for="show">Episode</span></th>
      </tr></thead>
      <tbody>
      {{range .Rows}}<tr data-row>
        <td class="include"><span class="stack">
          <input type="checkbox" name="include-{{.Index}}" aria-label="Import this file" data-for="show"{{if .Include}} checked{{end}}>
          <input type="radio" name="movie-file" value="{{.Index}}" aria-label="This file is the movie" data-for="movie" required{{if .MovieFile}} checked{{end}}>
        </span></td>
        <td class="file">
          <span class="mono">{{.Path}}</span>
          <span data-for="show">{{template "questions" .Ask.episodes}}{{template "questions" .Ask.special}}{{template "suggestions" .Suggest}}</span>
        </td>
        <td class="special"><input type="checkbox" class="special{{if .Ask.special}} ask{{end}}" name="special-{{.Index}}" aria-label="A special"{{if .Special}} checked{{end}}></td>
        <td class="numbers"><span class="stack">
          <span class="for-numbers episode">
            {{if .Season}}<span class="muted">S{{.Season}}</span><input type="hidden" name="season-{{.Index}}" value="{{.Season}}">{{end}}
            <input name="episode-{{.Index}}" value="{{.Episode}}" inputmode="numeric" aria-label="Episode"{{if .Ask.episodes}} class="ask"{{end}}>
          </span>
          <span class="for-name"><input name="special-name-{{.Index}}" value="{{.SpecialName}}" aria-label="The special's name" placeholder="The special's name"></span>
        </span></td>
      </tr>{{end}}
      </tbody>
    </table>
    {{if .LeftOut}}<ul class="left-out">{{range .LeftOut}}<li>Left out: <span class="mono">{{.Path}}</span> · {{.Reason}}</li>{{end}}</ul>{{end}}
  </div>

  <div class="actions">
    {{if .Editing}}
      <button class="primary">Save</button>
      <button class="quiet" formaction="cancel" formnovalidate>Cancel</button>
      <button class="quiet danger end" formaction="remove" formnovalidate>Remove from the library</button>
    {{else}}
      <button class="primary">Import</button>
      <button class="quiet" formaction="skip" formnovalidate>Skip</button>
    {{end}}
  </div>
</form>{{else}}<p class="muted">Nothing to settle.</p>{{end}}

<h2>Done</h2>
{{range .Done}}<div class="card done" id="{{.ID}}">
  <header>
    <span class="mono">{{.Name}}</span>
    {{if eq .Status "imported"}}<span class="status imported">Imported {{.When}}</span>
    {{else if eq .Status "left"}}<span class="status muted">Left alone {{.When}}</span>
    {{else}}<span class="status muted">Skipped {{.When}}</span>{{end}}
    {{if ne .Status "left"}}<form method="post" action="edit">
      <input type="hidden" name="name" value="{{.Name}}">
      <button class="quiet">Edit</button>
    </form>{{end}}
  </header>
  {{if .Reasons}}<ul class="left-out">{{range .Reasons}}<li>{{.}}</li>{{end}}</ul>{{end}}
  {{range .Groups}}<div class="group">
    <div>{{.Dir}}</div>
    <ul class="links">{{range .Links}}<li>
      <div class="path mono" title="{{.To}}">{{template "path" .Place}}</div>
      <div class="path mono from" title="{{.From}}">{{template "path" .Source}}</div>
    </li>{{end}}</ul>
  </div>{{end}}
  {{if .LeftOut}}<details><summary>{{len .LeftOut}} left out</summary><ul class="links">{{range .LeftOut}}<li>
    <div class="path mono" title="{{.Path}}">{{template "path" (split .Path)}}</div>
    <div class="from">{{.Reason}}</div>
  </li>{{end}}</ul></details>{{end}}
</div>{{else}}<p class="muted">Nothing imported yet.</p>{{end}}

<script>
  // Hides the fields that do not apply, which keep their place, and disables
  // them, so the form does not send them
  for (const form of document.querySelectorAll("form.card")) {
    const update = () => {
      const kind = form.querySelector("[name=kind]:checked")?.value ?? ""
      for (const element of form.querySelectorAll("[data-for]")) {
        set(element, element.dataset.for.split(" ").includes(kind))
      }
      for (const row of form.querySelectorAll("tr[data-row]")) {
        const included = kind === "movie" ? row.querySelector("[name=movie-file]").checked : row.querySelector("[name^=include]").checked
        const special = row.querySelector("input.special")
        set(row.querySelector("td.special"), included && kind === "show")
        set(row.querySelector(".for-numbers"), included && kind === "show" && !special.checked)
        set(row.querySelector(".for-name"), included && kind === "show" && special.checked)
      }
    }
    form.addEventListener("change", update)
    update()
  }
  function set(element, on) {
    element.classList.toggle("off", !on)
    for (const input of element.matches("input") ? [element] : element.querySelectorAll("input")) input.disabled = !on
  }
  // A suggestion fills in its field
  for (const button of document.querySelectorAll(".suggest")) {
    button.addEventListener("click", () => {
      const input = button.form.elements[button.dataset.field]
      if (input.type === "checkbox") input.checked = true
      else input.value = button.dataset.value
      input.dispatchEvent(new Event("change", { bubbles: true }))
    })
  }
</script>
{{define "questions"}}{{if .}}<ul class="questions">{{range .}}<li>{{.}}</li>{{end}}</ul>{{end}}{{end}}
{{define "suggestions"}}{{range .}}<button type="button" class="suggest" data-field="{{.Field}}" data-value="{{.Value}}"{{if .For}} data-for="{{.For}}"{{end}}>Use {{.Label}}</button>{{end}}{{end}}
{{define "title"}}<label class="field" data-for="{{.Kind}}">
  <span>Title{{if .Ask.title}}<i class="mark" aria-label="To settle">?</i>{{end}}</span>
  <input name="title-{{.Kind}}" value="{{.Title}}" list="library" required{{if .Ask.title}} class="ask"{{end}}>
  {{template "questions" .Ask.title}}{{template "suggestions" .SuggestTitle}}
</label>{{end}}
{{define "places"}}{{if .Near}}<div class="places ask" data-for="{{.Kind}}">
  <span>Put in<i class="mark" aria-label="To settle">?</i></span>
  {{range .Near}}<label class="check"><input type="radio" name="place" value="{{.}}" required> {{.}}, in the library</label>{{end}}
  <label class="check"><input type="radio" name="place" value="new"> A new one, with the title above</label>
  <ul class="questions"><li>Which one is this? The library has a {{.Kind}} of a close title</li></ul>
</div>{{end}}{{end}}
{{define "path"}}<span class="head">{{.Head}}</span><span class="tail">{{.Tail}}</span>{{end}}
`

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"split":  splitPath,
	"places": func(kind string, near []string) map[string]any { return map[string]any{"Kind": kind, "Near": near} },
}).Parse(pageTemplate))

// cardID names a torrent's card on the page, since a torrent's name can hold
// spaces, which an id cannot
func cardID(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return fmt.Sprintf("t%08x", h.Sum32())
}

// questions are the questions about each field, which the page shows next to
// the field and highlights it by
type questions map[string][]string

func (q questions) add(d Doubt) {
	if !slices.Contains(q[d.Field], d.Question) {
		q[d.Field] = append(q[d.Field], d.Question)
	}
}

// suggestion fills a field in doubt with the importer's guess, at the user's
// click
type suggestion struct {
	// The form's name for the field
	Field        string
	Value, Label string
	// The kind the field belongs to, if one
	For string
}

// tab is the fields of one kind, which the kind's tab shows
type tab struct {
	// "movie" or "show"
	Kind         string
	Title, Year  string
	SuggestTitle []suggestion
	Ask          questions
}

type card struct {
	*entry
	ID      string
	Editing bool
	// "movie", "show", or empty when in doubt
	Kind    string
	KindAsk []string
	// The reading of the torrent as a movie and as a show, which the user
	// picks one of by picking the kind
	Movie, Show   tab
	Edition       string
	Season        string
	SuggestSeason []suggestion
	// Whether the files are of different seasons, which each row then shows
	MultiSeason bool
	// The library's shows and movies of a title close to this one's
	NearShows, NearMovies []string
	Rows                  []row
	Count                 int
}

// row is a video of a waiting torrent, which the form refers to by index
type row struct {
	Index   int
	Path    string
	Include bool
	// Whether this is the file picked as the movie
	MovieFile bool
	// Only when the files are of different seasons
	Season      string
	Episode     string
	Special     bool
	SpecialName string
	Suggest     []suggestion
	Ask         questions
}

// The fields of each kind, which its tab holds
var (
	movieFields = []string{"edition", "include"}
	showFields  = []string{"season", "episodes", "special"}
)

// newCard fills a waiting torrent's form with what was read, and leaves the
// fields in doubt empty
func newCard(e *entry, shows, movies []string) card {
	c := card{entry: e, ID: cardID(e.Name), Editing: e.Status == "editing"}
	c.Movie = tab{Kind: "movie", Ask: questions{}}
	c.Show = tab{Kind: "show", Ask: questions{}}
	kindAsk := questions{}

	// A video's readings as a movie and as an episode
	var asMovie, asEpisode []Reading
	for _, video := range e.Videos {
		switch {
		case video.Kind == "episode":
			asEpisode = append(asEpisode, video)
			asMovie = append(asMovie, Reading{Path: video.Path, Kind: "movie", Title: video.Title, Year: video.Year})
		case video.AsEpisode != nil:
			asMovie = append(asMovie, video)
			asEpisode = append(asEpisode, *video.AsEpisode)
		default:
			asMovie = append(asMovie, video)
			asEpisode = append(asEpisode, Reading{Path: video.Path, Kind: "episode", Title: video.Title, Year: video.Year, Season: -1})
		}
		for _, d := range video.Doubts {
			if d.Field == "kind" {
				kindAsk.add(d)
			}
		}
	}
	c.KindAsk = kindAsk["kind"]
	switch {
	case c.KindAsk != nil:
	case len(e.Videos) > 0 && e.Videos[0].Kind == "episode":
		c.Kind = "show"
	case len(e.Videos) > 0:
		c.Kind = "movie"
	}

	rowAsk := map[string]questions{}
	ask := func(d Doubt, t *tab) {
		switch {
		case d.Field == "kind":
		case d.Path != "" && slices.Contains(showFields, d.Field) && d.Field != "season":
			if rowAsk[d.Path] == nil {
				rowAsk[d.Path] = questions{}
			}
			rowAsk[d.Path].add(d)
		case d.Field == "title":
			t.Ask.add(d)
		case slices.Contains(movieFields, d.Field):
			c.Movie.Ask.add(d)
		case slices.Contains(showFields, d.Field):
			c.Show.Ask.add(d)
		}
	}
	for i := range e.Videos {
		for _, d := range asMovie[i].Doubts {
			ask(d, &c.Movie)
		}
		for _, d := range asEpisode[i].Doubts {
			d.Path = asEpisode[i].Path
			ask(d, &c.Show)
		}
	}
	for _, d := range e.Doubts {
		t := &c.Movie
		if c.Kind == "show" {
			t = &c.Show
		}
		ask(d, t)
	}

	fillTab(&c.Movie, asMovie)
	fillTab(&c.Show, asEpisode)
	for _, video := range asMovie {
		if c.Edition == "" {
			c.Edition = video.Edition
		}
	}
	c.fillSeason(asEpisode)

	for i, video := range asEpisode {
		r := row{Index: i, Path: video.Path, Include: !slices.Contains(e.Excluded, video.Path), Ask: rowAsk[video.Path]}
		if r.Ask == nil {
			r.Ask = questions{}
		}
		fillRow(&r, video)
		if c.MultiSeason && !r.Special && video.Season >= 0 {
			r.Season = strconv.Itoa(video.Season)
		}
		c.Rows = append(c.Rows, r)
	}
	// The one file the torrent imports is the movie, and of more the user
	// picks it
	if included := slices.DeleteFunc(slices.Clone(c.Rows), func(r row) bool { return !r.Include }); len(included) == 1 {
		c.Rows[included[0].Index].MovieFile = true
	}

	for _, t := range []struct {
		library []string
		near    *[]string
		tab     tab
	}{{shows, &c.NearShows, c.Show}, {movies, &c.NearMovies, c.Movie}} {
		year, _ := strconv.Atoi(t.tab.Year)
		title := t.tab.Title
		if title == "" && len(t.tab.SuggestTitle) > 0 {
			title = t.tab.SuggestTitle[0].Value
		}
		name := withYear(title, year)
		if _, found := libraryDir(name, t.library); !found {
			*t.near = nearDirs(name, t.library)
		}
	}

	// What the user has to settle in the kind picked, or the kind itself
	c.Count = len(c.KindAsk)
	switch c.Kind {
	case "movie":
		c.Count += c.Movie.Ask.count()
	case "show":
		c.Count += c.Show.Ask.count()
		for _, r := range c.Rows {
			c.Count += r.Ask.count()
		}
	}
	return c
}

func (q questions) count() int {
	n := 0
	for _, field := range q {
		n += len(field)
	}
	return n
}

// fillTab fills a tab's title and year with its readings', or offers the
// title as a suggestion when in doubt
func fillTab(t *tab, readings []Reading) {
	var title string
	var year int
	for _, video := range readings {
		if video.Title != "" {
			title, year = video.Title, video.Year
			break
		}
	}
	if t.Ask["title"] == nil {
		t.Title = title
	} else if title != "" {
		t.SuggestTitle = []suggestion{{"title-" + t.Kind, title, "“" + title + "”", t.Kind}}
	}
	if year != 0 {
		t.Year = strconv.Itoa(year)
	}
}

// fillSeason fills the torrent's season, which its episodes share, or
// offers one when in doubt: the season their directory names, or else the
// first, which Jellyfin also takes when a name gives none
func (c *card) fillSeason(readings []Reading) {
	var seasons []int
	for _, video := range readings {
		special := video.Season == 0 && len(video.Episodes) == 0 && video.Name != ""
		if video.Season >= 0 && !special && !slices.Contains(seasons, video.Season) {
			seasons = append(seasons, video.Season)
		}
	}
	switch {
	case c.Show.Ask["season"] != nil:
		suggested := "1"
		if len(seasons) == 1 {
			suggested = strconv.Itoa(seasons[0])
		}
		c.SuggestSeason = []suggestion{{"season", suggested, "season " + suggested, "show"}}
	case len(seasons) == 1:
		c.Season = strconv.Itoa(seasons[0])
	case len(seasons) > 1:
		c.MultiSeason = true
	}
}

// fillRow fills a video's episode, or offers it as a suggestion when in doubt
func fillRow(r *row, video Reading) {
	field := func(name string) string { return fmt.Sprintf("%s-%d", name, r.Index) }
	r.SpecialName = video.Name
	special := video.Season == 0 && len(video.Episodes) == 0 && video.Name != ""
	if r.Ask["special"] == nil {
		r.Special = special
	} else if special {
		r.Suggest = append(r.Suggest, suggestion{field("special"), "on", "as a special", "show"})
	}
	if special || len(video.Episodes) == 0 {
		return
	}
	episode := strconv.Itoa(video.Episodes[0])
	if r.Ask["episodes"] == nil {
		r.Episode = episode
	} else {
		r.Suggest = append(r.Suggest, suggestion{field("episode"), episode, "episode " + episode, "show"})
	}
}

type done struct {
	*entry
	ID     string
	When   string
	Groups []linkGroup
	// Why the importer left the torrent alone
	Reasons []string
}

// linkGroup is the links of a torrent into one show or movie
type linkGroup struct {
	// Like shows/The Office US
	Dir   string
	Links []shownLink
}

type shownLink struct {
	Link
	// Within the show's or movie's directory, and the download's file
	Place, Source pathParts
}

// pathParts splits a path into its directories, which the page cuts when the
// path is too long, and its file's name, which it keeps
type pathParts struct {
	Head, Tail string
}

func splitPath(p string) pathParts {
	dir, file := path.Split(p)
	return pathParts{dir, file}
}

func newDone(e *entry) done {
	d := done{entry: e, ID: cardID(e.Name), When: e.Done.Format("2 Jan 15:04")}
	for _, link := range e.Links {
		// shows/<show>/... or movies/<movie>/...
		parts := strings.SplitN(link.To, "/", 3)
		dir, place := link.To, ""
		if len(parts) == 3 {
			dir, place = parts[0]+"/"+parts[1], parts[2]
		}
		if len(d.Groups) == 0 || d.Groups[len(d.Groups)-1].Dir != dir {
			d.Groups = append(d.Groups, linkGroup{Dir: dir})
		}
		group := &d.Groups[len(d.Groups)-1]
		group.Links = append(group.Links, shownLink{link, splitPath(place), splitPath(link.From)})
	}
	if e.Status == "left" {
		for _, video := range e.Videos {
			for _, doubt := range video.Doubts {
				if doubt.Field == "unsupported" {
					d.Reasons = append(d.Reasons, video.Path+": "+doubt.Question)
				}
			}
		}
	}
	return d
}
