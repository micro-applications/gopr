package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/godiff/internal/git"
	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

func TestIsMarkdown(t *testing.T) {
	for path, want := range map[string]bool{
		"README.md":       true,
		"a/B.MARKDOWN":    true,
		"x.mdx":           true,
		"x.md.txt":        false,
		"md":              false,
		"docs/guide.Md":   true,
		"notes.markdown2": false,
	} {
		if got := isMarkdown(path); got != want {
			t.Errorf("isMarkdown(%q) = %v, want %v", path, got, want)
		}
	}
}

// readmeChange is README.md on the branch of the pull request.
const readmeChange = "# App\n\nSome **bold** text\n"

// openedReadmePull opens acme/app#1, which changes README.md to
// readmeChange as well, with the review comments of comments.
func openedReadmePull(t *testing.T, comments string) (*window, *ui.Tester) {
	t.Helper()
	freshState(t)
	clone, base, head := pullClone(t, acmeApp, readmeChange)
	(&fakeGitHub{base: base, head: head, comments: comments}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: realPath(t, clone)})
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	for range 4 {
		tt.Frame()
	}
	if w.loadErr != nil || w.openErr != nil || w.fileByPath("README.md") == nil {
		t.Fatalf("not opened: load error %v, open error %v, %d files", w.loadErr, w.openErr, len(w.files))
	}
	return w, tt
}

func TestMarkdownPreview(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	// Changes shows the diff alone.
	if !tt.HasText("Some **bold** text") {
		t.Errorf("Changes lacks the diff's lines: %q", tt.Texts())
	}
	if tt.HasText("Some bold text") {
		t.Error("Changes shows the rendered preview")
	}
	// The header's Preview opens the file rendered, in a tab.
	if err := tt.Click("Preview"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.activeTab == 0 || w.fileTabs[w.activeTab-1].path != "README.md" {
		t.Fatalf("Preview opened tab %d of %d", w.activeTab, len(w.fileTabs))
	}
	if !tt.HasText("Some bold text") || tt.HasText("**bold**") {
		t.Errorf("the tab does not show the preview: %q", tt.Texts())
	}

	// Another pull request starts afresh.
	w.mdSource = map[string]bool{"README.md": true}
	w.openPull(acmeApp, 2)
	tt.Frame()
	if len(w.mdSource) != 0 || len(w.mdOpts) != 0 {
		t.Errorf("choices kept: source %v, %d options", w.mdSource, len(w.mdOpts))
	}
}

func TestMarkdownThreadInChanges(t *testing.T) {
	w, tt := openedReadmePull(t, `[
		{"id":1,"path":"README.md","side":"RIGHT","line":3,"user":{"login":"cy"},"body":"Bolder?","created_at":"2026-10-01T10:00:00Z"}]`)
	for range 3 {
		tt.Frame()
	}
	if n := w.openThreads("README.md"); n != 1 {
		t.Fatalf("%d open threads on README.md", n)
	}
	if !tt.HasText("Bolder?") {
		t.Errorf("the thread does not show under its line: %q", tt.Texts())
	}
}

func TestMarkdownHunkNav(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	readme := slices.IndexFunc(w.files, func(f *fileState) bool { return f.Path == "README.md" })
	main := slices.IndexFunc(w.files, func(f *fileState) bool { return f.Path == "main.go" })
	if readme < 0 || main < 0 || readme > main {
		t.Fatalf("README.md at %d, main.go at %d: README.md must come first", readme, main)
	}
	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if w.selFile != readme || w.selHunk < 0 {
		t.Errorf("J chose file %d hunk %d, want README.md (%d)", w.selFile, w.selHunk, readme)
	}
}

func TestMarkdownImagePathEscape(t *testing.T) {
	w := &window{}
	o := w.markdownOptions("docs/a.md")
	for _, url := range []string{"../../etc/passwd", "https://x/y.png", "/etc/passwd", "//x/y.png", "data:image/png;base64,AA"} {
		if _, err := o.LoadImage(url); err == nil {
			t.Errorf("LoadImage(%q) loaded", url)
		}
	}
	if w.markdownOptions("docs/a.md") != o {
		t.Error("the options of a path are not kept")
	}
}

func TestImagePath(t *testing.T) {
	for _, tc := range []struct{ dir, url, want string }{
		{"docs", "../img/a.png", "img/a.png"},
		{"docs", "a.png", "docs/a.png"},
		{"docs", "./b/../a.png", "docs/a.png"},
		{".", "img/a.png", "img/a.png"},
		{"docs", "a%20b.png", "docs/a b.png"},
	} {
		if got, err := imagePath(tc.dir, tc.url); err != nil || got != tc.want {
			t.Errorf("imagePath(%q, %q) = %q, %v; want %q", tc.dir, tc.url, got, err, tc.want)
		}
	}
	for _, tc := range []struct{ dir, url string }{
		{"docs", "../../x"},
		{".", "../x.png"},
		{"docs", "/etc/passwd"},
		{"docs", "//h/x"},
		{"docs", "https://x/y"},
		{"docs", "data:image/png;base64,AAAA"},
		{"docs", `a\b`},
		{".", ".."},
		{"docs", ".."},
		{"docs", ""},
	} {
		if got, err := imagePath(tc.dir, tc.url); err == nil {
			t.Errorf("imagePath(%q, %q) = %q, want an error", tc.dir, tc.url, got)
		}
	}
}

func TestMarkdownImageAtHead(t *testing.T) {
	// A picture beside the repository, which ../ from its root reaches.
	outside := t.TempDir()
	writeFile(t, outside, "secret.svg", `<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4"/></svg>`)
	dir := filepath.Join(outside, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.name", "Ada")
	gitIn(t, dir, "config", "user.email", "ada@example.com")
	writeFile(t, dir, "docs/a.md", "![Logo](../img/logo.svg)\n")
	writeFile(t, dir, "img/logo.svg", `<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4"/></svg>`)
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "First commit")
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// At a revision, and in the work tree.
	for _, rev := range []string{"HEAD", ""} {
		w := &window{repo: repo, newRev: rev}
		o := w.markdownOptions("docs/a.md")
		if img, err := o.LoadImage("../img/logo.svg"); err != nil || img == nil {
			t.Errorf("rev %q: the picture did not load: %v", rev, err)
		}
		if _, err := o.LoadImage("../img/missing.svg"); err == nil {
			t.Errorf("rev %q: a missing picture loaded", rev)
		}
		// Out of the repository, from its root.
		if _, err := w.markdownOptions("README.md").LoadImage("../secret.svg"); err == nil {
			t.Errorf("rev %q: a picture out of the repository loaded", rev)
		}
	}
}

func TestMarkdownFileTab(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	w.openFileTab("README.md", 0)
	tt.Frame()
	if !tt.HasText("Some bold text") || tt.HasText("**bold**") {
		t.Fatalf("the tab does not show the preview: %q", tt.Texts())
	}
	if err := tt.Click("Source"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !w.mdSource["README.md"] || !tt.HasText("Some **bold** text") {
		t.Errorf("Source does not show the lines: %q", tt.Texts())
	}
	if err := tt.Click("Preview"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.mdSource["README.md"] || tt.HasText("**bold**") {
		t.Error("Preview does not show again")
	}

	// A line to show is shown in the source.
	w.openFileTab("README.md", 3)
	tt.Frame()
	if !tt.HasText("Some **bold** text") {
		t.Errorf("opening at a line shows %q", tt.Texts())
	}
}

func TestMarkdownDeletedShowsDiff(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.name", "Ada")
	gitIn(t, dir, "config", "user.email", "ada@example.com")
	writeFile(t, dir, "NOTES.md", "# Notes\n\nGone **soon**\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "First commit")
	gitIn(t, dir, "rm", "-q", "NOTES.md")
	_, tt := newTestWindow(t, dir)
	for range 2 {
		tt.Frame()
	}
	if _, ok := tt.Find("Preview"); ok {
		t.Error("the deleted file offers Preview")
	}
	if !tt.HasText("Gone **soon**") {
		t.Errorf("the removed lines do not show: %q", tt.Texts())
	}
}

func TestMarkdownNoteInChanges(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	note := &aiNote{path: "README.md", side: sideNew, line: 3, text: "Bolder?"}
	w.notes = map[string][]*aiNote{"README.md": {note}}
	w.rowsDirty = true
	tt.Frame()
	tt.Key(0, ui.KeyN)
	tt.Frame()
	if w.noteSel != note {
		t.Fatalf("note %+v", w.noteSel)
	}
	first, last := w.list.Visible()
	shown := false
	for r := first; r <= last && r < len(w.rows); r++ {
		shown = shown || w.rows[r].kind == rowAINote && w.rows[r].note == note
	}
	if !shown || !tt.HasText("Bolder?") {
		t.Errorf("the note does not show: %q", tt.Texts())
	}
}

func TestMarkdownWidth(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	old := cfg.Get()
	t.Cleanup(func() { cfg.Update(func(s *Settings) { *s = old }) })
	off := cfg.OnChange(func(s Settings) { w.applySettings(s) })
	t.Cleanup(off)
	// A big screen, where the tab is wider than any limit.
	tt.SetSize(2400, 1200)
	w.previewFile("README.md")

	width := func() float32 {
		t.Helper()
		tt.Frame()
		r, ok := tt.Find("Preview column")
		if !ok {
			t.Fatal("no preview column")
		}
		return r.W
	}
	if got := width(); got != 820 {
		t.Errorf("medium preview is %v wide, want 820", got)
	}
	// The tab's icon buttons choose another width.
	for _, mw := range markdownWidths {
		if _, ok := tt.Find(mw.name); !ok {
			t.Errorf("no %s button", mw.name)
		}
	}
	if err := tt.Click("Narrow"); err != nil {
		t.Fatal(err)
	}
	if w.settings.MarkdownWidth != "narrow" {
		t.Fatalf("width %q, want narrow", w.settings.MarkdownWidth)
	}
	narrow := width()
	if narrow != 640 {
		t.Errorf("narrow preview is %v wide, want 640", narrow)
	}
	if err := tt.Click("Full Width"); err != nil {
		t.Fatal(err)
	}
	if full := width(); full <= 1080 {
		t.Errorf("full-width preview is %v wide, want the tab's width", full)
	}
}

func TestMarkdownWidthSetting(t *testing.T) {
	s := Settings{MarkdownWidth: "huge"}
	s.normalize()
	if s.MarkdownWidth != "medium" {
		t.Errorf("unknown width normalized to %q, want medium", s.MarkdownWidth)
	}
	for _, mw := range markdownWidths {
		s := Settings{MarkdownWidth: mw.key}
		s.normalize()
		if s.MarkdownWidth != mw.key {
			t.Errorf("%q normalized to %q", mw.key, s.MarkdownWidth)
		}
	}
}

func TestMarkdownFontSize(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	old := cfg.Get()
	t.Cleanup(func() { cfg.Update(func(s *Settings) { *s = old }) })
	off := cfg.OnChange(func(s Settings) { w.applySettings(s) })
	t.Cleanup(off)
	w.previewFile("README.md")

	height := func() float32 {
		t.Helper()
		tt.Frame()
		r, ok := tt.Find("Preview column")
		if !ok {
			t.Fatal("no preview column")
		}
		return r.H
	}
	if w.settings.MarkdownFontSize != defaultMarkdownFontSize {
		t.Fatalf("size %d, want %d", w.settings.MarkdownFontSize, defaultMarkdownFontSize)
	}
	before := height()
	for range 3 {
		if err := tt.Click("Bigger Text"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if got := w.settings.MarkdownFontSize; got != defaultMarkdownFontSize+3 {
		t.Errorf("size %d after three clicks, want %d", got, defaultMarkdownFontSize+3)
	}
	if after := height(); after <= before {
		t.Errorf("preview %v tall at the bigger size, %v before", after, before)
	}
	if err := tt.Click("Smaller Text"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := w.settings.MarkdownFontSize; got != defaultMarkdownFontSize+2 {
		t.Errorf("size %d after Smaller, want %d", got, defaultMarkdownFontSize+2)
	}
	changeMarkdownFontSize(0)
	if got := height(); got != before {
		t.Errorf("default size is %v tall, want %v", got, before)
	}
	// It stays within its bounds.
	for range 40 {
		changeMarkdownFontSize(1)
	}
	if got := cfg.Get().MarkdownFontSize; got != maxMarkdownFontSize {
		t.Errorf("size %d, want at most %d", got, maxMarkdownFontSize)
	}
}
