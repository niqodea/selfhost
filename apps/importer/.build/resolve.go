package main

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

// Link is a hard link from a download into the library
type Link struct {
	From string
	// Relative to the library, under shows/ or movies/
	To string
}

// Plan is what to do with a torrent: links when nothing is in doubt, or
// otherwise the doubts about the torrent as a whole, next to the doubts of
// each video, for the user to resolve
type Plan struct {
	Doubts []Doubt
	Links  []Link
}

// Resolve plans the links of a torrent's videos into the library, whose
// show and movie directories are shows and movies. The user's answers to a
// plan's doubts come back here as videos with no doubts and answered set,
// since the user has then seen the library's titles close to theirs and
// chosen one
func Resolve(videos []Reading, shows, movies []string, answered bool) Plan {
	var plan Plan
	doubt := func(field, path, format string, args ...any) {
		plan.Doubts = append(plan.Doubts, Doubt{Field: field, Path: path, Question: fmt.Sprintf(format, args...)})
	}
	if len(videos) == 0 {
		doubt("include", "", "Which file is the video? None looks like one")
		return plan
	}
	first := videos[0]
	for _, video := range videos[1:] {
		switch {
		case video.Kind != first.Kind:
			doubt("kind", "", "Is this a show or a movie? The importer read both")
		// A video of no kind has its doubts already, and its title may be an
		// episode's
		case first.Kind != "" && (key(video.Title) != key(first.Title) || video.Year != first.Year):
			doubt("title", "", "Which show or movie is this? The files say “%s” and “%s”", withYear(first.Title, first.Year), withYear(video.Title, video.Year))
		}
	}
	if first.Kind == "movie" && len(videos) > 1 {
		doubt("include", "", "Which file is the movie? There are %d", len(videos))
	}
	seen := map[string]string{}
	for _, video := range videos {
		for _, id := range episodeIDs(video) {
			if other, ok := seen[id]; ok {
				doubt("episodes", video.Path, "Which episode is this? It is the same as %s", other)
			}
			seen[id] = video.Path
		}
	}

	name := withYear(first.Title, first.Year)
	library := shows
	if first.Kind == "movie" {
		library = movies
	}
	dir, found := libraryDir(name, library)
	if near := nearDirs(name, library); !found && len(near) > 0 && !answered {
		doubt("place", "", "Which one is this? The library has “%s”", strings.Join(near, "” and “"))
	}
	if first.Kind == "movie" && found && first.Edition == "" {
		doubt("edition", "", "Is this another version of “%s”, which the library has? Give it an edition, like Extended", dir)
	}

	if len(plan.Doubts) > 0 {
		return plan
	}
	for _, video := range videos {
		if len(video.Doubts) > 0 || video.Kind == "" {
			return plan
		}
	}
	for _, video := range videos {
		plan.Links = append(plan.Links, Link{video.Path, linkPath(video, dir)})
	}
	return plan
}

// episodeIDs names the episodes a video holds, to find two videos of the same
// episode
func episodeIDs(video Reading) []string {
	if video.Kind == "episode" && len(video.Episodes) == 0 {
		return []string{"special " + video.Name}
	}
	var ids []string
	for _, episode := range video.Episodes {
		ids = append(ids, fmt.Sprintf("S%02dE%02d", video.Season, episode))
	}
	return ids
}

// linkPath names a video in the library as Jellyfin reads it
// Ref: https://jellyfin.org/docs/general/server/media/shows/
// Ref: https://jellyfin.org/docs/general/server/media/movies/
func linkPath(video Reading, dir string) string {
	extension := strings.ToLower(path.Ext(video.Path))
	if video.Kind == "episode" {
		season := fmt.Sprintf("Season %02d", video.Season)
		// A special the metadata may not know goes by its name, since its
		// number differs between metadata providers
		if len(video.Episodes) == 0 {
			return path.Join("shows", dir, season, strings.ReplaceAll(video.Name, "/", "-")+extension)
		}
		// The season's directory gives the season, so the name gives only the
		// episode
		// Ref: https://github.com/jellyfin/jellyfin/blob/5b385862fb97201d954a6ac7c9c745d791a738fa/tests/Jellyfin.Naming.Tests/TV/EpisodeNumberTests.cs#L66
		return path.Join("shows", dir, season, fmt.Sprintf("%02d", video.Episodes[0])+extension)
	}
	// Each version of a movie starts with its directory's name exactly
	name := dir
	if video.Edition != "" {
		name += " - " + video.Edition
	}
	return path.Join("movies", dir, name+extension)
}

// libraryDir returns the library's directory of the same title, or the
// title itself when there is none, and whether there is one
func libraryDir(name string, existing []string) (string, bool) {
	for _, dir := range existing {
		if key(dir) != "" && key(dir) == key(name) {
			return dir, true
		}
	}
	return name, false
}

// nearDirs returns the directories whose names are a title's with more or
// fewer words at the end, like Doctor Who (2005) for Doctor Who, which may be
// another show or movie of the same name
func nearDirs(name string, existing []string) []string {
	if key(name) == "" {
		return nil
	}
	var near []string
	for _, dir := range existing {
		if key(dir) != "" && key(dir) != key(name) && (strings.HasPrefix(key(dir), key(name)) || strings.HasPrefix(key(name), key(dir))) {
			near = append(near, dir)
		}
	}
	return near
}

func withYear(title string, year int) string {
	if year == 0 {
		return title
	}
	return fmt.Sprintf("%s (%d)", title, year)
}

// key compares titles by their letters and digits only
func key(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, title)
}
