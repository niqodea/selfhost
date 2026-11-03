package main

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// File is a file of a torrent, with its path relative to the downloads
// directory
type File struct {
	Path string
	Size int64
}

// Reading is what the names of a video and of its directories say about it
type Reading struct {
	Path string
	// "episode", "movie", or empty when the names say neither
	Kind     string
	Title    string
	Year     int
	Season   int
	Episodes []int
	// Of a special with no episode number, which goes by this name
	Name string `json:",omitempty"`
	// Of a movie, a label that tells its versions apart, like Extended
	Edition string `json:",omitempty"`
	// From a leading [Group]
	Group string `json:",omitempty"`
	// Whether the names give an episode but no season
	Lone bool `json:"-"`
	// For a name that may be a movie's or an episode's, its reading as an
	// episode, next to this one as a movie
	AsEpisode *Reading `json:",omitempty"`
	Doubts    []Doubt
}

// Doubt is a question for the user, about a reading that may be wrong
type Doubt struct {
	// The answer that settles it: kind, title, place, edition, season,
	// episodes, special, date or include, or unsupported for a video the importer
	// leaves alone whatever the answer
	Field    string
	Question string
	// The video it is about, when it is about one video of a torrent
	Path string `json:",omitempty"`
}

// LeftOut is a video that is not imported unless the user adds it
type LeftOut struct {
	Path   string
	Reason string
}

type Torrent struct {
	Videos  []Reading
	LeftOut []LeftOut
}

var videoExtensions = []string{".mkv", ".mp4", ".avi", ".m4v", ".mov", ".ts", ".webm", ".wmv", ".mpg", ".mpeg"}

// A word in a release name starts and ends at one of these characters, or at
// either end of the name
const (
	wordStart = `(?:^|[ ._\-\[(])`
	wordEnd   = `(?:$|[ ._\-\])])`
)

// The ways a name marks an episode. The groups are the season, the episode,
// and any episodes after it, which make a file of more than one episode
var markers = []*regexp.Regexp{
	// S01E02, and S01E02E03, S01E02-E03 or S01E02-03
	regexp.MustCompile(`(?i)` + wordStart + `s(\d{1,2})[ ._]?e(\d{1,3})((?:-?e\d{1,3}|-\d{1,3})*)` + wordEnd),
	// Season 1 Episode 2
	regexp.MustCompile(`(?i)` + wordStart + `season[ ._]?(\d{1,2})[ ._\-]*episode[ ._]?(\d{1,3})()` + wordEnd),
	// 1x02, and 1x02-03 or 1x02-1x03
	regexp.MustCompile(`(?i)` + wordStart + `(\d{1,2})x(\d{2,3})((?:-(?:\d{1,2}x|x)?\d{2,3})*)` + wordEnd),
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
	// A title, then an episode number with no season, as in Frieren - 05,
	// Frieren Ep 05 or Superman 2. A dash, when there is one, ends the title,
	// as in Show 2 - 05. The second group is what marks the number as an
	// episode's, if anything does
	loneDashRe = regexp.MustCompile(`(?i)^(.*?)[ ._]+(-)[ ._]+(?:e|ep|episode|#)?[ ._]?(\d{1,4})(?:v\d)?(?:$|[ ._\-\])\[(])`)
	loneRe     = regexp.MustCompile(`(?i)^(.*?)(?:^|[ ._]+)(e|ep|episode|#|\[)?[ ._]?(\d{1,4})(?:v\d)?(?:$|[ ._\-\])\[(])`)
	specialRe  = regexp.MustCompile(`(?i)` + wordStart + `(?:special|ova|oav)` + wordEnd)

	// A daily show's episode goes by its date: 2024-03-12, 12.03.2024 or
	// 12th Mar 2024
	dateRe = regexp.MustCompile(`(?i)` + wordStart + `(?:(?:19|20)\d{2}[ ._\-]\d{2}[ ._\-]\d{2}|\d{2}[ ._\-]\d{2}[ ._\-](?:19|20)\d{2}` +
		`|\d{1,2}(?:st|nd|rd|th)?[ ._\-](?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|june?|july?|aug(?:ust)?|sep(?:t|tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)[ ._\-](?:19|20)\d{2})` + wordEnd)

	// A directory that holds one season: Season 2, S02, or a release name
	// with S02 or Season 2 in it
	seasonDirRe   = regexp.MustCompile(`(?i)^(?:season|series|s)[ ._]?(\d{1,2})$`)
	seasonMarkRe  = regexp.MustCompile(`(?i)` + wordStart + `(?:s(\d{1,2})|season[ ._]?(\d{1,2}))(?:$|[ ._\-\])e])`)
	seasonRangeRe = regexp.MustCompile(`(?i)` + wordStart + `(?:s|season[ ._]?)\d{1,2}[ ._]?(?:-|to)[ ._]?(?:s|season[ ._]?)?\d{1,2}` + wordEnd)

	yearRe = regexp.MustCompile(wordStart + `((?:19|20)\d{2})` + wordEnd)
	// A number right after the year is an episode, as in Show (2009) - 04
	numberAfterYearRe = regexp.MustCompile(`^[ ._\-\]){}]*(\d{1,4})(?:v\d)?(?:$|[ ._\-\])}])`)

	sampleRe = regexp.MustCompile(`(?i)` + wordStart + `sample` + wordEnd + `|[ ._\-]trailer$`)
	// www.example.org - Name, or [www.example.org] Name
	websitePrefixRe = regexp.MustCompile(`(?i)^[\[(]?\s*w{2,3}[ .,][^\])]*?(?:[\])][ ._]*-?|[ ._]-)[ ._]*`)
	// [Group] Name
	leadingTagsRe  = regexp.MustCompile(`^(?:\[[^\]]*\][ ._\-]*)+`)
	trailingYearRe = regexp.MustCompile(` \(?((?:19|20)\d{2})\)?$`)
	resolutionRe   = regexp.MustCompile(`(?i)^\d{3,4}[pi]$`)
)

var (
	// A movie's name with one of these may be an episode's instead. In
	// English, Italian, French, German, Spanish, Turkish and Russian
	partRe = regexp.MustCompile(`(?i)` + wordStart + `(?:` +
		`(?:part|pt|ep|episode|episodio|épisode|folge|disc|disk|cd|vol|volume|chapter|book|seasons?|stagione|saison|staffel|temporada|sezon|bölüm|bolum|сезон|серия|серии|эпизод)[ ._:]*\d{1,3}` +
		`|\d{1,3}[ ._]?of[ ._]?\d{1,3}` +
		`|\d{1,3}[ ._]?(?:sez|ser|seriya|serija|serii|sezon|сезон|серия|серии)` +
		`|e\d{1,4}|s\d{1,2}|x\d{2}|mini-?series|complete` +
		`)` + wordEnd)
	// A part word that numbers a season rather than an episode
	seasonWordRe = regexp.MustCompile(`(?i)season|stagione|saison|staffel|temporada|sezon|сезон`)
	numberRe     = regexp.MustCompile(`\d+`)
)

// Words that name a version of a movie, which Jellyfin keeps in the
// movie's directory as one more file, labelled after a dash
// Ref: https://jellyfin.org/docs/general/server/media/movies/
var editions = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)` + wordStart + `directors?'?s?[ ._]cut` + wordEnd), "Directors Cut"},
	{regexp.MustCompile(`(?i)` + wordStart + `extended(?:[ ._](?:cut|edition|version))?` + wordEnd), "Extended"},
	{regexp.MustCompile(`(?i)` + wordStart + `theatrical(?:[ ._](?:cut|edition|version))?` + wordEnd), "Theatrical"},
	{regexp.MustCompile(`(?i)` + wordStart + `unrated(?:[ ._](?:cut|edition))?` + wordEnd), "Unrated"},
	{regexp.MustCompile(`(?i)` + wordStart + `uncut` + wordEnd), "Uncut"},
	{regexp.MustCompile(`(?i)` + wordStart + `imax(?:[ ._]edition)?` + wordEnd), "IMAX"},
	{regexp.MustCompile(`(?i)` + wordStart + `remastered` + wordEnd), "Remastered"},
	{regexp.MustCompile(`(?i)` + wordStart + `special[ ._]edition` + wordEnd), "Special Edition"},
}

// A title with one of these was cut from its name in the wrong place
var titleDoubts = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`[\[\]{}()<>【】「」]`), "has brackets"},
	{regexp.MustCompile(`(?:^| )- ?\d+(?: |$)`), "has a number after a dash, like an episode"},
	{regexp.MustCompile(`(?:^| )\d+-\d+(?: |$)`), "has a range of numbers"},
	{regexp.MustCompile(`(?:^| )\d+ \d+ \d+(?: |$)`), "has three numbers in a row, like a date"},
	{regexp.MustCompile(`(?i)(?:^| )(?:\d{3,4}[pi]|\d?cd\d?)(?: |$)`), "has a resolution or a disc count"},
	// A lowercase scene release starts with its group, as in
	// blow-how.to.be.single.2016
	{regexp.MustCompile(`^[^A-Z ]*-[^A-Z]*$`), "may start with a release group"},
}

// Words of a release name that are never in a title
var releaseWords = []string{
	"4k", "uhd", "hdr", "hdr10", "dv", "x264", "x265", "h264", "h265", "hevc", "avc", "xvid", "divx",
	"webrip", "webdl", "web-dl", "bluray", "blu-ray", "bdrip", "brrip", "remux", "hdtv", "pdtv",
	"dvdrip", "dvd", "dvdscr", "hdrip", "aac", "ac3", "eac3", "dts", "ddp", "dd5", "atmos", "truehd",
	"flac", "proper", "repack", "internal", "complete", "multi", "dual", "10bit", "8bit",
	"nf", "amzn", "dsnp", "hmax", "atvp", "3d",
	"extended", "theatrical", "edition", "imax", "unrated", "uncut", "remastered", "custom", "dc", "se",
	"german", "french", "fr", "truefrench", "vostfr", "vff", "italian", "ita", "spanish", "subbed", "dubbed",
	"www", "ww",
}

// Writing systems a title may use one of. A title in two, like 超能警探
// Memorist, holds a translation of itself
var scripts = [][]*unicode.RangeTable{
	{unicode.Latin},
	{unicode.Cyrillic},
	{unicode.Greek},
	{unicode.Arabic},
	{unicode.Hebrew},
	{unicode.Han, unicode.Hiragana, unicode.Katakana},
	{unicode.Hangul},
}

var extrasDirs = []string{"extras", "featurettes", "bonus", "behind the scenes", "deleted scenes", "trailers", "interviews", "shorts", "samples", "sample", "other"}

// Read reads every video of a torrent, and leaves out the ones that are
// likely not part of the show or movie
func Read(files []File) Torrent {
	var torrent Torrent
	var largest int64
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
			torrent.Videos = append(torrent.Videos, readFile(file.Path))
			largest = max(largest, file.Size)
		}
	}

	// A movie under a tenth of the largest video's size is an extra
	sizes := map[string]int64{}
	for _, file := range files {
		sizes[file.Path] = file.Size
	}
	torrent.Videos = slices.DeleteFunc(torrent.Videos, func(video Reading) bool {
		if video.Kind == "episode" || video.Kind == "dated" || sizes[video.Path]*10 >= largest {
			return false
		}
		torrent.LeftOut = append(torrent.LeftOut, LeftOut{video.Path, "much smaller than the largest video"})
		return true
	})
	return torrent
}

func (r *Reading) doubt(field, format string, args ...any) {
	r.Doubts = append(r.Doubts, Doubt{Field: field, Question: fmt.Sprintf(format, args...)})
}

func readFile(file string) Reading {
	r := Reading{Path: file}
	name := stem(file)
	if tags := leadingTagsRe.FindString(name); tags != "" {
		r.Group, _, _ = strings.Cut(strings.TrimPrefix(tags, "["), "]")
	}
	dirs := readDirs(path.Dir(file))

	start, end := readMarker(name, &r)
	switch {
	case start >= 0:
	case looseMarkerRe.MatchString(name):
		r.Kind, r.Title, r.Year = "episode", dirs.title, dirs.year
		r.doubt("episodes", "Which episode is this? Its name marks it in a form the importer does not read")
		return r
	case dirs.season >= 0 && bareEpisodeRe.MatchString(name):
		m := bareEpisodeRe.FindStringSubmatchIndex(name)
		r.Kind, r.Season, r.Episodes = "episode", dirs.season, []int{atoi(name[m[2]:m[3]])}
		start, end = 0, m[3]
	case dateRe.MatchString(name):
		r.doubt("unsupported", "Is named by a date, like a daily show's episode, which the importer leaves alone")
		return r
	default:
		if !yearRe.MatchString(name) && readLone(name, dirs, &r) {
			return r
		}
		if dirs.seasonal {
			readUnnumbered(name, dirs, &r)
			return r
		}
		readMovie(file, &r)
		// A name with neither a year nor an episode number, like Some
		// Title.mkv, is likely an episode's title, under the show's directory
		if r.Year == 0 && r.AsEpisode != nil && dirs.title != "" {
			r.AsEpisode.Title, r.AsEpisode.Year = dirs.title, dirs.year
		}
		return r
	}

	if r.Kind == "episode" {
		if looseMarkerRe.MatchString(name[:start] + " " + name[end:]) {
			r.doubt("episodes", "Which episodes does this hold? Its name has more than one episode marker")
		}
		if moreAfterMarkerRe.MatchString(name[end:]) {
			r.doubt("episodes", "Does this hold more than one episode? Its name has more numbers after the episode")
		}
		if dirs.season >= 0 && r.Season != dirs.season {
			r.doubt("season", "Which season is this? Its name says %d, its directory %d", r.Season, dirs.season)
		}
	}

	title, year := splitYear(cleanTitle(name[:start]))
	// A fansub release, which starts with [Group], may put the episode's
	// number counted from the first season before the marker
	if words := strings.Fields(title); r.Group != "" && len(words) > 1 && isNumber(words[len(words)-1]) {
		r.doubt("title", "Is “%s” the title? It ends with a number, which may be the episode counted from the first season", title)
	}
	readShowTitle(title, year, dirs, &r)
	return r
}

// readShowTitle sets the show from a file's name or from its directories,
// and doubts it when they disagree
func readShowTitle(title string, year int, dirs dirs, r *Reading) {
	switch {
	case title == "":
		title, year = dirs.title, dirs.year
	case dirs.title == "":
	case key(title) != key(dirs.title):
		r.doubt("title", "Which show is this? Its name says “%s”, its directory “%s”", title, dirs.title)
	case year == 0:
		year = dirs.year
	case dirs.year != 0 && dirs.year != year:
		r.doubt("title", "Which year is the show from? Its name says %d, its directory %d", year, dirs.year)
	}
	r.Title, r.Year = title, year
	r.Doubts = append(r.Doubts, checkTitle(title)...)
}

// readMarker fills the episode from the first marker in a name, and returns
// where the marker starts and where its numbers end, or -1 for both when the
// name has none
func readMarker(name string, r *Reading) (start, end int) {
	for _, marker := range markers {
		m := marker.FindStringSubmatchIndex(name)
		if m == nil {
			continue
		}
		r.Kind, r.Season, r.Episodes = "episode", atoi(name[m[2]:m[3]]), []int{atoi(name[m[4]:m[5]])}
		if m[6] < m[7] {
			r.doubt("unsupported", "Holds more than one episode, which the importer leaves alone")
		}
		return m[0], m[7]
	}
	return -1, -1
}

// readLone reads a title followed by a number, and reports whether the name
// is one. The number is an episode's, with no season, when something marks it
// so: a dash, E or Ep, a [Group] of fansubs, a season directory, or no title
// before it. Otherwise, as in Superman 2, it may be a sequel's
func readLone(name string, dirs dirs, r *Reading) bool {
	body := leadingTagsRe.ReplaceAllString(name, "")
	m := loneDashRe.FindStringSubmatch(body)
	if m == nil {
		m = loneRe.FindStringSubmatch(body)
	}
	if m == nil {
		return false
	}
	title, year := splitYear(cleanTitle(m[1]))
	r.Episodes, r.Season, r.Lone = []int{atoi(m[3])}, dirs.season, true
	if m[2] == "" && r.Group == "" && !dirs.seasonal && title != "" {
		episode := Reading{Path: r.Path, Kind: "episode", Episodes: r.Episodes, Season: dirs.season}
		readShowTitle(title, year, dirs, &episode)
		askSeason(&episode)
		r.Kind, r.Title, r.Season, r.Episodes, r.Lone, r.AsEpisode = "movie", cleanTitle(body), 0, nil, false, &episode
		r.doubt("kind", "A movie, “%s”, or episode %d of “%s”? Its name ends with a number, like a sequel's or an episode's", r.Title, episode.Episodes[0], episode.Title)
		return true
	}
	r.Kind = "episode"
	readShowTitle(title, year, dirs, r)
	askSeason(r)
	return true
}

// askSeason asks for the season of an episode whose name gives none. The
// question is the same for every file of a torrent, which the page asks once.
// A group that counts episodes across seasons numbers season 2's third episode
// as 31 when season 1 has 28, so it reminds the user of the episodes too
func askSeason(r *Reading) {
	if r.Season >= 0 {
		r.doubt("season", "Is this season %d? Only the directory says so. If the episodes count across seasons, fix them too", r.Season)
		return
	}
	r.doubt("season", "Which season is this? The names give only episode numbers. If they count across seasons, fix them too")
}

// readUnnumbered reads a video with no episode number in a directory of a
// season, which may be a special
func readUnnumbered(name string, dirs dirs, r *Reading) {
	r.Kind, r.Title, r.Year, r.Season = "episode", dirs.title, dirs.year, dirs.season
	if specialRe.MatchString(name) {
		r.Season, r.Name = 0, descriptiveName(name)
		r.doubt("special", "Is this a special? Its name has no episode number")
		return
	}
	r.doubt("episodes", "Which episode is this? Its name has no episode number")
}

// readMovie reads a movie from its file's name, or from its torrent's
// directory when the name has no year or is in doubt and the directory's is
// not, as in How.To.Be.Single.2016/blow-how.to.be.single.2016.mkv
func readMovie(file string, r *Reading) {
	movie := readMovieName(stem(file))
	root, _, _ := strings.Cut(file, "/")
	if root != file {
		rootMovie := readMovieName(root)
		switch {
		case rootMovie.Year != 0 && (movie.Year == 0 || len(movie.Doubts) > 0 && len(rootMovie.Doubts) == 0):
			movie = rootMovie
		case movie.Year != 0 && rootMovie.Year != 0 && (key(movie.Title) != key(rootMovie.Title) || movie.Year != rootMovie.Year):
			movie.doubt("title", "Which movie is this? Its name says “%s”, its directory “%s”", withYear(movie.Title, movie.Year), withYear(rootMovie.Title, rootMovie.Year))
		}
	}
	movie.Path, movie.Group = r.Path, r.Group
	if movie.AsEpisode != nil {
		movie.AsEpisode.Path, movie.AsEpisode.Group = r.Path, r.Group
	}
	*r = movie
	if r.Lone {
		askSeason(r)
	}
}

// readMovieName reads a movie from a name, or an episode with no season when
// a number follows the year, as in Fullmetal Alchemist (2009) 04
func readMovieName(name string) Reading {
	r := Reading{Kind: "movie"}
	var labels []string
	for _, edition := range editions {
		if m := edition.re.FindStringIndex(name); m != nil {
			labels = append(labels, edition.label)
			name = name[:m[0]] + " " + name[m[1]:]
		}
	}
	r.Edition = strings.Join(labels, " ")

	// The year is the last one with a title before it, since a title can
	// hold a year too, as in Blade Runner 2049 (2017)
	yearEnd := 0
	for _, m := range slices.Backward(findAll(yearRe, name)) {
		candidate := atoi(name[m[2]:m[3]])
		if candidate > time.Now().Year()+1 || cleanTitle(name[:m[0]]) == "" {
			continue
		}
		r.Title, r.Year, yearEnd = cleanTitle(name[:m[0]]), candidate, m[3]
		break
	}
	if r.Year == 0 {
		r.Title = cleanTitle(name)
		r.AsEpisode = &Reading{Kind: "episode", Title: r.Title, Season: -1}
		r.AsEpisode.doubt("episodes", "Which episode is this? Its name gives no episode number")
		askSeason(r.AsEpisode)
		r.doubt("kind", "A movie or an episode of a show? Its name has neither a year nor an episode number")
		return r
	}
	if m := numberAfterYearRe.FindStringSubmatch(name[yearEnd:]); m != nil {
		r.Kind, r.Edition, r.Episodes, r.Season, r.Lone = "episode", "", []int{atoi(m[1])}, -1, true
	} else if part := strings.Trim(partRe.FindString(name), " ._-[]()"); part != "" {
		r.AsEpisode = readPart(r, part)
		if number := numberRe.FindString(part); number != "" && !seasonWordRe.MatchString(part) {
			r.doubt("kind", "A movie, “%s”, or episode %s of a show? Its name has “%s”", withYear(r.Title, r.Year), number, part)
		} else {
			r.doubt("kind", "A movie, “%s”, or an episode of a show? Its name has “%s”", withYear(r.Title, r.Year), part)
		}
	}
	r.Doubts = append(r.Doubts, checkTitle(r.Title)...)
	return r
}

// readPart reads as an episode a movie whose name has a part word, as in
// Planet.Earth.II.2016.Part.3
func readPart(movie Reading, part string) *Reading {
	episode := &Reading{Kind: "episode", Title: movie.Title, Year: movie.Year, Season: -1}
	number := numberRe.FindString(part)
	switch {
	case number != "" && seasonWordRe.MatchString(part):
		episode.Season = atoi(number)
		episode.doubt("episodes", "Which episode is this? Its name gives only the season")
	case number != "":
		episode.Episodes = []int{atoi(number)}
		askSeason(episode)
	default:
		episode.doubt("episodes", "Which episode is this? Its name gives no episode number")
		askSeason(episode)
	}
	episode.Doubts = append(episode.Doubts, checkTitle(episode.Title)...)
	return episode
}

type dirs struct {
	// Of the nearest directory with a title
	title string
	year  int
	// -1 when the directory holding the file does not name exactly one
	season int
	// Whether any directory names a season
	seasonal bool
}

// readDirs reads the show and season from the directories around a file,
// nearest first
func readDirs(dir string) dirs {
	result := dirs{season: -1}
	names := strings.Split(dir, "/")
	for i, name := range slices.Backward(names) {
		if name == "." || name == "" {
			continue
		}
		nearest := i == len(names)-1
		if m := seasonDirRe.FindStringSubmatch(name); m != nil {
			result.seasonal = true
			if nearest {
				result.season = atoi(m[1])
			}
			continue
		}
		var marked Reading
		cut, _ := readMarker(name, &marked)
		switch {
		case cut >= 0:
			result.seasonal = true
			if nearest {
				result.season = marked.Season
			}
		case seasonMarkRe.MatchString(name):
			m := seasonMarkRe.FindStringSubmatchIndex(name)
			cut = m[0]
			result.seasonal = true
			if nearest && !seasonRangeRe.MatchString(name) && len(seasonMarkRe.FindAllString(name, -1)) == 1 {
				result.season = atoi(name[max(m[2], m[4]):max(m[3], m[5])])
			}
		case looseMarkerRe.MatchString(name):
			result.seasonal = true
		}
		if result.title != "" {
			continue
		}
		if cut < 0 {
			cut = len(name)
		}
		title, year := splitYear(cleanTitle(name[:cut]))
		if title != "" && len(checkTitle(title)) == 0 {
			result.title, result.year = title, year
		}
	}
	return result
}

func checkTitle(title string) []Doubt {
	if title == "" {
		return []Doubt{{Field: "title", Question: "What is the title? The names give none"}}
	}
	var reasons []string
	for _, d := range titleDoubts {
		if d.re.MatchString(title) {
			reasons = append(reasons, d.reason)
		}
	}
	if strings.ContainsFunc(title, func(r rune) bool { return !unicode.IsPrint(r) }) {
		reasons = append(reasons, "has a character that does not print")
	}
	used := 0
	for _, script := range scripts {
		if strings.ContainsFunc(title, func(r rune) bool { return unicode.In(r, script...) }) {
			used++
		}
	}
	if used > 1 {
		reasons = append(reasons, "mixes writing systems, so it may hold a translation")
	}
	for _, word := range strings.Fields(strings.ToLower(title)) {
		if slices.Contains(releaseWords, word) {
			reasons = append(reasons, fmt.Sprintf("has “%s”, a word of release names", word))
		}
	}
	var doubts []Doubt
	for _, reason := range reasons {
		doubts = append(doubts, Doubt{Field: "title", Question: fmt.Sprintf("Is “%s” the title? It %s", title, reason)})
	}
	return doubts
}

// descriptiveName is a special's name, cut before the first word of release
// names, as Show Name Special from Show.Name.Special.720p.WEB
func descriptiveName(name string) string {
	words := strings.Fields(cleanTitle(name))
	for i, word := range words {
		lower := strings.ToLower(word)
		if slices.Contains(releaseWords, lower) || resolutionRe.MatchString(lower) {
			words = words[:i]
			break
		}
	}
	return strings.Join(words, " ")
}

func cleanTitle(s string) string {
	s = websitePrefixRe.ReplaceAllString(s, "")
	s = leadingTagsRe.ReplaceAllString(s, "")
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " -([")
}

func splitYear(title string) (string, int) {
	m := trailingYearRe.FindStringSubmatchIndex(title)
	if m == nil {
		return title, 0
	}
	return strings.TrimSpace(title[:m[0]]), atoi(title[m[2]:m[3]])
}

// findAll is regexp's FindAllStringSubmatchIndex for an expression whose
// first group ends before its last character, so that the character after
// one match can start the next
func findAll(re *regexp.Regexp, s string) [][]int {
	var matches [][]int
	for offset := 0; offset < len(s); {
		m := re.FindStringSubmatchIndex(s[offset:])
		if m == nil {
			break
		}
		for i := range m {
			m[i] += offset
		}
		matches = append(matches, m)
		offset = m[3]
	}
	return matches
}

func stem(file string) string {
	name := path.Base(file)
	return strings.TrimSuffix(name, path.Ext(name))
}

func isNumber(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
