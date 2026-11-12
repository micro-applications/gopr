package main

import (
	"context"
	"slices"
	"time"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

// inboxState is the sidebar's list of pull requests: the viewer's own,
// then those waiting on their review, across their organizations.
type inboxState struct {
	login    string   // the viewer's, "" until known
	orgs     []string // the viewer's organizations
	sections [2]inboxSection
	loading  bool
	loaded   bool // a load finished
	// signedOut is set when no token was found.
	signedOut bool
	// err is why the viewer's organizations could not be read.
	err      error
	loadedAt time.Time
	list     ui.ListState
	filter   string
	// gen counts the loads, to drop the results of older ones.
	gen int
	// sel is the key of the row chosen, as acme/api#4.
	sel         string
	filterFocus bool
}

// inboxSection is a section of the inbox as GitHub's search returned it.
type inboxSection struct {
	pulls []github.InboxPull
	total int // the matches, which may be more than pulls
	err   error
}

// inboxSectionKeys are the names the store keeps the sections by.
var inboxSectionKeys = [2]string{github.Mine: "mine", github.Review: "review"}

// inboxStaleAfter is how old the inbox gets before the window's focus
// loads it again.
const inboxStaleAfter = 5 * time.Minute

// inboxScope is the owners the inbox shows, as the org menu has them.
func (w *window) inboxScope() github.Scope {
	return inboxScopeOf(w.inbox.login, w.inbox.orgs, state.inboxHidden())
}

// inboxScopeOf is the scope of the inbox: all owners when none is hidden,
// else the organizations not hidden, and the viewer's own repositories
// unless their login is.
func inboxScopeOf(login string, orgs, hidden []string) github.Scope {
	if len(hidden) == 0 {
		return github.Scope{All: true}
	}
	var s github.Scope
	for _, o := range orgs {
		if !slices.Contains(hidden, o) {
			s.Orgs = append(s.Orgs, o)
		}
	}
	if login != "" && !slices.Contains(hidden, login) {
		s.User = login
	}
	return s
}

// scopeEmpty reports whether a scope selects no owner.
func scopeEmpty(s github.Scope) bool {
	return !s.All && len(s.Orgs) == 0 && s.User == ""
}

// loadInbox reads the inbox from GitHub: the viewer's organizations the
// first time, then a search for each section. A load started later wins.
func (w *window) loadInbox() {
	in := &w.inbox
	in.gen++
	gen := in.gen
	in.loading = true
	if in.signedOut {
		// The user may have signed in since.
		forgetGitHubToken()
	}
	days := state.inboxDays()
	hidden := state.inboxHidden()
	login, orgs := in.login, slices.Clone(in.orgs)
	w.background(func() {
		var (
			signedOut bool
			err       error
			sections  [2]inboxSection
		)
		tok := githubToken()
		if tok == "" {
			signedOut = true
		} else {
			client := &github.Client{Token: tok}
			ctx := context.Background()
			if login == "" {
				login, orgs, err = client.ViewerOrgs(ctx)
			}
			if err == nil {
				scope := inboxScopeOf(login, orgs, hidden)
				since := time.Now().AddDate(0, 0, -days)
				for _, s := range []github.Section{github.Mine, github.Review} {
					q := github.InboxQuery(s, scope, since)
					if q == "" {
						continue
					}
					sec := &sections[s]
					sec.pulls, sec.total, sec.err = client.SearchPulls(ctx, q)
				}
			}
		}
		w.update(func() {
			if gen != in.gen {
				return
			}
			in.loading = false
			in.loaded = true
			in.loadedAt = time.Now()
			in.signedOut = signedOut
			in.err = err
			if err == nil && !signedOut {
				in.login, in.orgs = login, orgs
				in.sections = sections
			}
		})
	})
}

// refreshInboxIfStale loads the inbox again when it is old, or was
// signed out, as the window takes the focus.
func (w *window) refreshInboxIfStale() {
	in := &w.inbox
	if in.loading {
		return
	}
	if !in.loaded || in.signedOut || time.Since(in.loadedAt) > inboxStaleAfter {
		w.loadInbox()
	}
}

// refreshAll loads the inbox and the pull request shown again.
func (w *window) refreshAll() {
	w.loadInbox()
	w.refresh()
}
