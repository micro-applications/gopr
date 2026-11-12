package main

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

// resetToken forgets the token githubToken found, now and after the test,
// so that none passes from a test to another.
func resetToken(t *testing.T) {
	t.Helper()
	forgetGitHubToken()
	t.Cleanup(forgetGitHubToken)
}

// inboxWindow is a window on no repository whose inbox has loaded from f.
func inboxWindow(t *testing.T, f *fakeGitHub) (*window, *ui.Tester) {
	t.Helper()
	resetToken(t)
	f.serve(t)
	w, tt := emptyWindow(t)
	w.loadInbox()
	tt.Frame()
	return w, tt
}

// recordResolve makes resolveClone record the repositories asked for, and
// fail.
func recordResolve(t *testing.T) *[]github.Repo {
	t.Helper()
	var asked []github.Repo
	old := resolveClone
	resolveClone = func(_ context.Context, want github.Repo, _ string, _ func(string)) (string, error) {
		asked = append(asked, want)
		return "", context.Canceled
	}
	t.Cleanup(func() { resolveClone = old })
	return &asked
}

func twoSections() *fakeGitHub {
	return &fakeGitHub{
		mine:   []fakeInboxPull{{"acme/api", 4, "Add limiter"}},
		review: []fakeInboxPull{{"acme/web", 9, "Fix login"}},
	}
}

func TestInboxSections(t *testing.T) {
	freshState(t)
	f := twoSections()
	f.mineTotal = 150
	_, tt := inboxWindow(t, f)
	snapshot(t, tt, "inbox")
	for _, want := range []string{"My pull requests", "Waiting on my review", "Add limiter", "Fix login", "acme/api"} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	mine, _ := tt.Find("Add limiter")
	review, _ := tt.Find("Fix login")
	if mine.Y >= review.Y {
		t.Errorf("mine at %v, review at %v", mine.Y, review.Y)
	}
}

func TestInboxOpensPull(t *testing.T) {
	freshState(t)
	asked := recordResolve(t)
	_, tt := inboxWindow(t, twoSections())
	if err := tt.Click("Add limiter"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := (github.Repo{Owner: "acme", Name: "api"}); !slices.Equal(*asked, []github.Repo{want}) {
		t.Fatalf("resolved %v, want %v", *asked, want)
	}
}

func TestInboxCollapse(t *testing.T) {
	freshState(t)
	_, tt := inboxWindow(t, twoSections())
	if err := tt.Click("My pull requests"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Add limiter") || !tt.HasText("Fix login") {
		t.Errorf("collapsed mine: %q", tt.Texts())
	}
	if !state.inboxCollapsed("mine") {
		t.Error("the collapse was not saved")
	}
}

func TestInboxPeriod(t *testing.T) {
	freshState(t)
	f := twoSections()
	_, tt := inboxWindow(t, f)
	if err := tt.Click("7 days"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Last 30 days"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	searches := f.searches()
	want := "created:>=" + time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	if len(searches) == 0 || !strings.Contains(searches[len(searches)-1], want) {
		t.Fatalf("searches %q, want the last with %q", searches, want)
	}
	if state.inboxDays() != 30 {
		t.Errorf("days %d", state.inboxDays())
	}
	if !tt.HasText("30 days") {
		t.Errorf("no period on the button in %q", tt.Texts())
	}
}

func TestInboxOrgs(t *testing.T) {
	freshState(t)
	f := twoSections()
	f.orgs = []string{"acme", "zeta"}
	_, tt := inboxWindow(t, f)
	if err := tt.Click("Orgs"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"acme", "zeta", "-", "ada (personal)"}; !slices.Equal(got, want) {
		t.Fatalf("menu %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("zeta"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	searches := f.searches()
	last := searches[len(searches)-1]
	if !strings.Contains(last, "org:acme user:ada") || strings.Contains(last, "zeta") {
		t.Fatalf("last search %q", last)
	}
	if !slices.Equal(state.inboxHidden(), []string{"zeta"}) {
		t.Errorf("hidden %q", state.inboxHidden())
	}
}

func TestInboxNoOrgs(t *testing.T) {
	freshState(t)
	f := twoSections()
	f.orgs = []string{"acme"}
	_, tt := inboxWindow(t, f)
	var before int
	for _, item := range []string{"acme", "ada (personal)"} {
		before = len(f.searches())
		if err := tt.Click("Orgs"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if n := len(f.searches()); n != before {
		t.Errorf("%d searches after unchecking all: %q", n-before, f.searches()[before:])
	}
	if !tt.HasText("No organizations selected") || tt.HasText("Add limiter") {
		t.Errorf("unchecked all: %q", tt.Texts())
	}
}

func TestInboxCapped(t *testing.T) {
	freshState(t)
	f := twoSections()
	f.mineTotal = 150
	_, tt := inboxWindow(t, f)
	if !tt.HasText("Showing 1 of 150") {
		t.Errorf("no cap in %q", tt.Texts())
	}
}

func TestInboxSignedOut(t *testing.T) {
	freshState(t)
	resetToken(t)
	f := twoSections()
	f.serve(t)
	t.Setenv("GODIFF_NO_SHELL_PATH", "1")
	t.Setenv("PATH", t.TempDir()) // no gh
	for _, k := range []string{"GODIFF_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		t.Setenv(k, "")
	}
	w, tt := emptyWindow(t)
	withSettings(t, func(s *Settings) { s.GithubToken = "" })
	w.settings = cfg.Get()
	w.loadInbox()
	tt.Frame()
	if !tt.HasText("Sign in to GitHub") || !tt.HasText("gh auth login") {
		t.Errorf("no sign in in %q", tt.Texts())
	}
	if !w.inbox.signedOut {
		t.Error("not signed out")
	}
	if r := f.requests(); len(r) != 0 {
		t.Errorf("requests %q", r)
	}
}

func TestInboxError(t *testing.T) {
	freshState(t)
	f := twoSections()
	f.status = 401
	w, tt := inboxWindow(t, f)
	if w.inbox.err == nil || !tt.HasText(errorText(w.inbox.err)) {
		t.Fatalf("error %v in %q", w.inbox.err, tt.Texts())
	}
	try, ok := tt.Find("Try Again")
	if !ok || try.X > w.sidebarWidth {
		t.Errorf("no Try Again in the inbox: %v %v", try, ok)
	}
}

func TestInboxKeyboard(t *testing.T) {
	web := github.Repo{Owner: "acme", Name: "web"}
	for _, cloned := range []bool{false, true} {
		t.Run(map[bool]string{false: "not cloned", true: "cloned"}[cloned], func(t *testing.T) {
			freshState(t)
			if cloned {
				state.setClonePath(web, t.TempDir())
			}
			asked := recordResolve(t)
			w, tt := inboxWindow(t, twoSections())
			if err := tt.Click("Add limiter"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			*asked = nil
			// Down crosses into the next section.
			tt.Key(0, ui.KeyDown)
			tt.Frame()
			if w.inbox.sel != "acme/web#9" {
				t.Fatalf("chosen %q", w.inbox.sel)
			}
			if cloned {
				if !slices.Equal(*asked, []github.Repo{web}) {
					t.Fatalf("resolved %v, want %v", *asked, web)
				}
				return
			}
			if len(*asked) != 0 {
				t.Fatalf("down resolved %v without a clone", *asked)
			}
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			if !slices.Equal(*asked, []github.Repo{web}) {
				t.Fatalf("Enter resolved %v, want %v", *asked, web)
			}
		})
	}
}

func TestInboxClickWhileOpening(t *testing.T) {
	freshState(t)
	asked := recordResolve(t)
	w, tt := inboxWindow(t, twoSections())
	w.hold = true
	for range 2 {
		if err := tt.Click("Add limiter"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	drain(w)
	if len(*asked) != 1 {
		t.Fatalf("resolved %v, want one", *asked)
	}
}

func TestInboxDropsStaleLoad(t *testing.T) {
	freshState(t)
	f := twoSections()
	resetToken(t)
	f.serve(t)
	w, tt := emptyWindow(t)
	w.hold = true
	w.loadInbox()
	w.loadInbox()
	if len(w.held) != 2 {
		t.Fatalf("%d held, want the two loads", len(w.held))
	}
	older, newer := w.held[0], w.held[1]
	w.held = nil
	newer()
	f.mu.Lock()
	f.mine = []fakeInboxPull{{"acme/api", 5, "Old limiter"}}
	f.mu.Unlock()
	older()
	tt.Frame()
	if !tt.HasText("Add limiter") || tt.HasText("Old limiter") {
		t.Errorf("the older load won: %q", tt.Texts())
	}
}
