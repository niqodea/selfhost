package main

import (
	"net/url"
	"slices"
	"testing"
)

// Most names below come from the tests of other release name parsers
// Ref: https://github.com/guessit-io/guessit/blob/3c73072b8971d412935c6f749be63c564dd41385/guessit/test/episodes.yml
// Ref: https://github.com/guessit-io/guessit/tree/3c73072b8971d412935c6f749be63c564dd41385/guessit/test/cross_parser
// Ref: https://github.com/Sonarr/Sonarr/tree/9a41dfc129d5bbf8f3950f871623b46eea4f914e/src/NzbDrone.Core.Test/ParserTests

// outcome is what importling does with a torrent: where it hands each
// episode on, or else why it imports nothing
func outcome(paths ...string) (episodes []string, why []string) {
	var files []File
	for _, file := range paths {
		files = append(files, File{Path: file})
	}
	torrent := Read(files)
	for _, video := range torrent.Videos {
		if video.Unsupported != "" {
			why = append(why, video.Unsupported)
		}
	}
	if len(torrent.Videos) == 0 {
		why = append(why, "Has no video")
	}
	if why != nil {
		return nil, why
	}
	form := NewForm(torrent)
	if !form.Settled() {
		return nil, questions(form)
	}
	for _, link := range form.Links() {
		episodes = append(episodes, link.To)
	}
	return episodes, nil
}

// questions are the questions of the form's open fields
func questions(f Form) []string {
	all := f.Show.Questions()
	for _, row := range f.Rows {
		if row.Include {
			all = append(append(all, row.Season.Questions()...), row.Episode.Questions()...)
		}
	}
	return all
}

func TestImports(t *testing.T) {
	tests := []struct {
		files []string
		want  []string
	}{
		{[]string{"The.Office.US.S02E03.1080p.WEB-DL.x264-GRP.mkv"}, []string{"The Office US/Season 02/03.mkv"}},
		{[]string{"Doctor.Who.2005.S01E01.Rose.mkv"}, []string{"Doctor Who 2005/Season 01/01.mkv"}},
		{[]string{"Doctor Who (2005)/Season 1/Doctor.Who.S01E01.mkv"}, []string{"Doctor Who (2005)/Season 01/01.mkv"}},
		{[]string{"The.Office.US.S02/the.office.us.s02e01.mkv"}, []string{"The Office US/Season 02/01.mkv"}},
		{[]string{"Arcane 2021 Seasons 1 and 2 Complete 1080p x264/Arcane.S01E01.1080p.mkv"}, []string{"Arcane/Season 01/01.mkv"}},
		{[]string{"24.S03E04.720p.mkv"}, []string{"24/Season 03/04.mkv"}},
		{[]string{"Show Name - 1x02 - Title.mkv"}, []string{"Show Name/Season 01/02.mkv"}},
		{[]string{"CSI.S013E18.Sheltered.720p.WEB-DL.mkv"}, []string{"CSI/Season 13/18.mkv"}},
		{[]string{"One Piece S01E1056 1080p WEB.mkv"}, []string{"One Piece/Season 01/1056.mkv"}},
		{[]string{"Parks_and_Recreation-s03-e01.mkv"}, []string{"Parks and Recreation/Season 03/01.mkv"}},
		{[]string{"The Office - S06xE01.avi"}, []string{"The Office/Season 06/01.avi"}},
		{[]string{"[Judas] Aharen-san wa Hakarenai - S01E06v2.mkv"}, []string{"Aharen-san wa Hakarenai/Season 01/06.mkv"}},
		{[]string{"Show.Name.s10e15(233).480p.BDRip.mkv"}, []string{"Show Name/Season 10/15.mkv"}},
		{[]string{"We.Bare.Bears.S01E14.1080p.WEB-DL/mxNMuJWeO7PUWCMEwqKSsS6D8Vs9S6V3PHD.mkv"}, []string{"We Bare Bears/Season 01/14.mkv"}},
		{
			[]string{"Breaking Bad Season 2 Complete 720p/03 - Bit by a Dead Bee.mkv", "Breaking Bad Season 2 Complete 720p/04 - Down.mkv"},
			[]string{"Breaking Bad/Season 02/03.mkv", "Breaking Bad/Season 02/04.mkv"},
		},
		{
			[]string{"Show.Name.S01.1080p/S01E01.mkv", "Show.Name.S01.1080p/Sample/s01e01-sample.mkv", "Show.Name.S01.1080p/info.nfo"},
			[]string{"Show Name/Season 01/01.mkv"},
		},
	}
	for _, test := range tests {
		got, why := outcome(test.files...)
		if !slices.Equal(got, test.want) {
			t.Errorf("%v: got %v, want %v, unsettled %v", test.files, got, test.want, why)
		}
	}
}

func TestDoubts(t *testing.T) {
	for _, files := range [][]string{
		// Names with no season and episode, like a movie's, a daily show's or
		// an episode numbered from the first
		{"Fargo.1996.1080p.mkv"},
		{"Show.Name.2010.11.23.HDTV.XViD.mkv"},
		{"[HorribleSubs] Detective Conan - 862 [1080p].mkv"},
		{"The Twilight Zone 1985 S01E22c The Library.mp4"},
		// Episodes not read in full
		{"Show.Name.1x02.1x03.HDTV.mkv"},
		{"Series Title.S6.E1.E2.Episode Name.1080p.mkv"},
		// Names cut in the wrong place
		{"grp-show.name.s01e01.1080p.mkv"},
		// A directory named as one episode, with more than one video
		{"Show.S01E01/a1b2c3.mkv", "Show.S01E01/Behind.The.Scenes.mkv"},
		// Torrents whose videos disagree
		{"Show.S01/Show.S01E01.mkv", "Show.S01/Show.S01E01.REPACK.mkv"},
		// A torrent whose name gives no show
		{"S01/S01E01.mkv"},
		{"readme.txt"},
	} {
		episodes, why := outcome(files...)
		if len(episodes) > 0 {
			t.Errorf("%v: imported as %v", files, episodes)
		}
		if len(why) == 0 {
			t.Errorf("%v: nothing said why", files)
		}
	}
}

func TestLeftOut(t *testing.T) {
	torrent := Read([]File{
		{Path: "Show.S01/Show.S01E01.mkv"},
		{Path: "Show.S01/Show.S01E01.sample.mkv"},
		{Path: "Show.S01/Extras/Interview.mkv"},
		{Path: "Show.S01/Show.S01E01.nfo"},
	})
	var got []string
	for _, video := range torrent.LeftOut {
		got = append(got, video.Path)
	}
	slices.Sort(got)
	want := []string{"Show.S01/Extras/Interview.mkv", "Show.S01/Show.S01E01.sample.mkv"}
	if !slices.Equal(got, want) {
		t.Errorf("left out %v, want %v", got, want)
	}
}

func TestLeftAlone(t *testing.T) {
	for _, file := range []string{
		"Show.Name.S01E01E02.1080p.mkv",
		"Show.Name.S03E01-03.mkv",
		"Friends - [7x23-24] - The One with the Wedding.mkv",
		"Show Name - 1x02-1x03 - Title.mkv",
		"Shortland.Series.S22E5363-E5366.HDTV.mkv",
	} {
		if video := Read([]File{{Path: file}}).Videos[0]; video.Unsupported == "" {
			t.Errorf("%s: not left alone, read as %+v", file, video)
		}
	}
}

func TestAnswers(t *testing.T) {
	form := NewForm(Read([]File{{Path: "Breaking Bad/a.mkv"}, {Path: "Breaking Bad/b.mkv"}}))
	if form.Settled() {
		t.Fatal("settled before the answer")
	}
	answer := url.Values{"show": {"Breaking Bad"}, "season": {"2"}, "0.include": {"on"}, "0.episode": {"3"}, "1.include": {"on"}, "1.episode": {"3"}}
	if form := form.Answer(answer); form.Settled() {
		t.Error("imported two files as one episode")
	}
	answer.Set("1.episode", "4")
	form = form.Answer(answer)
	want := []Link{{"Breaking Bad/a.mkv", "Breaking Bad/Season 02/03.mkv"}, {"Breaking Bad/b.mkv", "Breaking Bad/Season 02/04.mkv"}}
	if got := form.Links(); !form.Settled() || !slices.Equal(got, want) {
		t.Errorf("got %+v, questions %v", got, questions(form))
	}

	// A file left out needs no numbers
	answer.Del("1.include")
	answer.Del("1.episode")
	if form := form.Answer(answer); !form.Settled() || len(form.Links()) != 1 {
		t.Errorf("questions %v", questions(form))
	}
}

func TestJoin(t *testing.T) {
	a, b, c := sure(1), sure(2), sure(1).doubt("a guess")
	for _, f := range []Field[int]{a, b, c} {
		if !slices.Equal(join(f, Field[int]{}).Values, f.Values) || !slices.Equal(join(f, f).Values, f.Values) {
			t.Errorf("%v joined with nothing or with itself changes", f)
		}
	}
	if !join(a, a).Sure() || join(a, b).Sure() || join(b, a).Sure() || join(a, c).Sure() {
		t.Error("two sources that disagree or doubt make a sure field")
	}
}
