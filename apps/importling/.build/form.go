package main

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Form is what importling knows of a torrent, which the user settles: the
// show, and the season and episode of each video
type Form struct {
	Show Field[string]
	Rows []Row
}

// Row is a video of the torrent
type Row struct {
	Path string
	// Whether the video goes on with the torrent
	Include bool
	Season  Field[int]
	Episode Field[int]
}

// Link is a hard link from a download to where importling hands it on: the
// show as the names call it, then the season and the episode, as in
// The Office US/Season 02/03.mkv
type Link struct {
	// Relative to the downloads directory
	From string
	// Relative to the directory importling hands torrents on in
	To string
}

// NewForm is the form of what the torrent's names say, checked
func NewForm(torrent Torrent) Form {
	f := Form{Show: torrent.Show}
	for _, video := range torrent.Videos {
		f.Rows = append(f.Rows, Row{Path: video.Path, Include: true, Season: video.Season, Episode: video.Episode})
	}
	return f.check()
}

// check doubts the episodes that two included videos share
func (f Form) check() Form {
	f.Rows = slices.Clone(f.Rows)
	seen := map[[2]int]string{}
	for i, row := range f.Rows {
		if !row.Include || !row.Season.Sure() || !row.Episode.Sure() {
			continue
		}
		id := [2]int{row.Season.Value(), row.Episode.Value()}
		if other, ok := seen[id]; ok {
			f.Rows[i].Episode = row.Episode.doubt("The same episode as " + other)
		}
		seen[id] = row.Path
	}
	return f
}

// Settled reports whether the show and the numbers of every included video
// are sure, and at least one video is included
func (f Form) Settled() bool {
	included := 0
	for _, row := range f.Rows {
		if !row.Include {
			continue
		}
		if !row.Season.Sure() || !row.Episode.Sure() {
			return false
		}
		included++
	}
	return f.Show.Sure() && included > 0
}

func rowField(i int, name string) string {
	return fmt.Sprintf("%d.%s", i, name)
}

// Links are the links of the included videos, once the form is settled. A
// slash in the show's name would make a directory, so it becomes a dash
func (f Form) Links() []Link {
	show := strings.ReplaceAll(f.Show.Value(), "/", "-")
	var links []Link
	for _, row := range f.Rows {
		if row.Include {
			to := path.Join(show, fmt.Sprintf("Season %02d", row.Season.Value()), fmt.Sprintf("%02d", row.Episode.Value())+strings.ToLower(path.Ext(row.Path)))
			links = append(links, Link{row.Path, to})
		}
	}
	return links
}

// Answer sets the fields to what the page's form sent, and checks them again.
// A field the user left empty or mistyped stays open, offering what it had
func (f Form) Answer(values url.Values) Form {
	if show := strings.TrimSpace(values.Get("show")); show != "" {
		f.Show = sure(show)
	} else {
		f.Show = f.Show.reask("It is empty")
	}
	number := func(field Field[int], name string) Field[int] {
		n, err := strconv.Atoi(strings.TrimSpace(values.Get(name)))
		if err != nil || n < 0 {
			return field.reask("It is not a number")
		}
		return sure(n)
	}
	f.Rows = slices.Clone(f.Rows)
	for i := range f.Rows {
		row := &f.Rows[i]
		row.Include = values.Has(rowField(i, "include"))
		if !row.Include {
			continue
		}
		// The torrent's season, unless its files are of different seasons
		season := "season"
		if values.Has(rowField(i, "season")) {
			season = rowField(i, "season")
		}
		row.Season = number(row.Season, season)
		row.Episode = number(row.Episode, rowField(i, "episode"))
	}
	return f.check()
}
