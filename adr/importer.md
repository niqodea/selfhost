# ADR: Importer - Import What Is Certain, Ask About the Rest

**Decision:** The importer links a finished torrent into the library only when its names allow one reading. Otherwise it keeps what it could read and asks the user, on its web page, only for the parts it could not.

## Context

Torrents name their files in many ways: scene names like `Show.S01E02.1080p.WEB-GRP`, season packs with only `03 - Title.mkv` inside, anime numbered from the first episode, daily shows named by date.
A rule that reads all of them has to guess, and a wrong guess puts a file under the wrong show or movie, where nobody notices it until they look for it in Jellyfin.

## Principles

- **A wrong import costs more than a question.** A question costs the user a click. A wrong import costs them finding it later and fixing it by hand.
- **Read names only through rules a person can check.** Each rule is one pattern, and a name that matches none of them is a question. A guessing library like guessit always returns its best guess, with no sign of how sure it is.
- **Decide per torrent.** One unclear file makes the whole torrent a question, so a season is never half imported.
- **Ask as little as possible.** The question comes filled in with what was read, and marks only the fields that are unclear.
- **Take the user's answer, never the guess.** A field in doubt starts empty and offers the guess as a suggestion, so importing always takes something the user chose.
- **Handle only plain torrents.** Each video is one episode or one movie. A torrent with a file of several episodes is left alone, and extras like samples and featurettes are never imported.
- **Leave nothing out without saying so.** The files the importer leaves out, and the torrents it leaves alone, are listed with the reason.
- **Never change a download.** The importer only makes hard links and never overwrites a file, so deleting a link undoes an import.
- **Work offline.** The importer looks nothing up, since Jellyfin fetches metadata from the folder names.

## Trade-offs

| Pros                                     | Cons                                                        |
|------------------------------------------|-------------------------------------------------------------|
| A torrent is filed wrongly only when its names agree on the wrong reading | Names in uncommon layouts always need the user              |
| Every question says which rule it comes from | The rules grow by hand as new layouts show up               |
| Undoing an import touches no download    | Downloads and the library must be on one filesystem         |
