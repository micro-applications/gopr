package main

import (
	"maps"
	"slices"
	"testing"

	"github.com/egoist/godiff/internal/diff"
	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

func TestChangedNewLines(t *testing.T) {
	f := &diff.File{Hunks: []diff.Hunk{{
		OldStart: 5, OldLines: 2, NewStart: 5, NewLines: 4,
		Lines: []diff.Line{
			{Kind: diff.Context, Old: 5, New: 5, Text: "a"},
			{Kind: diff.Add, New: 6, Text: "b"},
			{Kind: diff.Add, New: 7, Text: "c"},
			{Kind: diff.Context, Old: 6, New: 8, Text: "d"},
		},
	}}}
	got := changedNewLines(f)
	if want := map[int]bool{6: true, 7: true}; !maps.Equal(got, want) {
		t.Errorf("changedNewLines = %v, want %v", got, want)
	}
}

func TestFindLines(t *testing.T) {
	got := findLines([]string{"Foo", "bar", "FOO bar"}, "foo")
	if want := []int{0, 2}; !slices.Equal(got, want) {
		t.Errorf("findLines = %v, want %v", got, want)
	}
}

// inFull is a line of main.go that Changes does not show while the
// files are collapsed. (The pull request's diff of main.go shows all of
// its lines: its gaps are too small to hide.)
const inFull = "package main"

// openedPull opens acme/app#1, whose main.go changes, in a tester.
func openedPull(t *testing.T) (*window, *ui.Tester) {
	t.Helper()
	freshState(t)
	clone, base, head := pullClone(t, acmeApp)
	(&fakeGitHub{base: base, head: head, comments: `[]`}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: realPath(t, clone)})
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	for range 3 {
		tt.Frame()
	}
	if w.loadErr != nil || w.openErr != nil || len(w.files) == 0 {
		t.Fatalf("not opened: load error %v, open error %v, %d files", w.loadErr, w.openErr, len(w.files))
	}
	return w, tt
}

func TestFileTab(t *testing.T) {
	w, tt := openedPull(t)
	w.setAllCollapsed(true)
	tt.Frame()
	if tt.HasText(inFull) {
		t.Fatalf("Changes shows %q with the files collapsed", inFull)
	}
	if err := tt.Click("View Full File"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(w.fileTabs) != 1 || w.fileTabs[0].path != "main.go" || w.activeTab != 1 {
		t.Fatalf("tabs %+v, active %d; want main.go active", w.fileTabs, w.activeTab)
	}
	for _, want := range []string{inFull, `fmt.Println(greet("world"))`} {
		if !tt.HasText(want) {
			t.Errorf("the full file lacks %q: %q", want, tt.Texts())
		}
	}

	tt.Key(ui.Cmd, ui.KeyW)
	tt.Frame()
	if len(w.fileTabs) != 0 || w.activeTab != 0 {
		t.Fatalf("after ⌘W: tabs %+v, active %d", w.fileTabs, w.activeTab)
	}
	if tt.HasText(inFull) {
		t.Error("the full file still shows after ⌘W")
	}

	// Opening it again reuses its tab; another pull request has none.
	w.openFileTab("main.go", 0)
	w.openFileTab("main.go", 6)
	tt.Frame()
	if len(w.fileTabs) != 1 || w.activeTab != 1 {
		t.Fatalf("tabs %+v, active %d; want one", w.fileTabs, w.activeTab)
	}
	w.openPull(acmeApp, 2)
	tt.Frame()
	if len(w.fileTabs) != 0 || w.activeTab != 0 {
		t.Errorf("tabs %+v, active %d after opening another pull request", w.fileTabs, w.activeTab)
	}
}

func TestCloseFileTab(t *testing.T) {
	w := &window{fileTabs: []fileTab{{path: "a"}, {path: "b"}, {path: "c"}}}
	w.activeTab = 2
	w.closeFileTab(2)
	if w.activeTab != 1 || len(w.fileTabs) != 2 || w.fileTabs[1].path != "c" {
		t.Errorf("closing the active tab: active %d, tabs %+v", w.activeTab, w.fileTabs)
	}
	// Closing a tab left of the active one keeps the active one.
	w.activeTab = 2
	w.closeFileTab(1)
	if w.activeTab != 1 || w.fileTabs[0].path != "c" {
		t.Errorf("closing a tab on the left: active %d, tabs %+v", w.activeTab, w.fileTabs)
	}
}

func TestFileTabFromHunk(t *testing.T) {
	w, tt := openedPull(t)
	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if w.selHunk < 0 || w.files[w.selFile].Path != "main.go" {
		t.Fatalf("J chose file %d hunk %d", w.selFile, w.selHunk)
	}
	tt.Key(0, ui.KeyF)
	tt.Frame()
	if w.activeTab != 1 || w.fileTabs[0].path != "main.go" {
		t.Fatalf("tabs %+v, active %d; want main.go", w.fileTabs, w.activeTab)
	}
	if !tt.HasText(`"Hi, "`) {
		t.Errorf("the changed line does not show: %q", tt.Texts())
	}

	// ⌘F finds in the file shown.
	*w.findOpen() = true
	w.fileTabs[0].query = "GREET"
	tt.Frame()
	if w.finding {
		t.Error("the diff's find bar opened")
	}
	if got := w.fileTabs[0].found; !slices.Equal(got, []int{4, 9}) {
		t.Errorf("found lines %v, want [4 9]", got)
	}
}

func TestFileTabNotForDeleted(t *testing.T) {
	w, tt := newTestWindow(t, testRepo(t))
	tt.Frame()
	// old.txt's card alone shows, without the files pane, whose row has
	// the same name.
	w.filesShown = false
	w.filter = "old.txt"
	w.buildTree()
	w.rowsDirty = true
	tt.Frame()
	if !tt.HasText("old.txt") {
		t.Fatalf("no old.txt in %q", tt.Texts())
	}
	if _, ok := tt.Find("View Full File"); ok {
		t.Error("the deleted file's header offers View Full File")
	}
	if err := tt.RightClick("old.txt"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); len(menu) == 0 || slices.Contains(menu, "View Full File") {
		t.Errorf("the header's menu %q", menu)
	}
	tt.CloseMenu()

	// Nor does its row in the files pane.
	w.filesShown = true
	tt.Frame()
	if err := tt.RightClick("old.txt"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); slices.Contains(menu, "View Full File") {
		t.Errorf("the tree row's menu %q", menu)
	}
	tt.CloseMenu()

	// A file that is there has it, in its header and its row.
	w.filter = "main.go"
	w.buildTree()
	w.rowsDirty = true
	tt.Frame()
	if _, ok := tt.Find("View Full File"); !ok {
		t.Error("main.go's header lacks View Full File")
	}
	if err := tt.RightClick("main.go"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "View Full File") {
		t.Errorf("main.go's tree row menu %q", menu)
	}
}
