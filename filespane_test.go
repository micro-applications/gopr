package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

// inside reports whether box a lies within box b.
func inside(a, b ui.Rect) bool {
	return a.X >= b.X && a.Y >= b.Y && a.X+a.W <= b.X+b.W && a.Y+a.H <= b.Y+b.H
}

func TestFilesPaneShowsTree(t *testing.T) {
	freshState(t)
	clone, base, head := pullClone(t, acmeApp)
	clone = realPath(t, clone)
	(&fakeGitHub{base: base, head: head, comments: `[]`}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: clone})
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	for range 3 {
		tt.Frame()
	}
	shown := func() bool {
		t.Helper()
		pane, ok := tt.Find("Files")
		if !ok {
			return false
		}
		for _, name := range []string{"main.go", "main_test.go"} {
			r, ok := tt.Find(name)
			if !ok || !inside(r, pane) {
				t.Fatalf("%s not in the pane %+v: %+v, %v", name, pane, r, ok)
			}
		}
		return true
	}
	if !shown() {
		t.Fatalf("no files pane in %q", tt.Texts())
	}
	snapshot(t, tt, "files-pane")
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	if shown() || w.filesShown {
		t.Fatal("⌘⇧E did not hide the files pane")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	tt.Frame()
	if !shown() {
		t.Fatal("⌘⇧E did not show the files pane again")
	}
	if _, shownSaved := state.filesLayout(); !shownSaved {
		t.Error("the pane shown was not saved")
	}
}

func TestNoLocalModeUI(t *testing.T) {
	freshState(t)
	w, tt := emptyWindow(t)
	for _, label := range []string{"History (⌘2)", "Files (⌘1)"} {
		if _, ok := tt.Find(label); ok {
			t.Errorf("%q shows", label)
		}
	}
	w.paletteOpen = true
	tt.Frame()
	for _, cmd := range w.commands() {
		if strings.HasPrefix(cmd.title, "Commit") {
			t.Errorf("the command bar lists %q", cmd.title)
		}
	}
	if tt.HasText("Commit…") {
		t.Errorf("the command bar shows Commit… in %q", tt.Texts())
	}
}

func TestOpenPullDialog(t *testing.T) {
	freshState(t)
	var asked []github.Repo
	old := resolveClone
	resolveClone = func(_ context.Context, want github.Repo, _ string, _ func(string)) (string, error) {
		asked = append(asked, want)
		return "", errors.New("no clone")
	}
	t.Cleanup(func() { resolveClone = old })
	w, tt := emptyWindow(t)
	w.hold = true
	w.openDialog(dialogPull)
	tt.Frame()
	tt.Type("acme/app#1")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	drain(w)
	tt.Frame()
	if len(asked) != 1 || asked[0] != acmeApp {
		t.Fatalf("resolveClone asked for %v, want %v", asked, acmeApp)
	}
	if w.dialogOpen {
		t.Error("the dialog stays open")
	}
}

func TestOpenPullDialogNeedsRepo(t *testing.T) {
	freshState(t)
	calls := fakeResolve(t, nil)
	w, tt := emptyWindow(t)
	w.openDialog(dialogPull)
	tt.Frame()
	tt.Type("#3")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if *calls != 0 || !w.dialogOpen || w.dialogErr == "" {
		t.Fatalf("calls %d, open %v, error %q", *calls, w.dialogOpen, w.dialogErr)
	}
}

func TestFilesPaneResizeSaved(t *testing.T) {
	freshState(t)
	w, tt := newTestWindow(t, testRepo(t))
	pane, ok := tt.Find("Files")
	if !ok {
		t.Fatalf("no files pane in %q", tt.Texts())
	}
	x, y := pane.X+pane.W, pane.Y+pane.H/2
	tt.Press(x, y)
	tt.Move(x+20, y)
	tt.Move(x+40, y)
	tt.Release(x+40, y)
	tt.Frame()
	if w.filesWidth <= filesDefault {
		t.Fatalf("width %v after dragging", w.filesWidth)
	}
	if width, _ := state.filesLayout(); width != w.filesWidth {
		t.Errorf("saved width %v, want %v", width, w.filesWidth)
	}
}

func TestFilterClearedForAnotherPull(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	w.filter = "nothing matches this"
	tt.Frame()
	w.openPull(acmeApp, 2)
	tt.Frame()
	if w.filter != "" {
		t.Errorf("filter %q kept for another pull request", w.filter)
	}
}
