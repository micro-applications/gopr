package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

var (
	acmeApp = github.Repo{Owner: "acme", Name: "app"}
	acmeWeb = github.Repo{Owner: "acme", Name: "web"}
)

// fakeResolve makes resolveClone find the clones given, by repository, and
// counts its calls.
func fakeResolve(t *testing.T, clones map[github.Repo]string) *int {
	t.Helper()
	calls := new(int)
	old := resolveClone
	resolveClone = func(_ context.Context, want github.Repo, _ string, _ func(string)) (string, error) {
		*calls++
		if dir, ok := clones[want]; ok {
			return dir, nil
		}
		return "", errors.New("no clone of " + want.String())
	}
	t.Cleanup(func() { resolveClone = old })
	return calls
}

// realPath is a path as git names it, its links resolved: macOS's
// temporary directories are under a link.
func realPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// emptyWindow is a window on no repository, in a tester.
func emptyWindow(t *testing.T) (*window, *ui.Tester) {
	t.Helper()
	withSettings(t, func(s *Settings) { s.GithubToken = "t" })
	w := newWindow(nil, source{})
	w.sidebarShown, w.sidebarWidth = true, sidebarDefault
	tt := ui.NewTester(w.view, 1280, 860)
	tt.Frame()
	return w, tt
}

// drain runs the background work held, and what it holds in turn.
func drain(w *window) {
	for len(w.held) > 0 {
		fn := w.held[0]
		w.held = w.held[1:]
		fn()
	}
}

func TestEmptyWorkspace(t *testing.T) {
	freshState(t)
	_, tt := emptyWindow(t)
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("Choose a pull request") {
		t.Fatalf("no empty state in %q", tt.Texts())
	}
}

func TestOpenPullBindsRepo(t *testing.T) {
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
	if w.repo == nil || w.repo.Root != clone {
		t.Fatalf("repo %+v, want %s", w.repo, clone)
	}
	if want := pullSource(acmeApp, 1); w.source != want || w.loadErr != nil || w.openErr != nil {
		t.Fatalf("source %+v, load error %v, open error %v", w.source, w.loadErr, w.openErr)
	}
	if !tt.HasText("Greet louder") {
		t.Errorf("no title in %q", tt.Texts())
	}
	if !slices.Contains(state.recent(), clone) {
		t.Errorf("recent %v lacks %s", state.recent(), clone)
	}
}

func TestOpenPullLastChoiceWins(t *testing.T) {
	freshState(t)
	appClone, web, _, _ := twoRepos(t)
	w, tt := emptyWindow(t)
	w.hold = true
	w.openPull(acmeApp, 1)
	w.openPull(acmeWeb, 1)
	tt.Frame()
	if !tt.HasText("Finding acme/web…") {
		t.Errorf("no status in %q", tt.Texts())
	}
	if len(w.held) != 2 {
		t.Fatalf("%d held, want the two resolves", len(w.held))
	}
	// acme/web, chosen last, is found first: acme/app, found after, must
	// not take its place.
	resolveApp, resolveWeb := w.held[0], w.held[1]
	w.held = nil
	resolveWeb()
	drain(w)
	resolveApp()
	drain(w)
	tt.Frame()
	if want := pullSource(acmeWeb, 1); w.source != want {
		t.Fatalf("source %+v, want %+v", w.source, want)
	}
	if w.repo == nil || w.repo.Root != web || w.repo.Root == appClone {
		t.Fatalf("repo %+v, want %s", w.repo, web)
	}
}

// webClone is pullClone for acme/web, whose pull request adds web.go to
// main: it shares no commit with acme/app's beyond main.
func webClone(t *testing.T) (server, clone, base, head string) {
	t.Helper()
	server, clone, base, _ = pullServer(t)
	gitIn(t, server, "checkout", "-q", "-b", "change", "main")
	writeFile(t, server, "web.go", "package web\n")
	gitIn(t, server, "add", ".")
	gitIn(t, server, "commit", "-q", "-m", "Add web")
	head = strings.TrimSpace(gitIn(t, server, "rev-parse", "HEAD"))
	gitIn(t, server, "update-ref", "refs/pull/1/head", head)
	gitIn(t, server, "checkout", "-q", "main")
	gitIn(t, server, "branch", "-D", "change")
	gitIn(t, clone, "remote", "set-url", "origin", "https://github.com/acme/web.git")
	return server, clone, base, head
}

// twoRepos makes clones of acme/app and acme/web, whose #1 differ, with
// GitHub and resolveClone faked for both. acme/app's remote is upstream,
// which acme/web's clone lacks: a fetch of it there would fall back to
// acme/app's URL, and get its commits.
func twoRepos(t *testing.T) (appClone, web, appHead, webHead string) {
	t.Helper()
	var appServer, webServer, appBase, webBase string
	appServer, appClone, appBase, appHead = pullServer(t)
	gitIn(t, appClone, "remote", "rename", "origin", "upstream")
	gitIn(t, appClone, "remote", "set-url", "upstream", "https://github.com/acme/app.git")
	webServer, web, webBase, webHead = webClone(t)
	appClone, web = realPath(t, appClone), realPath(t, web)
	if appHead == webHead {
		t.Fatal("the pull requests have the same head")
	}
	fakeGitHubGits(t, map[github.Repo]string{acmeApp: appServer, acmeWeb: webServer})
	(&fakeGitHub{base: appBase, head: appHead, comments: `[]`, others: map[string]fakePull{
		"/repos/acme/web/pulls/1": {title: "Add web", base: webBase, head: webHead},
	}}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: appClone, acmeWeb: web})
	return appClone, web, appHead, webHead
}

func TestSwitchRepoDuringFetch(t *testing.T) {
	freshState(t)
	appClone, web, appHead, _ := twoRepos(t)
	w, tt := emptyWindow(t)
	w.hold = true

	// acme/app#1 is found, and its load, which fetches, waits.
	w.openPull(acmeApp, 1)
	if len(w.held) != 1 {
		t.Fatalf("%d held, want the resolve alone", len(w.held))
	}
	resolveApp := w.held[0]
	w.held = nil
	resolveApp()
	if w.repo == nil || w.repo.Root != appClone {
		t.Fatalf("not bound to acme/app: %+v", w.repo)
	}
	appWork := w.held
	w.held = nil

	// acme/web#1 is chosen meanwhile, and found before acme/app's load
	// runs.
	w.openPull(acmeWeb, 1)
	drain(w)
	w.held = appWork
	drain(w)
	tt.Frame()

	if w.repo.Root != web {
		t.Fatalf("repo %s, want %s", w.repo.Root, web)
	}
	if want := pullSource(acmeWeb, 1); w.source != want || w.loadErr != nil {
		t.Fatalf("source %+v, error %v", w.source, w.loadErr)
	}
	var paths []string
	for _, f := range w.files {
		paths = append(paths, f.Path)
	}
	if !slices.Equal(paths, []string{"web.go"}) {
		t.Errorf("files %v, want web's", paths)
	}
	if exec.Command("git", "-C", web, "rev-parse", "-q", "--verify", appHead+"^{commit}").Run() == nil {
		t.Error("acme/app's commits were fetched into acme/web's clone")
	}
}

func TestOpenPullError(t *testing.T) {
	freshState(t)
	calls := new(int)
	old := resolveClone
	resolveClone = func(context.Context, github.Repo, string, func(string)) (string, error) {
		*calls++
		return "", errors.New("boom")
	}
	t.Cleanup(func() { resolveClone = old })
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	tt.Frame()
	for _, want := range []string{"Unable to open the pull request", "boom"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Try Again"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if *calls != 2 {
		t.Errorf("resolveClone called %d times, want 2", *calls)
	}
	if w.repo != nil {
		t.Errorf("bound to %s", w.repo.Root)
	}
}

func TestOpenPullStatusOutlastsOldLoad(t *testing.T) {
	freshState(t)
	appClone, web, _, _ := twoRepos(t)
	w, tt := emptyWindow(t)
	w.hold = true

	// acme/app#1 shows, its load held as it fetches.
	w.openPull(acmeApp, 1)
	resolveApp := w.held[0]
	w.held = nil
	resolveApp()
	if w.repo == nil || w.repo.Root != appClone {
		t.Fatalf("not bound to acme/app: %+v", w.repo)
	}
	appWork := w.held
	w.held = nil

	// acme/web is cloned meanwhile; acme/app's load goes on, and ends,
	// while it clones.
	old := resolveClone
	resolveClone = func(_ context.Context, want github.Repo, _ string, status func(string)) (string, error) {
		status("Cloning acme/web…")
		w.held = appWork
		drain(w)
		tt.Frame()
		if !tt.HasText("Cloning acme/web…") {
			t.Errorf("no clone status in %q", tt.Texts())
		}
		return web, nil
	}
	t.Cleanup(func() { resolveClone = old })
	w.openPull(acmeWeb, 1)
	drain(w)
	tt.Frame()
	if w.repo.Root != web || w.source != pullSource(acmeWeb, 1) {
		t.Fatalf("repo %s, source %+v", w.repo.Root, w.source)
	}
}

func TestOpenPullAgainAfterLoadFailed(t *testing.T) {
	freshState(t)
	clone, base, head := pullClone(t, acmeApp)
	(&fakeGitHub{base: base, head: head, comments: `[]`}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: realPath(t, clone)})
	w, tt := emptyWindow(t)
	w.openPull(acmeApp, 1)
	tt.Frame()
	// The load failed, as when offline; the pull request is opened again.
	w.loadErr = errors.New("offline")
	w.openPull(acmeApp, 1)
	tt.Frame()
	if w.loadErr != nil || len(w.files) == 0 {
		t.Errorf("not loaded again: error %v, %d files", w.loadErr, len(w.files))
	}
}
