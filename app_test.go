package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/godiff/internal/git"
	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

func TestMain(m *testing.M) {
	// Keep the user's settings out of the tests, and the tests' out of them.
	dir, _ := os.MkdirTemp("", "godiff-config")
	cfg.path = filepath.Join(dir, "godiff.jsonc")
	cfg.settings = defaultSettings()
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Ada", "GIT_AUTHOR_EMAIL=ada@example.com", "GIT_COMMITTER_NAME=Ada", "GIT_COMMITTER_EMAIL=ada@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const mainGo = `package main

import "fmt"

func greet(name string) string {
	return "Hello, " + name
}

func main() {
	fmt.Println(greet("world"))
}
`

// testRepo makes a repository with a commit and changes on top of it.
func testRepo(t *testing.T) string {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.name", "Ada")
	gitIn(t, dir, "config", "user.email", "ada@example.com")
	var long strings.Builder
	for i := range 60 {
		long.WriteString("line ")
		long.WriteString(strings.Repeat("x", i%7))
		long.WriteString("\n")
	}
	writeFile(t, dir, "main.go", mainGo)
	writeFile(t, dir, "docs/long.txt", long.String())
	writeFile(t, dir, "old.txt", "bye\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "First commit")

	writeFile(t, dir, "main.go", strings.Replace(mainGo, `"Hello, " + name`, `"Hi, " + name + "!"`, 1))
	writeFile(t, dir, "docs/long.txt", strings.Replace(long.String(), "line xxxxx\n", "line changed\n", 1))
	writeFile(t, dir, "src/new.go", "package src\n\n// New is new.\nfunc New() {}\n")
	os.Remove(filepath.Join(dir, "old.txt"))
	return dir
}

// newTestWindow opens a window's state on a repository, loaded.
func newTestWindow(t *testing.T, dir string) (*window, *ui.Tester) {
	t.Helper()
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := newWindow(repo, source{})
	w.settings = defaultSettings()
	w.sidebarShown, w.sidebarWidth = true, sidebarDefault
	w.filesShown, w.filesWidth = true, filesDefault
	w.load()
	w.loadUser()
	tt := ui.NewTester(w.view, 1280, 860)
	tt.Frame()
	return w, tt
}

// snapshot saves the tester's frame in $GODIFF_SNAPSHOTS, to look at.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("GODIFF_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func TestReview(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	if len(w.files) != 4 {
		t.Fatalf("files: %d", len(w.files))
	}
	for _, f := range w.files {
		if !f.loaded {
			t.Errorf("%s not loaded", f.Path)
		}
	}
	for _, want := range []string{"main.go", "new.go", "old.txt", "long.txt", "Total:"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "review")

	// The gap of unchanged lines shows them.
	if !tt.HasText("51 unmodified lines") {
		t.Fatalf("no gap in %q", tt.Texts())
	}
	if err := tt.Click("51 unmodified lines"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("51 unmodified lines") {
		t.Error("the gap did not expand")
	}

	// Viewed collapses a file.
	f := w.files[0]
	if err := tt.Click("Viewed"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !w.isViewed(f) || !f.collapsed {
		t.Errorf("%s: viewed %v, collapsed %v", f.Path, w.isViewed(f), f.collapsed)
	}
}

func TestUnified(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	if err := tt.Click("Unified"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.split() {
		t.Error("still split")
	}
	snapshot(t, tt, "unified")
}

func TestComments(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	// j chooses the first hunk, Enter comments on it.
	w.focusList = true
	tt.Frame()
	tt.Key(0, ui.KeyJ)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(w.comments) != 1 {
		t.Fatalf("comments: %d", len(w.comments))
	}
	tt.Type("Rename this.")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	snapshot(t, tt, "comment")
	if !w.comments[0].pending() {
		t.Fatalf("comment %+v", w.comments[0])
	}
	md := w.commentsMarkdown()
	if !strings.HasPrefix(md, "# Address these Review Comments\n\n1. **docs/long.txt** (New line ") || !strings.Contains(md, "   Rename this.") || !strings.Contains(md, "   ```diff\n   @@ ") {
		t.Errorf("markdown:\n%s", md)
	}
}

func TestReviewShortcutsFromSidebar(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	if err := tt.Click("old.txt"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Changed files") {
		t.Fatal("the sidebar did not take focus")
	}
	tt.Key(0, ui.KeyJ)
	tt.Key(0, ui.KeyEnter)
	if len(w.comments) != 1 {
		t.Fatalf("review shortcuts from the sidebar: %d comments", len(w.comments))
	}
}

func TestReviewListReturnsAfterEmptyView(t *testing.T) {
	for _, name := range []string{"filter", "load error"} {
		t.Run(name, func(t *testing.T) {
			w, tt := newTestWindow(t, testRepo(t))
			switch name {
			case "filter":
				w.filter = "no-such-file"
				w.buildTree()
				w.rowsDirty = true
			case "load error":
				w.loadErr = errors.New("unable to load changes")
			}
			// A request to focus the review waits until its list returns.
			w.focusList = true
			tt.Frame()
			tt.Frame()
			if _, ok := tt.Find("Changes"); ok || !w.focusList {
				t.Fatal("the hidden review was built or took focus")
			}
			if _, ok := tt.Find("Changed files"); name == "filter" && ok {
				t.Fatal("the empty file tree was built")
			}
			tt.Key(0, ui.KeyJ)
			tt.Key(0, ui.KeyEnter)
			if w.selHunk >= 0 || len(w.comments) != 0 {
				t.Fatal("review shortcuts ran while its list was hidden")
			}

			w.filter, w.loadErr = "", nil
			w.buildTree()
			w.rowsDirty = true
			tt.Frame()
			if !tt.Focused("Changes") || w.focusList {
				t.Fatal("the returning review did not take focus")
			}
			tt.Key(0, ui.KeyJ)
			tt.Key(0, ui.KeyEnter)
			if len(w.comments) != 1 {
				t.Fatalf("review shortcuts did not return: %d comments", len(w.comments))
			}
		})
	}
}

func TestFind(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	w.finding = true
	tt.Frame()
	tt.Type("hi")
	tt.Frame()
	if len(w.matches) != 1 {
		t.Fatalf("matches %+v for %q", w.matches, w.query)
	}
	snapshot(t, tt, "find")
	if w.files[w.matches[0].file].Path != "main.go" {
		t.Errorf("match in %s", w.files[w.matches[0].file].Path)
	}
	// Only the files with a match show.
	if tt.HasText("new.go") {
		t.Error("new.go shows while finding")
	}
}

func TestPalette(t *testing.T) {
	cfg.Update(func(s *Settings) { s.DiffStyle = "split" })
	w, tt := newTestWindow(t, testRepo(t))
	w.paletteOpen = true
	tt.Frame()
	tt.Type("toggle diff layout")
	tt.Frame()
	snapshot(t, tt, "palette")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if w.paletteOpen || cfg.Get().DiffStyle != "unified" {
		t.Errorf("open %v, style %s", w.paletteOpen, cfg.Get().DiffStyle)
	}
	cfg.Update(func(s *Settings) { s.DiffStyle = "split" })
}

func TestTree(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	// Clicking a file in the tree scrolls the review to it.
	box, ok := tt.Find("old.txt")
	if !ok {
		t.Fatal("no old.txt")
	}
	tt.ClickAt(box.X+4, box.Y+box.H/2)
	tt.Frame()
	if w.files[w.current].Path != "old.txt" || w.treeSel != "f:old.txt" {
		t.Fatalf("current %s, chosen %q", w.files[w.current].Path, w.treeSel)
	}
	if !tt.Focused("Changed files") {
		t.Error("the tree did not take the focus")
	}
	// Up goes to the file above.
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if w.files[w.current].Path != "main.go" {
		t.Errorf("after Up: %s", w.files[w.current].Path)
	}
	snapshot(t, tt, "tree")
	// Left on a directory closes it.
	w.treeSel = "d:docs"
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if !w.closedDirs["d:docs"] || len(w.visibleTree()) != 5 {
		t.Errorf("docs not closed: %v", w.visibleTree())
	}
}

func TestDark(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	tt.SetDark(true)
	w.nextHunk(1)
	tt.Frame()
	snapshot(t, tt, "dark")
}

func TestHorizontalScroll(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "main.go", strings.Replace(mainGo, `"Hello, " + name`, `"Hello, " + name + "`+strings.Repeat("long ", 80)+`"`, 1))
	w, tt := newTestWindow(t, dir)
	box, ok := tt.Find("package main")
	if !ok {
		t.Fatal("no main.go")
	}
	tt.Scroll(box.X+40, box.Y+box.H/2, 120, 0)
	tt.Frame()
	if w.hscroll["main.go"] <= 0 {
		t.Errorf("hscroll %v", w.hscroll)
	}
	tt.Scroll(box.X+40, box.Y+box.H/2, 0, 40)
	tt.Frame()
	if w.hscroll["main.go"] <= 0 {
		t.Errorf("vertical scroll changed hscroll %v", w.hscroll)
	}
	snapshot(t, tt, "hscroll")
}

func pngOf(t *testing.T, w, h int, c color.Color) string {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if (x/8+y/8)%2 == 0 {
				img.Set(x, y, c)
			}
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.String()
}

func TestImage(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "logo.png", pngOf(t, 64, 48, color.NRGBA{200, 40, 40, 255}))
	gitIn(t, dir, "add", "logo.png")
	gitIn(t, dir, "commit", "-q", "-m", "Add a logo")
	writeFile(t, dir, "logo.png", pngOf(t, 96, 48, color.NRGBA{40, 90, 220, 255}))
	w, tt := newTestWindow(t, dir)
	var logo *fileState
	for _, f := range w.files {
		if f.Path == "logo.png" {
			logo = f
		}
	}
	if logo == nil || logo.oldImage == nil || logo.newImage == nil {
		t.Fatalf("logo %+v", logo)
	}
	w.revealFile(slices.Index(w.files, logo))
	tt.Frame()
	if !tt.HasText("New · 96×48 · " + formatBytes(logo.newSize)) {
		t.Errorf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "image")
}

func TestParseArgs(t *testing.T) {
	acme := github.Repo{Owner: "acme", Name: "api"}
	for _, tc := range []struct {
		args []string
		want request
	}{
		{nil, request{}},
		{[]string{"acme/api#4"}, request{acme, 4}},
		{[]string{"https://github.com/acme/api/pull/4"}, request{acme, 4}},
		{[]string{"https://github.com/acme/api/pull/4/files"}, request{acme, 4}},
	} {
		req, err := parseArgs(tc.args)
		if err != nil || req != tc.want {
			t.Errorf("%v: got %+v, %v", tc.args, req, err)
		}
	}
	for _, args := range [][]string{{"HEAD~1"}, {"#4"}, {"a", "b"}, {"acme/api#4", "acme/api#5"}, {"--cwd", "/x"}} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("%v: error %v", args, err)
		}
	}
	if _, err := parseArgs([]string{"--help"}); err != errHelp {
		t.Errorf("--help: %v", err)
	}
}

func TestLaunchDropsOtherArgs(t *testing.T) {
	var opened []request
	var said []string
	oldOpen, oldComplain := openWindow, complain
	openWindow = func(req request) { opened = append(opened, req) }
	complain = func(msg string) { said = append(said, msg) }
	t.Cleanup(func() { openWindow, complain = oldOpen, oldComplain })

	// Arguments it does not take open the window, which says why.
	launch([]string{"HEAD~1"})
	if len(opened) != 1 || opened[0] != (request{}) {
		t.Errorf("opened %+v, want the window alone", opened)
	}
	if len(said) != 1 || !strings.Contains(said[0], "owner/repo#123") {
		t.Errorf("said %q", said)
	}

	opened, said = nil, nil
	launch([]string{"acme/api#4"})
	if want := (request{github.Repo{Owner: "acme", Name: "api"}, 4}); len(opened) != 1 || opened[0] != want || len(said) != 0 {
		t.Errorf("opened %+v, said %q", opened, said)
	}
}

func TestCLIName(t *testing.T) {
	if cliName != "gopr" {
		t.Errorf("cliName %q", cliName)
	}
	s := cliScript()
	if !strings.Contains(s, "# Opens GoPR") || strings.Contains(s, "--cwd") {
		t.Errorf("script %q", s)
	}
}

func TestFileIcons(t *testing.T) {
	dir := testRepo(t)
	for _, p := range []string{"web/App.tsx", "web/api.ts", "web/site.css", "package.json", "Dockerfile", ".gitignore", "README.md", "ci.yml", "lib.rs", "tool.py"} {
		writeFile(t, dir, p, "x\n")
	}
	w, tt := newTestWindow(t, dir)
	tt.SetDark(true)
	tt.Frame()
	snapshot(t, tt, "file-icons")
	want := map[string]string{"web/App.tsx": "react", "web/api.ts": "typescript", "Dockerfile": "docker", ".gitignore": "git", "main.go": "go", "docs/long.txt": "text"}
	for _, f := range w.files {
		if token, ok := want[f.Path]; ok && (f.kind == nil || f.kind.Token != token) {
			t.Errorf("%s: icon %+v, want %s", f.Path, f.kind, token)
		}
	}
}
