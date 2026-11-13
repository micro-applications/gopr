package main

import (
	"strings"
	"testing"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

const markdownDesc = "<!-- Describe the change -->\n## Why\n\nThe greeting was **too long**.\n\n- Shorter\n- Louder\n\nSee [the issue](https://github.com/acme/app/issues/3).\n\n<img alt=\"before and after\" src=\"https://example.com/x.png\">\n"

// openedDescribed opens acme/app#1 with a description.
func openedDescribed(t *testing.T, body string) (*window, *ui.Tester) {
	t.Helper()
	freshState(t)
	clone, base, head := pullClone(t, acmeApp)
	(&fakeGitHub{base: base, head: head, comments: `[]`, body: body}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: realPath(t, clone)})
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	tt.SetSize(1400, 1000)
	for range 4 {
		tt.Frame()
	}
	if w.loadErr != nil || w.openErr != nil || w.pr == nil || w.pr.meta == nil {
		t.Fatalf("not opened: %v, %v", w.loadErr, w.openErr)
	}
	return w, tt
}

func TestDescriptionMarkdown(t *testing.T) {
	_, tt := openedDescribed(t, markdownDesc)
	snapshot(t, tt, "description")
	for _, want := range []string{"Why", "Shorter", "Louder", "[before and after]"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	texts := strings.Join(tt.Texts(), "\n")
	for _, raw := range []string{"## Why", "**too long**", "- Shorter", "](https://", "<img", "Describe the change"} {
		if strings.Contains(texts, raw) {
			t.Errorf("the description shows %q as written", raw)
		}
	}
	if _, ok := tt.Find("Pull request description"); !ok {
		t.Error("no description")
	}
	if tt.HasText("Show more") {
		t.Error("a short description is collapsed")
	}
	// Its links open in the browser.
	var opened []string
	old := openExternal
	openExternal = func(url string) { opened = append(opened, url) }
	t.Cleanup(func() { openExternal = old })
	if err := tt.Click("the issue"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(opened) != 1 || opened[0] != "https://github.com/acme/app/issues/3" {
		t.Errorf("opened %q", opened)
	}
}

func TestDescriptionLong(t *testing.T) {
	var b strings.Builder
	for i := range 40 {
		b.WriteString("- Item number ")
		b.WriteString(strings.Repeat("x", i%5+1))
		b.WriteString("\n")
	}
	w, tt := openedDescribed(t, b.String())
	snapshot(t, tt, "description-long")
	short, ok := tt.Find("Pull request description")
	if !ok || short.H > descCollapsed+1 {
		t.Fatalf("collapsed description %+v", short)
	}
	if err := tt.Click("Show more"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if full, _ := tt.Find("Pull request description"); full.H <= short.H*2 || !w.pr.descOpen {
		t.Errorf("Show more: %v tall, from %v", full.H, short.H)
	}
	if !tt.HasText("Show less") {
		t.Error("no Show less")
	}
}

func TestHTMLText(t *testing.T) {
	for in, want := range map[string]string{
		`<img alt="shot" src="x.png">`:                           "[shot]",
		"<details>\n<summary>More</summary>\n\nText\n</details>": "More\nText",
		"<br/>":                                  "",
		`<p align="center">  Hello   world </p>`: "Hello world",
	} {
		if got := htmlText(in); got != want {
			t.Errorf("htmlText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommentsMarkdown(t *testing.T) {
	freshState(t)
	clone, base, head := pullClone(t, acmeApp)
	(&fakeGitHub{base: base, head: head, reviewBody: "Please **rename** `greet` first.", comments: `[
		{"id":1,"path":"main.go","side":"RIGHT","line":6,"user":{"login":"cy"},"body":"Why the *exclamation* mark?\n\n- louder\n- shorter","created_at":"2026-10-01T10:00:00Z"},
		{"id":2,"in_reply_to_id":1,"path":"main.go","side":"RIGHT","line":6,"user":{"login":"bo"},"body":"See [the guide](https://example.com/guide).","created_at":"2026-10-01T11:00:00Z"}]`}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: realPath(t, clone)})
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	tt.SetSize(1400, 1200)
	for range 5 {
		tt.Frame()
	}
	snapshot(t, tt, "comments-markdown")
	for _, want := range []string{"rename", "greet", "exclamation", "louder", "shorter", "the guide"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	texts := strings.Join(tt.Texts(), "\n")
	for _, raw := range []string{"**rename**", "`greet`", "*exclamation*", "- louder", "](https://"} {
		if strings.Contains(texts, raw) {
			t.Errorf("a comment shows %q as written", raw)
		}
	}
	var opened []string
	old := openExternal
	openExternal = func(url string) { opened = append(opened, url) }
	t.Cleanup(func() { openExternal = old })
	if err := tt.Click("the guide"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(opened) != 1 || opened[0] != "https://example.com/guide" {
		t.Errorf("opened %q", opened)
	}
}
