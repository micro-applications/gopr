package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

// inboxItemKind is what a row of the inbox's list shows.
type inboxItemKind uint8

const (
	inboxHeader inboxItemKind = iota // a section's header
	inboxPull                        // a pull request
	inboxNote                        // a section's note: none, capped
	inboxError                       // a section's error
)

// inboxItem is a row of the inbox's list.
type inboxItem struct {
	kind    inboxItemKind
	section github.Section
	pull    *github.InboxPull
	text    string
}

// inboxKey names a pull request of the inbox, as acme/api#4.
func inboxKey(p *github.InboxPull) string {
	return fmt.Sprintf("%s#%d", p.Repo, p.Number)
}

// inboxMatches reports whether a pull request passes the inbox's filter:
// its title, author, repository or number holds it.
func inboxMatches(p *github.InboxPull, filter string) bool {
	if filter == "" {
		return true
	}
	hay := strings.ToLower(fmt.Sprintf("%s\n%s\n%s\n#%d", p.Title, p.Author.Login, p.Repo, p.Number))
	return strings.Contains(hay, strings.ToLower(filter))
}

// inboxItems lists the rows of the inbox: each section's header, then its
// pull requests passing the filter, unless collapsed.
func (w *window) inboxItems() []inboxItem {
	in := &w.inbox
	filter := strings.TrimSpace(in.filter)
	var items []inboxItem
	for _, s := range []github.Section{github.Mine, github.Review} {
		items = append(items, inboxItem{kind: inboxHeader, section: s})
		if state.inboxCollapsed(inboxSectionKeys[s]) {
			continue
		}
		sec := &in.sections[s]
		if sec.err != nil {
			items = append(items, inboxItem{kind: inboxError, section: s, text: errorText(sec.err)})
			continue
		}
		shown := 0
		for i := range sec.pulls {
			if p := &sec.pulls[i]; inboxMatches(p, filter) {
				items = append(items, inboxItem{kind: inboxPull, section: s, pull: p})
				shown++
			}
		}
		switch {
		case shown == 0 && filter != "" && len(sec.pulls) > 0:
			items = append(items, inboxItem{kind: inboxNote, section: s, text: "No matching pull requests"})
		case shown == 0:
			items = append(items, inboxItem{kind: inboxNote, section: s, text: "No pull requests"})
		}
		if sec.total > len(sec.pulls) {
			items = append(items, inboxItem{kind: inboxNote, section: s, text: fmt.Sprintf("Showing %d of %s", len(sec.pulls), thousands(sec.total))})
		}
	}
	return items
}

// inboxView is the sidebar's body: the menus of the inbox, its filter and
// its sections.
func (w *window) inboxView(c *ui.Context) {
	in := &w.inbox
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).Gap(4).Padding(0, 10, 6).Children(func() {
			w.inboxOrgMenu(c)
			w.inboxPeriodMenu(c)
			ui.Spacer(c)
			refresh := iconButton(c, iconRefresh, "Refresh (⌘R)")
			if in.loading {
				refresh.Opacity(0.5)
			}
			if refresh.Clicked() {
				w.loadInbox()
			}
		})
		ui.Box(c).Padding(0, 10, 6).Children(func() {
			searchInput(c, &in.filter, "Filter pull requests", &in.filterFocus, &w.typing)
		})
		w.inboxBody(c)
	})
}

// inboxOrgMenu chooses the owners the inbox shows: the viewer's
// organizations, and their own repositories.
func (w *window) inboxOrgMenu(c *ui.Context) {
	in := &w.inbox
	ui.MenuButton(c, "Orgs", func(m *ui.Menu) {
		hidden := state.inboxHidden()
		toggle := func(owner string) {
			if slices.Contains(hidden, owner) {
				hidden = slices.DeleteFunc(hidden, func(h string) bool { return h == owner })
			} else {
				hidden = append(hidden, owner)
			}
			state.setInboxHidden(hidden)
			w.loadInbox()
		}
		if in.login == "" {
			m.Item("Loading…").Disabled(true)
			return
		}
		for _, o := range in.orgs {
			if m.Item(o).Checked(!slices.Contains(hidden, o)).Chosen() {
				toggle(o)
			}
		}
		if len(in.orgs) > 0 {
			m.Separator()
		}
		if m.Item(in.login + " (personal)").Checked(!slices.Contains(hidden, in.login)).Chosen() {
			toggle(in.login)
		}
	}).Height(26).Tooltip("The organizations whose pull requests show")
}

// inboxPeriodMenu chooses how far back the inbox goes, by the pull
// requests' creation.
func (w *window) inboxPeriodMenu(c *ui.Context) {
	days := state.inboxDays()
	ui.MenuButton(c, fmt.Sprintf("%d days", days), func(m *ui.Menu) {
		for _, d := range []int{7, 30} {
			if m.Item(fmt.Sprintf("Last %d days", d)).Checked(days == d).Chosen() && d != days {
				state.setInboxDays(d)
				w.loadInbox()
			}
		}
	}).Height(26).Tooltip("The pull requests opened in this period")
}

// inboxBody shows the sections of the inbox, or why there are none.
func (w *window) inboxBody(c *ui.Context) {
	t := c.Theme()
	in := &w.inbox
	note := func(msg string, actions func()) {
		ui.Column(c).Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
			if actions != nil {
				actions()
			}
		})
	}
	switch {
	case in.signedOut:
		note("Sign in to GitHub to see your pull requests.", nil)
		return
	case in.err != nil:
		note(errorText(in.err), func() {
			if ui.Button(c, "Try Again").Clicked() {
				w.loadInbox()
			}
		})
		return
	case !in.loaded:
		if in.loading {
			note("Loading…", nil)
		} else {
			ui.Column(c).Grow(1)
		}
		return
	case scopeEmpty(w.inboxScope()):
		note("No organizations selected", nil)
		return
	}
	items := w.inboxItems()
	choose := func(p *github.InboxPull) {
		in.sel = inboxKey(p)
		if w.openErr == nil && (w.source == pullSource(p.Repo, p.Number) ||
			w.opening != nil && w.opening.repo == p.Repo && w.opening.number == p.Number) {
			return // shown, or opening
		}
		w.openPull(p.Repo, p.Number)
	}
	in.list.Key = func(i int) any {
		it := items[i]
		if it.kind == inboxPull {
			return inboxKey(it.pull)
		}
		return fmt.Sprintf("%d:%d:%s", it.kind, it.section, it.text)
	}
	in.list.Label = func(i int) string {
		if it := items[i]; it.kind == inboxPull {
			return it.pull.Title
		}
		return ""
	}
	pal := paletteFor(t)
	ui.List(c, &in.list, len(items), func(i int) {
		it := items[i]
		switch it.kind {
		case inboxHeader:
			w.inboxSectionHeader(c, it.section)
		case inboxPull:
			p := it.pull
			key := inboxKey(p)
			current := w.source == pullSource(p.Repo, p.Number) ||
				(w.opening != nil && w.opening.repo == p.Repo && w.opening.number == p.Number)
			selected := key == in.sel && in.list.FocusWithin(c)
			row := pullRow(c, w, pal, &p.Summary, p.Repo.String(), it.section != github.Mine, current, selected)
			if row.Clicked() {
				choose(p)
			}
		case inboxNote:
			ui.Text(c, it.text).FontSize(11).TextColor(t.TextMuted).Padding(4, 10, 8)
		case inboxError:
			ui.Column(c).Gap(6).Padding(4, 10, 8).AlignItems(ui.Start).Children(func() {
				ui.Text(c, it.text).FontSize(11).TextColor(t.Danger).Selectable()
				if ui.Button(c, "Try Again").Clicked() {
					w.loadInbox()
				}
			})
		}
	}).Grow(1).Padding(0, 8, 8).Gap(1).Focusable().FocusRing(false).Label("Pull requests")

	// The arrows move through the pull requests, across the sections.
	var pulls []int
	at := -1
	for i, it := range items {
		if it.kind == inboxPull {
			if inboxKey(it.pull) == in.sel {
				at = len(pulls)
			}
			pulls = append(pulls, i)
		}
	}
	// A move opens the pull request when its repository needs no clone:
	// else Enter opens it, as a click does.
	move := func(j int) {
		if j >= 0 && j < len(pulls) {
			p := items[pulls[j]].pull
			if w.hasClone(p.Repo) {
				choose(p)
			} else {
				in.sel = inboxKey(p)
			}
			in.list.ScrollIntoView(pulls[j])
		}
	}
	if at >= 0 && in.list.Shortcut(c, 0, ui.KeyEnter) {
		choose(items[pulls[at]].pull)
	}
	if in.list.Shortcut(c, 0, ui.KeyDown) {
		move(at + 1)
	}
	if in.list.Shortcut(c, 0, ui.KeyUp) {
		move(max(at-1, 0))
	}
	if in.list.Shortcut(c, 0, ui.KeyHome) {
		move(0)
	}
	if in.list.Shortcut(c, 0, ui.KeyEnd) {
		move(len(pulls) - 1)
	}
}

// hasClone reports whether a repository is known on disk: the window's
// own, or a clone kept in the store.
func (w *window) hasClone(repo github.Repo) bool {
	return (w.gh.ok() && strings.EqualFold(w.gh.repo.String(), repo.String())) || state.clonePath(repo) != ""
}

// inboxSectionHeader is a section's title, which collapses it, and its
// count.
func (w *window) inboxSectionHeader(c *ui.Context, s github.Section) {
	t := c.Theme()
	key := inboxSectionKeys[s]
	collapsed := state.inboxCollapsed(key)
	title := [2]string{github.Mine: "My pull requests", github.Review: "Waiting on my review"}[s]
	sec := &w.inbox.sections[s]
	b := ui.ButtonBase(c).Height(28).Padding(0, 8, 0, 4).Radius(6).Margin(6, 0, 0).FocusRing(false).TextColor(t.TextMuted).Label(title)
	if b.Hovered() {
		b.Background(ui.RGBA(127, 127, 127, 0.08))
	}
	if b.Clicked() {
		collapsed = !collapsed
		state.setInboxCollapsed(key, collapsed)
	}
	b.Children(func() {
		ui.Row(c).Grow(1).Gap(5).Children(func() {
			ic := ui.Icon(c, iconChevronDown).FontSize(12)
			target := float32(0)
			if collapsed {
				target = -90
			}
			ic.Rotate(ic.Animate("rot", target, 150*time.Millisecond))
			ui.Text(c, title).FontSize(12).FontWeight(600).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			if sec.err == nil && w.inbox.loaded {
				ui.Text(c, thousands(sec.total)).FontFeatures("tnum").FontSize(10).FontWeight(500)
			}
		})
	})
}
