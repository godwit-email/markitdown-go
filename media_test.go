package markitdown

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

func TestYouTubeVideoIDs(t *testing.T) {
	cases := []struct{ url, id string }{
		{"https://www.youtube.com/watch?v=V2qZ_lgxTzg", "V2qZ_lgxTzg"},
		{"https://youtu.be/V2qZ_lgxTzg", "V2qZ_lgxTzg"},
		{"https://www.youtube.com/shorts/V2qZ_lgxTzg", "V2qZ_lgxTzg"},
		{"https://www.youtube.com/embed/V2qZ_lgxTzg", "V2qZ_lgxTzg"},
	}
	for _, tc := range cases {
		if got := youtubeVideoID(tc.url); got != tc.id {
			t.Fatalf("%s: %q", tc.url, got)
		}
		res, err := ExtractPage(strings.NewReader("<html></html>"), "", "a.html", tc.url, 0)
		if err != nil || res.Kind != KindYouTube {
			t.Fatalf("%s accept %#v %v", tc.url, res, err)
		}
	}
}

func TestYouTubeMetadata(t *testing.T) {
	cases := []struct{ head, title, content string }{
		{"<title>Video title</title>", "Video title", "\n## Video title\n"},
		{`<meta property="og:title" content="Video title">`, "Video title", "\n## Video title\n"},
		{`<meta itemprop="name" content="Video title">`, "Video title", "\n## Video title\n"},
		{`<meta name="description" content="Video description">`, "", "\n### Description\nVideo description\n"},
		{`<meta property="og:description" content="Video description">`, "", "\n### Description\nVideo description\n"},
		{`<script>var ytInitialData = {"attributedDescriptionBodyText": {"content": "Video description"}};</script>`, "", "\n### Description\nVideo description\n"},
		{
			`<meta itemprop="interactionCount" content="42"><meta name="keywords" content="example"><meta itemprop="duration" content="PT1M">`,
			"",
			"\n### Video Metadata\n- **Views:** 42\n- **Keywords:** example\n- **Runtime:** PT1M\n\n",
		},
	}
	page := "https://www.youtube.com/watch?v=12345"
	for _, tc := range cases {
		html := "<html><head>" + tc.head + "</head><body>Unused HTML body</body></html>"
		res, err := ExtractPage(strings.NewReader(html), "text/html; charset=utf-8", "a.html", page, 0)
		if err != nil {
			t.Fatal(err)
		}
		if res.Title != tc.title || res.Text != "# YouTube\n"+tc.content {
			t.Fatalf("head %s\n title %q\n got %q", tc.head, res.Title, res.Text)
		}
		if strings.Contains(res.Text, "Unused HTML body") {
			t.Fatalf("body leaked: %s", res.Text)
		}
	}
}

func TestYouTubeMissingTitleFallsBackToHTML(t *testing.T) {
	page := "https://www.youtube.com/watch?v=12345"
	cases := []string{
		"<html><head></head><body>Video Content</body></html>",
		"<html><head><title></title></head><body>Video Content</body></html>",
	}
	for _, html := range cases {
		res, err := ExtractPage(strings.NewReader(html), "text/html", "a.html", page, 0)
		if err != nil {
			t.Fatal(err)
		}
		if res.Title != "" || res.Text != "Video Content" {
			t.Fatalf("%q %#v", html, res)
		}
	}
	res, err := ExtractPage(strings.NewReader("<html><head><title>Fallback Title</title></head><body>Video Content</body></html>"), "text/html", "a.html", page, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Fallback Title" || res.Text != "# YouTube\n\n## Fallback Title\n" {
		t.Fatalf("%#v", res)
	}
	if strings.Contains(res.Text, "Video Content") {
		t.Fatalf("body kept: %s", res.Text)
	}
}

func TestYouTubeEmptyExtractionPreservesHTML(t *testing.T) {
	body := "<html><head><script>do_not_render_script()</script><style>.do_not_render_style { color: red; }</style></head><body><h1>Saved transcript</h1><table><tr><th>Time</th><th>Text</th></tr><tr><td>00:01</td><td>caf\u00e9</td></tr></table></body></html>"
	encoded, err := charmap.Windows1252.NewEncoder().Bytes([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	// The Python test seeks past a prefix before convert. Extract reads the
	// bytes it is given, so the prefix is not part of this input.
	res, err := ExtractPage(bytes.NewReader(encoded), "text/html; charset=windows-1252", "a.html", "https://www.youtube.com/watch?v=12345", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Saved transcript #\n\n| Time | Text |\n| --- | --- |\n| 00:01 | café |"
	if res.Text != want || res.Title != "" {
		t.Fatalf("%q title %q", res.Text, res.Title)
	}
	if strings.Contains(res.Text, "do_not_render") {
		t.Fatalf("%s", res.Text)
	}
	plain, err := Extract(strings.NewReader("<h1>Paris</h1>"), "text/html", "a.html", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.Text, "# Paris #") || !strings.Contains(plain.Text, "# Paris") {
		t.Fatalf("default heading changed: %s", plain.Text)
	}
}

func TestIPYNBHeadingKeepsHash(t *testing.T) {
	nb := `{"nbformat":4,"nbformat_minor":5,"metadata":{},"cells":[{"cell_type":"markdown","source":["# #hashtag campaign results\n"],"metadata":{}}]}`
	res, err := Extract(strings.NewReader(nb), "", "notes.ipynb", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "#hashtag campaign results" {
		t.Fatalf("%q", res.Title)
	}
}

func TestIPYNBExactAndBOM(t *testing.T) {
	raw := `{"nbformat":4,"nbformat_minor":5,"metadata":{},"cells":[{"cell_type":"markdown","source":["# Quarterly notes\n"],"metadata":{}},{"cell_type":"code","source":["print('hello')\n"],"metadata":{}}]}`
	want := "# Quarterly notes\n\n\n```python\nprint('hello')\n\n```"
	res, err := Extract(strings.NewReader(raw), "application/json; charset=utf-8", "notes.ipynb", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != want || res.Title != "Quarterly notes" || res.Kind != KindIPYNB {
		t.Fatalf("%#v", res)
	}
	bom := append([]byte{0xEF, 0xBB, 0xBF}, raw...)
	res, err = Extract(bytes.NewReader(bom), "", "notes.ipynb", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "# Quarterly notes") || strings.Contains(res.Text, "nbformat") || res.Title != "Quarterly notes" {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Text, "```python\nprint('hello')") {
		t.Fatalf("%s", res.Text)
	}
	res, err = Extract(bytes.NewReader(bom), "application/json; charset=utf-8-sig", "notes.ipynb", 0)
	if err != nil || !strings.Contains(res.Text, "# Quarterly notes") {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestIPYNBFixture(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "test_notebook.ipynb")), "application/json", "test_notebook.ipynb", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"# Test Notebook", "```python", `print("markitdown")`, "```", "## Code Cell Below"} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
	if strings.Contains(res.Text, "nbformat") || strings.Contains(res.Text, "nbformat_minor") {
		t.Fatalf("raw notebook json leaked:\n%s", res.Text)
	}
}

func TestIPYNBRejectsUndecodableJSON(t *testing.T) {
	res, err := Extract(bytes.NewReader([]byte{0x80, 0x81, 0x82, 0x83}), "application/json; charset=utf-8", "a.json", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind == KindIPYNB {
		t.Fatalf("binary json accepted as notebook: %#v", res)
	}
	french := []byte("lettre d'information sur l'événement")
	res, err = Extract(bytes.NewReader(french), "application/json; charset=ascii", "a.json", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind == KindIPYNB {
		t.Fatalf("%#v", res)
	}
	ok := `{"nbformat": 4, "nbformat_minor": 5, "cells": []}`
	res, err = Extract(strings.NewReader(ok), "application/json; charset=utf-8", "a.json", 0)
	if err != nil || res.Kind != KindIPYNB {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestCharsetSample(t *testing.T) {
	const sampleSize = 65536
	others := []struct{ sample, tail []byte }{
		{nil, nil},
		{[]byte("ordinary text"), nil},
		{[]byte("short incomplete \xc3"), nil},
		{bytes.Repeat([]byte("a"), sampleSize), []byte{0xC3, 0xA9}},
		{append(bytes.Repeat([]byte("a"), sampleSize-2), 0xC3, 0xA9), []byte{0xF0, 0x9F, 0x98, 0x80}},
		{append(bytes.Repeat([]byte("a"), sampleSize-1), 0xC3), nil},
		{append(bytes.Repeat([]byte("a"), sampleSize-1), 0xF0), []byte{0x9F}},
		{append(bytes.Repeat([]byte("a"), sampleSize-1), 0xC3), []byte("x")},
		{append(bytes.Repeat([]byte("a"), sampleSize-1), 0xE0), []byte{0x80, 0x80}},
		{append(bytes.Repeat([]byte("a"), sampleSize-1), 0xED), []byte{0xA0, 0x80}},
		{append(append([]byte{0xFF}, bytes.Repeat([]byte("a"), sampleSize-2)...), 0xC3), []byte{0xA9}},
	}
	for i, tc := range others {
		data := append(append([]byte{}, tc.sample...), tc.tail...)
		r := bytes.NewReader(data)
		got, err := readCharsetSample(r)
		if err != nil {
			t.Fatal(i, err)
		}
		if !bytes.Equal(got, tc.sample) {
			t.Fatalf("case %d len %d want %d", i, len(got), len(tc.sample))
		}
		if tell := len(data) - r.Len(); tell > sampleSize+3 {
			t.Fatalf("case %d tell %d", i, tell)
		}
	}
	prefix := []byte(`{"name":"résumé","notes":"`)
	for _, ch := range []string{"é", "日", "😀"} {
		enc := []byte(ch)
		for split := 1; split < len(enc); split++ {
			data := append(append([]byte{}, prefix...), bytes.Repeat([]byte("a"), sampleSize-split-len(prefix))...)
			data = append(data, []byte(ch+`"}`)...)
			r := bytes.NewReader(data)
			got, err := readCharsetSample(r)
			if err != nil {
				t.Fatal(err)
			}
			wantN := sampleSize + len(enc) - split
			if !bytes.Equal(got, data[:wantN]) || len(data)-r.Len() != wantN || wantN > sampleSize+3 {
				t.Fatalf("%q split %d got %d tell %d", ch, split, len(got), len(data)-r.Len())
			}
			if !utf8.Valid(got) {
				t.Fatalf("%q split %d sample is not utf-8", ch, split)
			}
			res, err := Extract(bytes.NewReader(data), "", "", 0)
			if err != nil {
				t.Fatal(err)
			}
			if res.Text != string(data) {
				t.Fatalf("%q split %d text mismatch", ch, split)
			}
		}
	}
}

func TestExplicitCharsetStillWins(t *testing.T) {
	prefix := []byte(`{"name":"résumé","notes":"`)
	data := append(append([]byte{}, prefix...), bytes.Repeat([]byte("a"), 65536-1-len(prefix))...)
	data = append(data, []byte("é"+`"}`)...)
	want, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Extract(bytes.NewReader(data), "application/json; charset=cp1252", "a.json", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != string(want) {
		t.Fatal("cp1252 did not win")
	}
}

func TestLateNonASCIIIndex(t *testing.T) {
	data := testFile(t, "json_late_non_ascii.json")
	first := -1
	for i, b := range data {
		if b >= 128 {
			first = i
			break
		}
	}
	if first <= 8192 || first >= 65536 {
		t.Fatalf("first non-ascii at %d", first)
	}
}

func TestBinaryExtensionsAreNotTheWordNone(t *testing.T) {
	undecodable := undecodableBytes()
	for _, extn := range []string{".txt", ".md"} {
		res, err := Extract(bytes.NewReader(undecodable), "", "a"+extn, 0)
		if err != nil {
			t.Fatal(extn, err)
		}
		if res.Text == "None" {
			t.Fatalf("%s became None", extn)
		}
	}
	res, err := Extract(bytes.NewReader(undecodable), "", "a.csv", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text == "| None |\n| --- |" {
		t.Fatalf("%s", res.Text)
	}
}

func undecodableBytes() []byte {
	b := make([]byte, 4096)
	for i := range b {
		b[i] = byte((7*i*i + 251*i + 193) % 256)
	}
	return b
}

func TestImageAndAudioMetadata(t *testing.T) {
	jpg, err := Extract(bytes.NewReader(testFile(t, "test.jpg")), "image/jpeg", "test.jpg", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"Author: AutoGen Authors",
		"Title: AutoGen: Enabling Next-Gen LLM Applications via Multi-Agent Conversation",
		"Description: AutoGen enables diverse LLM-based applications",
		"ImageSize: 1615x1967",
		"DateTimeOriginal: 2024:03:14 22:10:00",
	} {
		if !strings.Contains(jpg.Text, s) {
			t.Fatalf("jpg missing %s\n%s", s, jpg.Text)
		}
	}
	if strings.Contains(jpg.Text, "# Description:") {
		t.Fatal("llm caption invented")
	}
	empty, err := Extract(bytes.NewReader([]byte("not a jpeg")), "", "a.jpg", 0)
	if err != nil || empty.Text != "" || empty.Kind != KindImage {
		t.Fatalf("%#v %v", empty, err)
	}
	mp3, err := Extract(bytes.NewReader(testFile(t, "test.mp3")), "", "test.mp3", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"Title: f67a499e-a7d0-4ca3-a49b-358bd934ae3e",
		"Artist: Artist Name Test String",
		"Album: Album Name Test String",
		"SampleRate: 48000",
	} {
		if !strings.Contains(mp3.Text, s) {
			t.Fatalf("mp3 missing %s\n%s", s, mp3.Text)
		}
	}
	if strings.Contains(mp3.Text, "### Audio Transcript:") {
		t.Fatal("transcript invented")
	}
}

func TestAudioWithoutTranscriptIsNotUnsupported(t *testing.T) {
	for _, name := range []string{"test.wav", "test.m4a"} {
		res, err := Extract(bytes.NewReader(testFile(t, name)), "", name, 0)
		if err != nil {
			t.Fatal(name, err)
		}
		if res.Kind != KindAudio {
			t.Fatalf("%s %#v", name, res)
		}
	}
	_, err := Extract(bytes.NewReader([]byte{0x00, 0x01}), "application/octet-stream", "a.bin", 0)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
