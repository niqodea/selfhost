package main

import (
	"slices"
	"testing"
)

// Most names below come from the tests of other release name parsers
// Ref: https://github.com/guessit-io/guessit/blob/3c73072b8971d412935c6f749be63c564dd41385/guessit/test/episodes.yml
// Ref: https://github.com/guessit-io/guessit/blob/3c73072b8971d412935c6f749be63c564dd41385/guessit/test/movies.yml
// Ref: https://github.com/guessit-io/guessit/tree/3c73072b8971d412935c6f749be63c564dd41385/guessit/test/cross_parser
// Ref: https://github.com/Sonarr/Sonarr/tree/9a41dfc129d5bbf8f3950f871623b46eea4f914e/src/NzbDrone.Core.Test/ParserTests
// Ref: https://github.com/Radarr/Radarr/blob/35538cb5ed0e26f6b14342bfee404803fb87177a/src/NzbDrone.Core.Test/ParserTests/ParserFixture.cs

func plan(paths []string, shows, movies []string) (Torrent, Plan) {
	var files []File
	for _, file := range paths {
		files = append(files, File{Path: file})
	}
	torrent := Read(files)
	return torrent, Resolve(torrent.Videos, shows, movies, false)
}

func doubts(torrent Torrent, plan Plan) []Doubt {
	all := plan.Doubts
	for _, video := range torrent.Videos {
		all = append(all, video.Doubts...)
	}
	return all
}

func TestLinks(t *testing.T) {
	tests := []struct {
		files  []string
		shows  []string
		movies []string
		want   []string
	}{
		{files: []string{"The.Office.US.S02E03.1080p.WEB-DL.x264-GRP.mkv"}, want: []string{"shows/The Office US/Season 02/03.mkv"}},
		{files: []string{"Doctor.Who.2005.S01E01.Rose.mkv"}, want: []string{"shows/Doctor Who (2005)/Season 01/01.mkv"}},
		{files: []string{"24.S03E04.720p.mkv"}, want: []string{"shows/24/Season 03/04.mkv"}},
		{files: []string{"Show Name - 1x02 - Title.mkv"}, want: []string{"shows/Show Name/Season 01/02.mkv"}},
		{
			files: []string{
				"Breaking Bad Season 2 Complete 720p/03 - Bit by a Dead Bee.mkv",
				"Breaking Bad Season 2 Complete 720p/04 - Down.mkv",
			},
			want: []string{"shows/Breaking Bad/Season 02/03.mkv", "shows/Breaking Bad/Season 02/04.mkv"},
		},
		{
			files: []string{"Show.Name.S01.1080p/S01E01.mkv", "Show.Name.S01.1080p/Sample/s01e01-sample.mkv", "Show.Name.S01.1080p/info.nfo"},
			want:  []string{"shows/Show Name/Season 01/01.mkv"},
		},
		{files: []string{"Doctor.Who.S01E02.mkv"}, shows: []string{"doctor who"}, want: []string{"shows/doctor who/Season 01/02.mkv"}},
		{files: []string{"1917.2019.1080p.BluRay.x264/1917.2019.1080p.BluRay.x264.mkv"}, want: []string{"movies/1917 (2019)/1917 (2019).mkv"}},
		{files: []string{"Blade.Runner.2049.2017.1080p.mkv"}, want: []string{"movies/Blade Runner 2049 (2017)/Blade Runner 2049 (2017).mkv"}},
		{files: []string{"Wonder.Woman.1984.2020.1080p.mkv"}, want: []string{"movies/Wonder Woman 1984 (2020)/Wonder Woman 1984 (2020).mkv"}},
		{files: []string{"Fargo.1996.1080p.mkv"}, want: []string{"movies/Fargo (1996)/Fargo (1996).mkv"}},
		{files: []string{"www.Torrenting.org - Movie.2008.720p.X264-DIMENSION.mkv"}, want: []string{"movies/Movie (2008)/Movie (2008).mkv"}},
		{files: []string{"Movie (2019)/a1b2c3.mkv"}, want: []string{"movies/Movie (2019)/Movie (2019).mkv"}},
		{files: []string{"Suicide Squad EXTENDED (2016) 2160p.mkv"}, want: []string{"movies/Suicide Squad (2016)/Suicide Squad (2016) - Extended.mkv"}},
		{files: []string{"Heat.1995.Directors.Cut.1080p.mkv"}, movies: []string{"Heat (1995)"}, want: []string{"movies/Heat (1995)/Heat (1995) - Directors Cut.mkv"}},
		{
			files: []string{"How.To.Be.Single.2016.1080p.BluRay.x264-BLOW/blow-how.to.be.single.2016.1080p.bluray.x264.mkv"},
			want:  []string{"movies/How To Be Single (2016)/How To Be Single (2016).mkv"},
		},
	}
	for _, test := range tests {
		torrent, plan := plan(test.files, test.shows, test.movies)
		var got []string
		for _, link := range plan.Links {
			got = append(got, link.To)
		}
		if !slices.Equal(got, test.want) {
			t.Errorf("%v: got %v, want %v, doubts %v", test.files, got, test.want, doubts(torrent, plan))
		}
	}
}

func TestDoubts(t *testing.T) {
	tests := []struct {
		files  []string
		shows  []string
		movies []string
	}{
		// Episodes that look like movies
		{files: []string{"Planet.Earth.II.2016.Part.3.Jungles.mkv"}},
		{files: []string{"Show.Name.2010.11.23.HDTV.XViD.mkv"}},
		{files: []string{"Show.Name.03.04.2012.HDTV.mkv"}, shows: []string{"Show Name"}},
		{files: []string{"the.flash.2014.208.hdtv-lol.mkv"}},
		{files: []string{"[Taka]_Fullmetal_Alchemist_(2009)_04_[720p][40F2A957].mp4"}},
		{files: []string{"Mastercook Italia - Stagione 6 (2016) 720p Episodio 13.mkv"}},
		{files: []string{"The Twilight Zone 1985 S01E22c The Library.mp4"}},
		{files: []string{"Caprica (2008)/Season 1/Apotheosis_1920x1080.mp4"}},
		// Episodes not read in full
		{files: []string{"Show.Name.1x02.1x03.HDTV.mkv"}},
		{files: []string{"Series Title.S6.E1.E2.Episode Name.1080p.mkv"}},
		{files: []string{"Avatar.The.Last.Airbender.Book.1.Water/01 The Boy in the Iceberg.avi"}},
		{files: []string{"[HorribleSubs] Detective Conan - 862 [1080p].mkv"}},
		{files: []string{"[K-F] Some Anime Show 214 10x14.mkv"}},
		// Titles cut in the wrong place
		{files: []string{"Superman 2.mkv"}},
		{files: []string{"超能警探.Memorist.S01E01.2160p.mkv"}},
		{files: []string{"blow-how.to.be.single.2016.1080p.bluray.x264.mkv"}},
		// Torrents whose videos disagree
		{files: []string{"Show.S01E01.mkv", "Show.S01E01.REPACK.mkv"}},
		{files: []string{"Show.S01E01.mkv", "Other.S01E02.mkv"}},
		{files: []string{"Show.S01E01.mkv", "Show.Special.mkv"}},
		{files: []string{"Movie.One.2019.mkv", "Movie.Two.2020.mkv"}},
		{files: []string{"Movie (2019)/Other.Movie.2018.mkv"}},
		// Names close to ones in the library
		{files: []string{"Doctor.Who.S01E01.mkv"}, shows: []string{"Doctor Who (2005)"}},
		{files: []string{"Fargo.1996.mkv"}, movies: []string{"Fargo (1996)"}},
		{files: []string{"readme.txt"}},
	}
	for _, test := range tests {
		torrent, plan := plan(test.files, test.shows, test.movies)
		if len(plan.Links) > 0 {
			t.Errorf("%v: linked as %v", test.files, plan.Links)
		}
		if len(doubts(torrent, plan)) == 0 {
			t.Errorf("%v: no doubt said why", test.files)
		}
	}
}

func TestLeftOut(t *testing.T) {
	torrent := Read([]File{
		{Path: "Movie (2019)/Movie.2019.mkv", Size: 4000},
		{Path: "Movie (2019)/Movie.2019.sample.mkv", Size: 40},
		{Path: "Movie (2019)/Extras/Interview.mkv", Size: 400},
		{Path: "Movie (2019)/Making.Of.mkv", Size: 300},
	})
	var got []string
	for _, video := range torrent.LeftOut {
		got = append(got, video.Path)
	}
	slices.Sort(got)
	want := []string{"Movie (2019)/Extras/Interview.mkv", "Movie (2019)/Making.Of.mkv", "Movie (2019)/Movie.2019.sample.mkv"}
	if !slices.Equal(got, want) {
		t.Errorf("left out %v, want %v", got, want)
	}
}

func TestAnswers(t *testing.T) {
	torrent, first := plan([]string{"Planet.Earth.II.2016.Part.3.Jungles.mkv"}, nil, nil)
	if len(first.Links) > 0 {
		t.Fatalf("linked as %v before the answer", first.Links)
	}
	answer := torrent.Videos[0]
	answer.Kind, answer.Title, answer.Year, answer.Season, answer.Episodes, answer.Doubts = "episode", "Planet Earth II", 0, 1, []int{3}, nil
	got := Resolve([]Reading{answer}, nil, nil, true)
	want := []Link{{"Planet.Earth.II.2016.Part.3.Jungles.mkv", "shows/Planet Earth II/Season 01/03.mkv"}}
	if !slices.Equal(got.Links, want) {
		t.Errorf("got %v, want %v", got.Links, want)
	}
}

func TestQuestions(t *testing.T) {
	tests := []struct {
		file  string
		field string
		want  Reading
	}{
		{"[SubsPlease] Frieren - 05 (1080p) [ABCD1234].mkv", "season", Reading{Kind: "episode", Title: "Frieren", Season: -1, Episodes: []int{5}}},
		{"Frieren Ep 05.mkv", "season", Reading{Kind: "episode", Title: "Frieren", Season: -1, Episodes: []int{5}}},
		{"Superman 2.mkv", "kind", Reading{Kind: "movie", Title: "Superman 2"}},
		{"[Taka]_Fullmetal_Alchemist_(2009)_04_[720p].mp4", "season", Reading{Kind: "episode", Title: "Fullmetal Alchemist", Year: 2009, Season: -1, Episodes: []int{4}}},
		{"Show.Name.S01/Show.Name.Special.720p.mkv", "special", Reading{Kind: "episode", Title: "Show Name", Season: 0, Name: "Show Name Special"}},
		{"Planet.Earth.II.2016.Part.3.mkv", "kind", Reading{Kind: "movie", Title: "Planet Earth II", Year: 2016}},
	}
	for _, test := range tests {
		got := Read([]File{{Path: test.file}}).Videos[0]
		if got.Kind != test.want.Kind || got.Title != test.want.Title || got.Year != test.want.Year || got.Season != test.want.Season ||
			!slices.Equal(got.Episodes, test.want.Episodes) || got.Name != test.want.Name {
			t.Errorf("%s: read as %+v", test.file, got)
		}
		doubts := got.Doubts
		if got.AsEpisode != nil {
			doubts = append(doubts, got.AsEpisode.Doubts...)
		}
		if !slices.ContainsFunc(doubts, func(d Doubt) bool { return d.Field == test.field }) {
			t.Errorf("%s: no question about %s in %v", test.file, test.field, got.Doubts)
		}
	}
}

func TestLeftAlone(t *testing.T) {
	for _, file := range []string{
		"Show.Name.S01E01E02.1080p.mkv",
		"Show.Name.S03E01-03.mkv",
		"Friends - [7x23-24] - The One with the Wedding.mkv",
		"Show Name - 1x02-1x03 - Title.mkv",
		"Daily.Show.2024.03.12.Guest.720p.mkv",
		"Some.Late.Show.03.04.2024.720p.mkv",
	} {
		video := Read([]File{{Path: file}}).Videos[0]
		if !slices.ContainsFunc(video.Doubts, func(d Doubt) bool { return d.Field == "unsupported" }) {
			t.Errorf("%s: not left alone, doubts %v", file, video.Doubts)
		}
	}
}

func TestTwoReadings(t *testing.T) {
	tests := []struct {
		file    string
		movie   string
		episode Reading
	}{
		{"Superman 2.mkv", "Superman 2", Reading{Title: "Superman", Season: -1, Episodes: []int{2}}},
		{"Planet.Earth.II.2016.Part.3.Jungles.mkv", "Planet Earth II", Reading{Title: "Planet Earth II", Year: 2016, Season: -1, Episodes: []int{3}}},
		{"Mastercook Italia Stagione 6 (2016) 720p.mkv", "Mastercook Italia Stagione 6", Reading{Title: "Mastercook Italia Stagione 6", Year: 2016, Season: 6}},
	}
	for _, test := range tests {
		got := Read([]File{{Path: test.file}}).Videos[0]
		if got.Kind != "movie" || got.Title != test.movie || got.AsEpisode == nil {
			t.Fatalf("%s: read as %+v", test.file, got)
		}
		e := got.AsEpisode
		if e.Title != test.episode.Title || e.Year != test.episode.Year || e.Season != test.episode.Season || !slices.Equal(e.Episodes, test.episode.Episodes) {
			t.Errorf("%s: read as an episode as %+v", test.file, *e)
		}
	}
}
