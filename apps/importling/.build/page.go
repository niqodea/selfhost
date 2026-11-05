package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	"path"
	"slices"
	"strconv"
)

// The page shows each field of a form the same way: a sure field filled in,
// and an open one empty and marked, with its questions under it and its
// values as suggestions, which fill it in at a click. It also follows these
// rules:
//   - Nothing moves when the user picks something. A file left out keeps its
//     row
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
<title>Importling</title>
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
  .count { flex: none; font-size: 0.875rem; font-weight: 600; color: var(--ask); }
  .count.ready { color: var(--ok); }

  /* An open field: the field, a mark next to its label, and the questions
     under it */
  .ask { border-color: var(--ask) !important; box-shadow: 0 0 0 1px var(--ask); background: color-mix(in srgb, var(--ask) 8%, var(--surface)) !important; }
  .mark { display: inline-grid; place-items: center; width: 1.1rem; height: 1.1rem; margin-left: 0.25rem; border-radius: 50%; background: var(--ask); color: var(--surface); font-size: 0.75rem; font-weight: 700; vertical-align: 0.1em; }
  .questions { margin: 0.25rem 0 0; padding: 0; list-style: none; color: var(--ask); font-size: 0.875rem; }
  .suggest { margin: 0.25rem 0.25rem 0 0; padding: 0.125rem 0.5rem; border: 1px dashed var(--ask); border-radius: 1rem; background: none; color: var(--text); font-size: 0.8125rem; white-space: normal; text-align: left; }
  .suggest:hover { border-style: solid; }

  .fields { display: grid; grid-template-columns: minmax(12rem, 1fr) 10rem; gap: 1rem; align-items: start; margin-bottom: 0.5rem; }
  .help { margin-bottom: 1rem; }
  .field > span { display: block; margin-bottom: 0.25rem; font-size: 0.875rem; color: var(--muted); }

  table { width: 100%; border-collapse: collapse; table-layout: fixed; margin-bottom: 0.5rem; }
  th { padding: 0 0.5rem 0.25rem; font-size: 0.875rem; font-weight: 400; color: var(--muted); text-align: left; }
  td { padding: 0.5rem; border-top: 1px solid var(--border); vertical-align: top; }
  col.include { width: 3.5rem; }
  col.numbers { width: 10rem; }
  .episode { display: flex; gap: 0.375rem; align-items: center; }
  td.include, th.include { text-align: center; }
  td.include { padding-top: 0.75rem; }
  td.file { overflow-wrap: anywhere; }
  tr:has([name$=include]:not(:checked)) td.file > .mono { color: var(--muted); text-decoration: line-through; }
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
    .fields { grid-template-columns: 1fr; }
    thead { display: none; }
    table, tbody, tr, td { display: block; }
    tr { display: grid; grid-template-columns: 2.5rem 1fr; gap: 0.25rem 0.5rem; padding: 0.5rem 0; border-top: 1px solid var(--border); }
    td { border: none; padding: 0; }
    td.include { grid-row: 1 / 3; }
    td.numbers { grid-column: 2; }
  }
</style>
<body>
<h1>Importling</h1>

<h2>Waiting for you</h2>
{{range .Waiting}}<form class="card" id="{{.ID}}" method="post" action="answer" autocomplete="off">
  <input type="hidden" name="name" value="{{.Name}}">
  <header>
    <span class="mono">{{.Name}}</span>
    <span class="count" data-count></span>
  </header>

  <div class="fields">
    <div>{{template "input" (field .Show "Show, as the names call it" "required")}}{{template "questions" .Show}}{{template "chips" .Show}}</div>
    <div>{{if .SharedSeason}}{{template "input" (field .Season "Season" "inputmode=numeric required")}}{{template "questions" .Season}}{{template "chips" .Season}}{{end}}</div>
  </div>

  <table>
    <colgroup><col class="include"><col><col class="numbers"></colgroup>
    <thead><tr><th class="include">Import</th><th>File</th><th>Episode</th></tr></thead>
    <tbody>
    {{range .Rows}}<tr data-row>
      <td class="include"><input type="checkbox" name="{{.Index}}.include" aria-label="Import this file"{{if .Include}} checked{{end}}></td>
      <td class="file">
        <span class="mono">{{.Path}}</span>
        {{with .Season}}{{template "questions" .}}{{template "chips" .}}{{end}}{{template "questions" .Episode}}{{template "chips" .Episode}}
      </td>
      <td class="numbers"><span class="episode">
        {{with .Season}}<span class="muted">S</span><input name="{{.Name}}" value="{{.Value}}" inputmode="numeric" aria-label="Season" required{{if .Open}} class="ask" data-open{{end}}>{{end}}
        <span class="muted">E</span><input name="{{.Episode.Name}}" value="{{.Episode.Value}}" inputmode="numeric" aria-label="Episode" required{{if .Episode.Open}} class="ask" data-open{{end}}>
      </span></td>
    </tr>{{end}}
    </tbody>
  </table>
  {{if .LeftOut}}<ul class="left-out">{{range .LeftOut}}<li>Left out: <span class="mono">{{.Path}}</span> · {{.Reason}}</li>{{end}}</ul>{{end}}

  <div class="actions">
    <button class="primary">Import</button>
    <button class="quiet" formaction="skip" formnovalidate>Skip</button>
  </div>
</form>{{else}}<p class="muted">Nothing to settle.</p>{{end}}

<h2>Done</h2>
{{range .Done}}<div class="card done" id="{{.ID}}">
  <header>
    <span class="mono">{{.Name}}</span>
    {{if eq .Status "imported"}}<span class="status imported">Imported {{.When}}</span>
    {{else if eq .Status "left"}}<span class="status muted">Left alone {{.When}}</span>
    {{else}}<span class="status muted">Skipped {{.When}}</span>
    <form method="post" action="reopen">
      <input type="hidden" name="name" value="{{.Name}}">
      <button class="quiet">Reopen</button>
    </form>{{end}}
  </header>
  {{if .Unsupported}}<ul class="left-out">{{range .Unsupported}}<li>{{.}}</li>{{end}}</ul>{{end}}
  {{if .Links}}<ul class="links group">{{range .Links}}<li>
    <div class="mono">{{.To}}</div>
    <div class="path mono from" title="{{.From}}">{{template "path" .Source}}</div>
  </li>{{end}}</ul>{{end}}
  {{if .LeftOut}}<details><summary>{{len .LeftOut}} left out</summary><ul class="links">{{range .LeftOut}}<li>
    <div class="path mono" title="{{.Path}}">{{template "path" (split .Path)}}</div>
    <div class="from">{{.Reason}}</div>
  </li>{{end}}</ul></details>{{end}}
</div>{{else}}<p class="muted">Nothing imported yet.</p>{{end}}

<script>
  for (const form of document.querySelectorAll("form.card")) {
    form.addEventListener("input", () => update(form))
    form.addEventListener("change", () => update(form))
    update(form)
  }

  function update(form) {
    // A file left out keeps its row, and its numbers are disabled, so the
    // form neither sends nor requires them
    for (const row of form.querySelectorAll("tr[data-row]")) {
      const included = row.querySelector("[name$=include]").checked
      for (const input of row.querySelectorAll("td.numbers input")) input.disabled = !included
    }

    // What is left to settle: each required field still empty
    const left = new Set()
    for (const input of form.elements) {
      if (input.required && !input.disabled && !input.value.trim()) left.add(input.name)
    }
    // An open field stays marked until filled in
    for (const input of form.querySelectorAll("input[data-open]")) input.classList.toggle("ask", !input.value.trim())
    const count = form.querySelector("[data-count]")
    count.textContent = left.size ? left.size + " to settle" : "Ready"
    count.classList.toggle("ready", !left.size)
  }

  // A suggestion fills in the field it names
  for (const button of document.querySelectorAll(".suggest")) {
    button.addEventListener("click", () => {
      for (const [name, value] of Object.entries(JSON.parse(button.dataset.fill))) button.form.elements[name].value = value
      button.form.dispatchEvent(new Event("change"))
    })
  }
</script>
{{define "mark"}}{{if .Open}}<i class="mark" aria-label="To settle">?</i>{{end}}{{end}}
{{define "questions"}}{{if .Open}}<ul class="questions">{{range .Questions}}<li>{{.}}</li>{{end}}</ul>{{end}}{{end}}
{{define "chips"}}{{range .Chips}}<button type="button" class="suggest" data-fill="{{.Fill}}">{{.Label}}</button>{{end}}{{end}}
{{define "input"}}<label class="field">
  <span>{{.Label}}{{template "mark" .Input}}</span>
  <input name="{{.Input.Name}}" value="{{.Input.Value}}" {{.Attributes}}{{if .Input.Open}} class="ask" data-open{{end}}>
</label>{{end}}
{{define "path"}}<span class="head">{{.Head}}</span><span class="tail">{{.Tail}}</span>{{end}}
`

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"split": splitPath,
	"field": func(input input, label, attributes string) map[string]any {
		return map[string]any{"Input": input, "Label": label, "Attributes": template.HTMLAttr(attributes)}
	},
}).Parse(pageTemplate))

// cardID names a torrent's card on the page, since a torrent's name can hold
// spaces, which an id cannot
func cardID(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return fmt.Sprintf("t%08x", h.Sum32())
}

// input is a field as the page shows it
type input struct {
	// The form's name for the field
	Name string
	// Of a sure field
	Value     string
	Open      bool
	Questions []string
	Chips     []chip
}

// chip is a suggestion, which fills in fields with importling's guess at
// the user's click
type chip struct {
	Label string
	// The form's names of the fields it fills in, and their values, as JSON
	Fill string
}

func newChip(label string, fill map[string]string) chip {
	data, _ := json.Marshal(fill)
	return chip{label, string(data)}
}

// newInput shows a field, written as text, offering each of its values when
// it is open
func newInput[T comparable](name string, f Field[T], text func(T) string, label func(T) string) input {
	in := input{Name: name, Open: !f.Sure()}
	if f.Sure() {
		in.Value = text(f.Value())
		return in
	}
	in.Questions = f.Questions()
	for _, value := range f.Values {
		if text(value) != "" {
			in.Chips = append(in.Chips, newChip(label(value), map[string]string{name: text(value)}))
		}
	}
	return in
}

type card struct {
	*entry
	ID   string
	Show input
	// The torrent's season, when its episodes share one, or else each row has
	// its own
	Season       input
	SharedSeason bool
	Rows         []row
}

// row is a video of a waiting torrent, which the form refers to by index
type row struct {
	Index   int
	Path    string
	Include bool
	Season  *input
	Episode input
}

func newCard(e *entry) card {
	f := e.Form
	c := card{entry: e, ID: cardID(e.Name), Show: newInput("show", f.Show, same, quoted)}

	// The season is the torrent's when every episode's says the same
	var seasons []Field[int]
	for _, r := range f.Rows {
		if r.Include {
			seasons = append(seasons, r.Season)
		}
	}
	shared := len(seasons) > 0 && !slices.ContainsFunc(seasons, func(s Field[int]) bool {
		return !slices.Equal(s.Values, seasons[0].Values) || !slices.Equal(s.Doubts, seasons[0].Doubts)
	})
	if shared {
		c.Season, c.SharedSeason = newInput("season", seasons[0], strconv.Itoa, useSeason), true
	}

	for i, r := range f.Rows {
		rw := row{Index: i, Path: r.Path, Include: r.Include, Episode: newInput(rowField(i, "episode"), r.Episode, strconv.Itoa, useEpisode)}
		if !shared {
			season := newInput(rowField(i, "season"), r.Season, strconv.Itoa, useSeason)
			rw.Season = &season
		}
		c.Rows = append(c.Rows, rw)
	}
	return c
}

func quoted(s string) string { return "Use “" + s + "”" }

func same(s string) string { return s }

func useSeason(n int) string { return "Use season " + strconv.Itoa(n) }

func useEpisode(n int) string { return "Use episode " + strconv.Itoa(n) }

type done struct {
	*entry
	ID    string
	When  string
	Links []shownLink
}

// shownLink is an imported episode, where importling handed it on and the
// download's file
type shownLink struct {
	Link
	Source pathParts
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
	if e.Status != "imported" {
		return d
	}
	for _, link := range e.Form.Links() {
		d.Links = append(d.Links, shownLink{link, splitPath(link.From)})
	}
	return d
}
