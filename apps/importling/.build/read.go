package main

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// File is a file of a torrent, with its path relative to the downloads
// directory
type File struct {
	Path string
	Size int64
}

// Video is what the names of a video and of its directories say about it:
// the season and episode it is
type Video struct {
	Path    string
	Season  Field[int]
	Episode Field[int]
	// Why importling leaves the torrent alone whatever the answers, if it
	// does
	Unsupported string `json:",omitempty"`
}

// LeftOut is a video that is not imported, with the reason
type LeftOut struct {
	Path   string
	Reason string
}

// Torrent is what a torrent's names say: the show, from the torrent's own
// name, since a torrent is one show, and each video
type Torrent struct {
	Show    Field[string]
	Videos  []Video
	LeftOut []LeftOut
}

var videoExtensions = []string{".mkv", ".mp4", ".avi", ".m4v", ".mov", ".ts", ".webm", ".wmv", ".mpg", ".mpeg"}

// A word in a release name starts and ends at one of these characters, or at
// either end of the name
const (
	wordStart = `(?:^|[ ._\-\[(])`
	wordEnd   = `(?:$|[ ._\-\])])`
	// A marker can also end where a bracket opens, as in S01E10[1080p]
	markerEnd = `(?:$|[ ._\-\])\[(])`
)

// The ways a name marks an episode. The groups are the season, the episode,
// and any episodes after it, which make a file of more than one episode
var markers = []*regexp.Regexp{
	// S01E02, S001E02, S01-E02, S01xE02, S01E1056 or S01E02v2, and S01E02E03,
	// S01E02-E03 or S01E02-03
	regexp.MustCompile(`(?i)` + wordStart + `s0?(\d{1,2})(?:[ ._\-]|x)?e(\d{1,4})((?:-?e\d{1,4}|-\d{1,4})*)(?:v\d)?` + markerEnd),
	// Season 1 Episode 2
	regexp.MustCompile(`(?i)` + wordStart + `season[ ._]?(\d{1,2})[ ._\-]*episode[ ._]?(\d{1,3})()` + wordEnd),
	// 1x02 or 1x02v2, and 1x02-03 or 1x02-1x03
	regexp.MustCompile(`(?i)` + wordStart + `(\d{1,2})x(\d{2,3})((?:-(?:\d{1,2}x|x)?\d{2,3})*)(?:v\d)?` + markerEnd),
}

var (
	// Looser forms of the markers, like S2016E14, S01E22c or 2x9, which mark
	// an episode the markers do not read
	looseMarkerRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:s\d{1,4}[ ._]?[ed]\d|\d{1,2}x\d{1,3})`)
	// More episodes after a marker, as in S01E01 E02 or Episode 5-6
	moreAfterMarkerRe = regexp.MustCompile(`(?i)^[ ._\-,&+]*(?:e|x|ep)?\d{1,3}` + wordEnd)
	// A file in a season directory named only by its episode: 03, E03,
	// Ep 03, Episode 3, each maybe followed by the episode title
	bareEpisodeRe = regexp.MustCompile(`(?i)^(?:e|ep|episode)?[ ._]?(\d{1,3})` + wordEnd)

	// A directory that holds one season: Season 2, S02, or a release name
	// with S02 or Season 2 in it
	seasonDirRe   = regexp.MustCompile(`(?i)^(?:season|series|s)[ ._]?(\d{1,2})$`)
	seasonMarkRe  = regexp.MustCompile(`(?i)` + wordStart + `(?:s(\d{1,2})|season[ ._]?(\d{1,2}))(?:$|[ ._\-\])e])`)
	seasonRangeRe = regexp.MustCompile(`(?i)` + wordStart + `(?:s|season[ ._]?)\d{1,2}[ ._]?(?:-|to)[ ._]?(?:s|season[ ._]?)?\d{1,2}` + wordEnd)

	// A trailing [HDTV 720p] or [1080p]
	trailingTagRe = regexp.MustCompile(`\[([^\]]*)\][ ._\-]*$`)

	sampleRe = regexp.MustCompile(`(?i)` + wordStart + `sample` + wordEnd + `|[ ._\-]trailer$`)
	// www.example.org - Name, or [www.example.org] Name
	websitePrefixRe = regexp.MustCompile(`(?i)^[\[(]?\s*w{2,3}[ .,][^\])]*?(?:[\])][ ._]*-?|[ ._]-)[ ._]*`)
	// [Group] Name
	leadingTagsRe = regexp.MustCompile(`^(?:\[[^\]]*\][ ._\-]*)+`)
	resolutionRe  = regexp.MustCompile(`(?i)^\d{3,4}[pi]$`)
)

// A name with one of these was cut from its name in the wrong place
var nameDoubts = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`[\[\]{}()<>【】「」]`), "has brackets"},
	{regexp.MustCompile(`(?:^| )- ?\d+(?: |$)`), "has a number after a dash, like an episode"},
	{regexp.MustCompile(`(?:^| )\d+-\d+(?: |$)`), "has a range of numbers"},
	{regexp.MustCompile(`(?:^| )\d+ \d+ \d+(?: |$)`), "has three numbers in a row, like a date"},
	{regexp.MustCompile(`(?i)(?:^| )(?:\d{3,4}[pi]|\d?cd\d?)(?: |$)`), "has a resolution or a disc count"},
	// A lowercase scene release starts with its group, as in
	// grp-show.name.s01e01
	{regexp.MustCompile(`^[^A-Z ]*-[^A-Z]*$`), "may start with a release group"},
}

var trailingYearRe = regexp.MustCompile(` \(?(?:19|20)\d{2}\)?$`)

// Words of a release name that are never in a show's name
var releaseWords = []string{
	"4k", "uhd", "hdr", "hdr10", "dv", "x264", "x265", "h264", "h265", "hevc", "avc", "xvid", "divx",
	"webrip", "webdl", "web-dl", "bluray", "blu-ray", "bdrip", "brrip", "remux", "hdtv", "pdtv",
	"dvdrip", "dvd", "dvdscr", "hdrip", "aac", "ac3", "eac3", "dts", "ddp", "dd5", "atmos", "truehd",
	"flac", "proper", "repack", "internal", "complete", "multi", "dual", "10bit", "8bit",
	"nf", "amzn", "dsnp", "hmax", "atvp", "3d",
	"german", "french", "fr", "truefrench", "vostfr", "vff", "italian", "ita", "spanish", "subbed", "dubbed",
	"www", "ww",
}

var extrasDirs = []string{"extras", "featurettes", "bonus", "behind the scenes", "deleted scenes", "trailers", "interviews", "shorts", "samples", "sample", "other"}

// Read reads the show from the torrent's name, which is its top directory or
// its one file, and reads every video, leaving out samples and extras. A pack
// may have a name that leaves doubt, like Arcane 2021 Seasons 1 and 2
// Complete, and files named plainly, like Arcane.S01E01, so a name that
// leaves doubt gives way to the name of the first video that marks its
// episode
func Read(files []File) Torrent {
	var torrent Torrent
	if len(files) > 0 {
		root, _, _ := strings.Cut(files[0].Path, "/")
		torrent.Show = readShow(root)
	}
	var kept []string
	for _, file := range files {
		if !slices.Contains(videoExtensions, strings.ToLower(path.Ext(file.Path))) {
			continue
		}
		dirs := strings.Split(path.Dir(file.Path), "/")
		switch {
		case slices.ContainsFunc(dirs, func(dir string) bool { return slices.Contains(extrasDirs, strings.ToLower(dir)) }):
			torrent.LeftOut = append(torrent.LeftOut, LeftOut{file.Path, "in a directory of extras"})
		case sampleRe.MatchString(stem(file.Path)):
			torrent.LeftOut = append(torrent.LeftOut, LeftOut{file.Path, "named as a sample or trailer"})
		default:
			kept = append(kept, file.Path)
		}
	}
	perDir := map[string]int{}
	for _, file := range kept {
		perDir[path.Dir(file)]++
	}
	for _, file := range kept {
		torrent.Videos = append(torrent.Videos, readVideo(file, perDir[path.Dir(file)] == 1))
	}
	for _, file := range kept {
		if start, _, _, _, _ := readMarker(stem(file)); torrent.Show.Sure() || start < 0 {
			continue
		}
		if show := readShow(path.Base(file)); show.Sure() {
			torrent.Show = show
		}
		break
	}
	return torrent
}

// readVideo reads a video, and from its directory's name when it is the only
// video there
func readVideo(file string, onlyInDir bool) Video {
	v := Video{Path: file}
	name := stem(file)
	dirSeason := readDirSeason(path.Dir(file))

	start, end, season, episode, more := readMarker(name)
	// A file with no marker, often named by a hash, alone in a directory
	// named as one episode goes by the directory's name
	if dir := path.Base(path.Dir(file)); onlyInDir && start < 0 && !looseMarkerRe.MatchString(name) && dir != "." {
		if dirStart, _, _, _, dirMore := readMarker(dir); dirStart >= 0 && !dirMore {
			name = dir
			start, end, season, episode, more = readMarker(name)
		}
	}
	switch {
	case start >= 0 && more:
		v.Unsupported = "Holds more than one episode, which importling leaves alone"
		return v
	case start >= 0:
	case dirSeason >= 0 && bareEpisodeRe.MatchString(name):
		m := bareEpisodeRe.FindStringSubmatchIndex(name)
		season, episode = dirSeason, atoi(name[m[2]:m[3]])
		start, end = 0, m[3]
	default:
		v.Season = ask[int]("Its name gives no season")
		if dirSeason >= 0 {
			v.Season = sure(dirSeason)
		}
		v.Episode = ask[int]("Its name gives no episode, so it may not be a show's")
		if looseMarkerRe.MatchString(name) {
			v.Episode = ask[int]("Its name marks the episode in a form importling does not read")
		}
		return v
	}

	v.Season = sure(season)
	v.Episode = sure(episode)
	if looseMarkerRe.MatchString(name[:start] + " " + name[end:]) {
		v.Episode = v.Episode.doubt("Its name has more than one episode marker, so it may hold more than one")
	}
	if moreAfterMarkerRe.MatchString(name[end:]) {
		v.Episode = v.Episode.doubt("Its name has more numbers after the episode, so it may hold more than one")
	}
	if dirSeason >= 0 && season != dirSeason {
		v.Season = join(v.Season, sure(dirSeason)).doubt("Its name and its directory give different seasons")
	}
	return v
}

// readShow reads the show from the torrent's name: what comes before the
// first episode or season it marks, as in Breaking Bad Season 2 Complete or
// The.Office.US.S02E03.1080p.mkv. The show is in doubt when it may be cut
// wrong
func readShow(torrent string) Field[string] {
	name := torrent
	if slices.Contains(videoExtensions, strings.ToLower(path.Ext(name))) {
		name = stem(name)
	}
	if start, _, _, _, _ := readMarker(name); start >= 0 {
		name = name[:start]
	} else if m := seasonMarkRe.FindStringIndex(name); m != nil {
		name = name[:m[0]]
	}
	title := cleanTitle(trimReleaseTags(name))
	if title == "" {
		return ask[string]("The torrent's name gives no show")
	}
	return sure(title).doubt(checkName(title)...)
}

// readMarker reads the first marker in a name, and returns where it starts
// and where its numbers end, or -1 for both when the name has none, and
// whether it marks more than one episode
func readMarker(name string) (start, end, season, episode int, more bool) {
	for _, marker := range markers {
		m := marker.FindStringSubmatchIndex(name)
		if m == nil {
			continue
		}
		return m[0], m[7], atoi(name[m[2]:m[3]]), atoi(name[m[4]:m[5]]), m[6] < m[7]
	}
	return -1, -1, 0, 0, false
}

// readDirSeason reads the season the directory holding a file names, or -1
// when it names none, or more than one
func readDirSeason(dir string) int {
	name := path.Base(dir)
	if m := seasonDirRe.FindStringSubmatch(name); m != nil {
		return atoi(m[1])
	}
	if start, _, season, _, _ := readMarker(name); start >= 0 {
		return season
	}
	if m := seasonMarkRe.FindStringSubmatchIndex(name); m != nil && !seasonRangeRe.MatchString(name) && len(seasonMarkRe.FindAllString(name, -1)) == 1 {
		return atoi(name[max(m[2], m[4]):max(m[3], m[5])])
	}
	return -1
}

// checkName says why a show's name may be cut from its name in the wrong
// place. A year at the end, like (2005), is part of a name
func checkName(title string) []string {
	var reasons []string
	for _, d := range nameDoubts {
		if d.re.MatchString(trailingYearRe.ReplaceAllString(title, "")) {
			reasons = append(reasons, d.reason)
		}
	}
	if strings.ContainsFunc(title, func(r rune) bool { return !unicode.IsPrint(r) }) {
		reasons = append(reasons, "has a character that does not print")
	}
	for _, word := range strings.Fields(strings.ToLower(title)) {
		if slices.Contains(releaseWords, word) {
			reasons = append(reasons, fmt.Sprintf("has “%s”, a word of release names", word))
		}
	}
	for i, reason := range reasons {
		reasons[i] = fmt.Sprintf("“%s” %s, so it may not be the show's name", title, reason)
	}
	return reasons
}

// trimReleaseTags cuts the bracketed tags at the end of a name that hold
// only words of release names, like [HDTV 720p]
func trimReleaseTags(name string) string {
	for {
		m := trailingTagRe.FindStringSubmatchIndex(name)
		if m == nil {
			return name
		}
		words := strings.FieldsFunc(strings.ToLower(name[m[2]:m[3]]), func(r rune) bool { return strings.ContainsRune(" ._", r) })
		if len(words) == 0 || slices.ContainsFunc(words, func(word string) bool { return !slices.Contains(releaseWords, word) && !resolutionRe.MatchString(word) }) {
			return name
		}
		name = name[:m[0]]
	}
}

func cleanTitle(s string) string {
	s = websitePrefixRe.ReplaceAllString(s, "")
	s = leadingTagsRe.ReplaceAllString(s, "")
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " -([")
}

func stem(file string) string {
	name := path.Base(file)
	return strings.TrimSuffix(name, path.Ext(name))
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
