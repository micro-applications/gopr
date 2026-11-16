package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

func TestStackGroups(t *testing.T) {
	acme, other := github.Repo{Owner: "acme", Name: "app"}, github.Repo{Owner: "acme", Name: "web"}
	pull := func(repo github.Repo, n int, base, head string) *github.InboxPull {
		return &github.InboxPull{Repo: repo, Summary: github.Summary{Number: n, BaseRef: base, HeadRef: head}}
	}
	// Newest first, as the search sorts them.
	pulls := []*github.InboxPull{
		pull(acme, 9, "main", "solo"),
		pull(acme, 3, "b", "c"),
		pull(other, 7, "a", "x"), // another repository's
		pull(acme, 1, "main", "a"),
		pull(acme, 2, "a", "b"),
		// develop has two pull requests based on it: no stack.
		pull(acme, 5, "main", "develop"),
		pull(acme, 6, "develop", "x"),
		pull(acme, 8, "develop", "y"),
	}
	var got []string
	for _, g := range stackGroups(pulls) {
		var ns []int
		for _, p := range g {
			ns = append(ns, p.Number)
		}
		got = append(got, fmt.Sprint(ns))
	}
	if want := "[[9] [3 2 1] [7] [5] [6] [8]]"; fmt.Sprint(got) != want {
		t.Errorf("groups %v, want %s", got, want)
	}
}

func TestStackSwitcher(t *testing.T) {
	freshState(t)
	clone, base, head := pullClone(t, acmeApp)
	clone = realPath(t, clone)
	(&fakeGitHub{base: base, head: head, comments: `[]`}).serve(t)
	fakeResolve(t, map[github.Repo]string{acmeApp: clone})
	w, tt := emptyWindow(t)
	w.inbox.loaded = true
	w.inbox.sections[github.Mine].pulls = []github.InboxPull{
		{Repo: acmeApp, Summary: github.Summary{Number: 2, Title: "Shout the greeting", BaseRef: "greet", HeadRef: "shout", Author: github.User{Login: "bo"}}},
		{Repo: acmeApp, Summary: github.Summary{Number: 9, Title: "Unrelated fix", BaseRef: "main", HeadRef: "fix", Author: github.User{Login: "bo"}}},
		{Repo: acmeApp, Summary: github.Summary{Number: 1, Title: "Greet louder", BaseRef: "main", HeadRef: "greet", Author: github.User{Login: "bo"}}},
	}
	w.openPull(acmeApp, 1)
	for range 3 {
		tt.Frame()
	}
	if w.pr == nil || w.pr.meta == nil {
		t.Fatalf("no pull request shown: %q", tt.Texts())
	}
	if !slices.Contains(tt.Texts(), "Stack of 2") {
		t.Errorf("no stack in the sidebar: %q", tt.Texts())
	}
	// A stacked row opens its pull request, as any row does.
	r, ok := tt.Find("Shout the greeting")
	if !ok {
		t.Fatalf("no stacked row: %q", tt.Texts())
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if w.source != pullSource(acmeApp, 2) && (w.opening == nil || w.opening.number != 2) {
		t.Fatalf("the click opened %+v, shows %v", w.opening, w.source)
	}
	w.openPull(acmeApp, 1)
	for range 3 {
		tt.Frame()
	}
	// #1 in the middle of a stack: on #4, under #2 and #3.
	stack := &github.Stack{Trunk: "main", Pulls: []github.StackPull{
		{Number: 4, Title: "Name the greeter", BaseRef: "main", HeadRef: "name"},
		{Number: 1, Title: "Greet louder", BaseRef: "name", HeadRef: "greet"},
		{Number: 2, Title: "Shout the greeting", BaseRef: "greet", HeadRef: "shout"},
		{Number: 3, Title: "Whisper too", Draft: true, BaseRef: "shout", HeadRef: "whisper"},
	}}
	w.pr.stack = stack
	w.pr.stackOpen = true
	tt.Frame()
	tt.Frame()
	if !slices.Contains(tt.Texts(), "2/4") || !slices.Contains(tt.Texts(), "Whisper too") {
		t.Fatalf("no stack switcher: %q", tt.Texts())
	}
	snapshot(t, tt, "stack")
	w.pr.stackOpen = false
	tt.Frame()
	// ⇧K and ⇧J go up and down the stack, and not through the hunks.
	for _, step := range []struct {
		key  ui.Key
		want int
	}{{ui.KeyK, 2}, {ui.KeyJ, 4}} {
		w.openPull(acmeApp, 1)
		for range 3 {
			tt.Frame()
		}
		w.pr.stack = stack
		tt.Key(ui.Shift, step.key)
		tt.Frame()
		if w.source != pullSource(acmeApp, step.want) && (w.opening == nil || w.opening.number != step.want) {
			t.Fatalf("⇧%v opened %+v, shows %v", step.key, w.opening, w.source)
		}
		if w.selHunk >= 0 {
			t.Errorf("⇧%v chose hunk %d", step.key, w.selHunk)
		}
		if next := w.prs[pullSource(acmeApp, step.want)]; next == nil || next.stack == nil {
			t.Errorf("#%d did not keep the stack", step.want)
		}
	}
	// Where J does choose a hunk: the checks above meant something.
	w.openPull(acmeApp, 1)
	for range 3 {
		tt.Frame()
	}
	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if w.selHunk < 0 {
		t.Error("J chose no hunk")
	}
}
