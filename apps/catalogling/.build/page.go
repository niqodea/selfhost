package main

import "html/template"

const pageTemplate = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Catalogling</title>
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
    --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font: 16px/1.5 system-ui, sans-serif;
  }
  * { box-sizing: border-box; }
  body { margin: 0 auto; max-width: 60rem; padding: 1.5rem 1rem 4rem; background: var(--background); color: var(--text); }
  h1 { margin: 0 0 1rem; font-size: 1.25rem; font-weight: 600; }
  h2 { margin: 2.5rem 0 0.75rem; font-size: 1rem; font-weight: 600; color: var(--muted); }
  .muted { color: var(--muted); }
  .mono { font-family: var(--mono); font-size: 0.875rem; overflow-wrap: anywhere; }
  input, button { font: inherit; color: var(--text); }
  input { width: 100%; padding: 0.375rem 0.5rem; border: 1px solid var(--border); border-radius: 0.375rem; background: var(--surface); }
  input:focus-visible { outline: 2px solid var(--accent); outline-offset: -1px; }
  button { padding: 0.5rem 1rem; border: 1px solid var(--border); border-radius: 0.5rem; background: var(--surface); cursor: pointer; white-space: nowrap; }
  button:hover { border-color: var(--muted); }
  .primary { background: var(--accent); border-color: var(--accent); color: #ffffff; font-weight: 600; }
  .primary:hover { border-color: var(--accent); filter: brightness(1.1); }
  .quiet { border-color: transparent; background: none; color: var(--muted); }
  .quiet:hover { border-color: transparent; color: var(--text); text-decoration: underline; }
  .card { margin-bottom: 1rem; padding: 1rem 1.25rem; border: 1px solid var(--border); border-radius: 0.75rem; background: var(--surface); }
  .card header { display: flex; gap: 1rem; align-items: baseline; margin-bottom: 0.75rem; }
  .card header .mono { flex: 1; min-width: 0; font-weight: 600; }
  .status { flex: none; font-size: 0.875rem; }
  .filed { color: var(--ok); }
  .field > span { display: block; margin-bottom: 0.25rem; font-size: 0.875rem; color: var(--muted); }
  .ask { border-color: var(--ask); box-shadow: 0 0 0 1px var(--ask); background: color-mix(in srgb, var(--ask) 8%, var(--surface)); }
  .question { margin: 0.25rem 0 0; color: var(--ask); font-size: 0.875rem; }
  .error { margin: 0 0 0.75rem; color: light-dark(#b91c1c, #f87171); font-size: 0.875rem; }
  .suggest { margin: 0.25rem 0.25rem 0 0; padding: 0.125rem 0.5rem; border: 1px dashed var(--ask); border-radius: 1rem; background: none; font-size: 0.8125rem; white-space: normal; text-align: left; }
  .suggest:hover { border-style: solid; }
  .episodes { margin: 0.75rem 0; padding: 0; list-style: none; }
  .episodes li { display: flex; gap: 0.75rem; padding: 0.125rem 0; }
  .episodes li > :first-child { flex: none; }
  .actions { display: flex; gap: 0.5rem; margin-top: 1rem; }
</style>
<body>
<h1>Catalogling</h1>
<datalist id="shows">{{range .Shows}}<option value="{{.}}">{{end}}</datalist>

<h2>Waiting for you</h2>
{{range .Waiting}}<form class="card" method="post" action="file" autocomplete="off">
  <input type="hidden" name="name" value="{{.Name}}">
  <header><span class="mono">{{.Name}}</span></header>
  {{if .InLibrary}}<p class="error">The library already has these episodes of {{.InLibrary}}, so they stay here</p>
  {{else}}<label class="field">
    <span>Show, as the library names it</span>
    <input name="show" list="shows" required class="ask">
  </label>
  <p class="question">{{.Question}}</p>
  {{range .Offers}}<button type="button" class="suggest" data-show="{{.Show}}">Use “{{.Show}}”, {{if .New}}new{{else}}in the library{{end}}</button>{{end}}{{end}}
  <ul class="episodes">{{range .Files}}<li><span class="mono">{{.}}</span></li>{{end}}</ul>
  <div class="actions">
    {{if not .InLibrary}}<button class="primary">File</button>{{end}}
    <button class="quiet" formaction="discard" formnovalidate>Discard</button>
  </div>
</form>{{else}}<p class="muted">Nothing to settle.</p>{{end}}

<h2>Filed</h2>
{{range .Filings}}<div class="card">
  <header>
    <span class="mono">{{.Name}}</span>
    <span class="status filed">Filed under {{.Show}} · {{.Time.Format "2 Jan 15:04"}}</span>
  </header>
  <ul class="episodes">{{range .Files}}<li><span class="mono">{{.}}</span></li>{{end}}</ul>
</div>{{else}}<p class="muted">Nothing filed yet.</p>{{end}}

<script>
  // A suggestion fills in the show
  for (const button of document.querySelectorAll(".suggest")) {
    button.addEventListener("click", () => { button.form.elements.show.value = button.dataset.show })
  }
</script>
`

var page = template.Must(template.New("page").Parse(pageTemplate))

// card is a waiting show as the page shows it
type card struct {
	Name  string
	Files []string
	// The show the name means, when it means one, whose files the library
	// already has, since catalogling moved the others
	InLibrary string
	// Why the show waits, and the shows offered for it
	Question string
	Offers   []Offer
}

// Offer is a show a waiting show may be filed under: one of the library, or
// the waiting name, as a new show
type Offer struct {
	Show string
	New  bool
}

// newCard asks which show a waiting show is: which one when its name means
// more than one, and whether it is new when it means none, offering the
// shows of a similar name
func (c *catalogling) newCard(name string, catalog Catalog) card {
	k := card{Name: name, Files: c.files(name)}
	found := catalog.Find(name)
	switch {
	case len(found) == 1:
		k.InLibrary = found[0]
		return k
	case len(found) > 1:
		k.Question = "More than one show of the library goes by this name"
		for _, show := range found {
			k.Offers = append(k.Offers, Offer{Show: show})
		}
		return k
	}
	k.Question = "No show of the library goes by this name, so it may be new"
	for _, show := range catalog.Similar(name) {
		k.Question = "No show of the library goes by this name, but some have a similar one"
		k.Offers = append(k.Offers, Offer{Show: show})
	}
	k.Offers = append(k.Offers, Offer{Show: name, New: true})
	return k
}
