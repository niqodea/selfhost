# ADR: Importling - Read What Is Certain, Ask About the Rest

**Decision:** Importling reads a finished torrent of a TV show: the show, as the names call it, and the season and episode of each file. It hands the torrent on only when its names allow one reading. Otherwise it keeps what it could read and asks the user, on its web page, only for the parts it could not.

## Context

Torrents name their files in many ways: scene names like `Show.S01E02.1080p.WEB-GRP`, season packs with only `03 - Title.mkv` inside, a hashed file in a directory named as the episode.
A rule that reads all of them has to guess, and a wrong guess files an episode as another, where nobody notices it until they look for it.

Importling reads only what a torrent says. Which show of the library a name means, through its aliases or a name close to it, is for the next app, the catalog manager, which also links the episodes into the library.

## Principles

- **A wrong reading costs more than a question.** A question costs the user a click. A wrong reading costs them finding it later and fixing it by hand.
- **Read names only through rules a person can check.** Each rule is one pattern, and a name that matches none of them is a question. A guessing library like guessit always returns its best guess, with no sign of how sure it is.
- **Handle only episodes of a season.** A file is read when its name or its directory gives the season and the episode. Any other, like a movie, a daily show's episode, or an episode numbered from the first, is a question the user can answer or skip.
- **Decide per torrent.** One unclear file makes the whole torrent a question, so a season is never half handed on.
- **Ask as little as possible.** The question comes filled in with what was read, and marks only the fields that are unclear.
- **Take the user's answer, never the guess.** A field in doubt starts empty and offers the guess as a suggestion, so importling always hands on something the user chose.
- **Handle only plain torrents.** Each video is one episode. A torrent with a file of several episodes is left alone, and samples and extras are never read.
- **Leave nothing out without saying so.** The files importling leaves out, and the torrents it leaves alone, are listed with the reason.
- **Never change a download.** Importling only reads the downloads and makes hard links to them.

## Model

- **A field is sure or open.** Each value, like the show's name or an episode's season, holds the values the names give and the doubts about them. It is sure with one value and no doubt, and open otherwise. The page fills in a sure field, and leaves an open one empty, with its doubts as questions and its values as suggestions.
- **The show comes from the torrent's name.** A torrent is one show, so its name, the top directory or its one file, gives the show, cut before the first episode or season it marks. A name that leaves doubt, as a pack's often does, gives way to the name of its first video that marks an episode.
- **Joining puts sources together.** Two sources that agree make a sure field, and two that disagree an open one with both values, as when a video's name and its directory give different seasons.
- **A form is the show and a row per video.** Each row says whether the video goes on with the torrent, and its season and episode. Two rows of one episode are a question.
- **An answer sets fields.** The page sends the form, which sets each field the user answered as sure, and the checks run again.
- **A settled form is handed on as the library's layout.** Once every field is sure, importling hard-links each included video as `imported/<show>/Season XX/YY.<ext>`, named by the show as the torrent calls it, so the catalog manager needs to know nothing of torrents, only which show of the library a name means. A link appears whole, so the catalog manager never sees half a file, and a folder the next app reads, like a mail spool, needs neither app running for the other to work. An episode already in `imported/` links nothing of the torrent, and becomes a question.

## Trade-offs

| Pros                                     | Cons                                                        |
|------------------------------------------|-------------------------------------------------------------|
| A torrent is read wrongly only when its names agree on the wrong reading | Names in uncommon layouts always need the user              |
| Every question says which rule it comes from | The rules grow by hand as new layouts show up               |
| Importling needs nothing of the library  | Nothing reaches the library until the catalog manager exists |
