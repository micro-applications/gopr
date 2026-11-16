package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/godiff/internal/github"
	"github.com/egoist/mygo/ui"
)

// stackGroups groups pull requests into stacks: a pull request based on
// the head branch of another of the same repository goes with it, unless
// others are based on that branch too: a stack is a chain. Each
// group lists its pull requests from the top of the stack down, as GitHub
// does; groups keep the order of their first pull request, and a pull
// request in no stack is a group of one.
func stackGroups(pulls []*github.InboxPull) [][]*github.InboxPull {
	head := map[string]int{} // acme/api:branch → the pull request with that head
	for i, p := range pulls {
		if !p.CrossRepo && p.HeadRef != "" {
			head[strings.ToLower(p.Repo.String())+":"+p.HeadRef] = i
		}
	}
	parent := make([]int, len(pulls))
	children := make([]int, len(pulls))
	for i, p := range pulls {
		parent[i] = -1
		if j, ok := head[strings.ToLower(p.Repo.String())+":"+p.BaseRef]; ok && j != i {
			parent[i] = j
			children[j]++
		}
	}
	for i, j := range parent {
		if j >= 0 && children[j] > 1 {
			parent[i] = -1
		}
	}
	// depth is how far a pull request is from the bottom of its stack, and
	// root the bottom, both bounded against cycles.
	depth := make([]int, len(pulls))
	root := make([]int, len(pulls))
	for i := range pulls {
		r, d := i, 0
		for parent[r] >= 0 && d < len(pulls) {
			r = parent[r]
			d++
		}
		root[i], depth[i] = r, d
	}
	byRoot := map[int]int{} // root → its group
	var groups [][]int
	for i := range pulls {
		g, ok := byRoot[root[i]]
		if !ok {
			g = len(groups)
			byRoot[root[i]] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], i)
	}
	out := make([][]*github.InboxPull, len(groups))
	for gi, g := range groups {
		slices.SortStableFunc(g, func(a, b int) int { return depth[b] - depth[a] })
		for _, i := range g {
			out[gi] = append(out[gi], pulls[i])
		}
	}
	return out
}

// stackPos is where a row is in a stack of the sidebar, which draws the
// line joining its pull requests.
type stackPos uint8

const (
	stackNone   stackPos = iota // not in a stack
	stackTop                    // the top of a stack
	stackMiddle                 // between the top and the bottom
	stackBottom                 // the bottom, based on the trunk
)

// stackPosOf is the position of the i-th of n pull requests of a stack,
// from the top.
func stackPosOf(i, n int) stackPos {
	switch {
	case n < 2:
		return stackNone
	case i == 0:
		return stackTop
	case i == n-1:
		return stackBottom
	}
	return stackMiddle
}

// loadStack reads the stack of the pull request shown, if it is in one.
func (w *window) loadStack() {
	p := w.pr
	if p == nil || p.meta == nil || p.stackLoading {
		return
	}
	p.stackLoading = true
	meta := p.meta
	w.background(func() {
		client := ghClient()
		var s *github.Stack
		var err error
		if client.Token != "" {
			s, err = client.PullStack(context.Background(), p.repo, meta)
		}
		w.update(func() {
			p.stackLoading = false
			if err == nil {
				p.stack = s
			}
		})
	})
}

// moveInStack opens the pull request dir places up (+1) or down (-1) the
// stack of the one shown.
func (w *window) moveInStack(dir int) {
	p := w.pr
	if p == nil || p.stack == nil {
		return
	}
	i := p.stack.Index(p.number) + dir
	if i < 0 || i >= len(p.stack.Pulls) || p.stack.Index(p.number) < 0 {
		return
	}
	w.openStackPull(p.stack.Pulls[i].Number)
}

// openStackPull opens a pull request of the stack of the one shown, which
// keeps the stack meanwhile so the header's switcher stays.
func (w *window) openStackPull(n int) {
	p := w.pr
	p.stackOpen = false
	if n == p.number {
		return
	}
	if next := w.pullFor(pullSource(p.repo, n)); next.stack == nil {
		next.stack = p.stack
	}
	w.inbox.sel = inboxKey(&github.InboxPull{Summary: github.Summary{Number: n}, Repo: p.repo})
	w.openPull(p.repo, n)
}

// stackRailX is where the line joining a stack's rows runs, from the
// list's left; stackRailTick is how far down a row its branch leaves it,
// beside the pull request's icon.
const (
	stackRailX    = 14
	stackRailTick = 14
)

// stackLabel heads a stack in the sidebar: how many pull requests it
// has, and the branch it goes onto.
func stackLabel(c *ui.Context, n int, trunk string) {
	t := c.Theme()
	ui.Row(c).Gap(6).Padding(6, 8, 3, 8).Children(func() {
		ui.Icon(c, iconStack).FontSize(12).TextColor(t.TextMuted).Shrink(0)
		ui.Textf(c, "Stack of %d", n).FontFeatures("tnum").FontSize(11).FontWeight(600).TextColor(t.TextMuted).Shrink(0)
		if trunk != "" {
			ui.Text(c, "onto "+trunk).FontSize(11).TextColor(t.TextMuted.Alpha(0.8)).SingleLine().Shrink(1).MinWidth(0)
		}
	})
}

// stackRail shows a pull request of a stack: row builds it, indented,
// beside the line that joins it to the label and the rows above and below.
func stackRail(c *ui.Context, pos stackPos, row func()) {
	color := ui.RGBA(127, 127, 127, 0.4)
	ui.Row(c).Padding(0, 0, 0, 20).Draw(func(p *ui.Painter, r ui.Rect) {
		x := r.X + stackRailX
		// The rows sit 1 apart: the line crosses the gap.
		bottom := r.Y + r.H + 1
		if pos == stackBottom {
			bottom = r.Y + stackRailTick
		}
		p.Line(x, r.Y-1, x, bottom, 1.5, color)
		p.Line(x, r.Y+stackRailTick, r.X+20, r.Y+stackRailTick, 1.5, color)
	}).Children(row)
}

// stackSwitcher is the header's button for the stack of the pull request
// shown, as GitHub's: where it is in the stack, from the bottom, and a
// list of the stack to open another from.
func (w *window) stackSwitcher(c *ui.Context, pal *palette) {
	p := w.pr
	s := p.stack
	at := s.Index(p.number)
	if at < 0 {
		return
	}
	t := c.Theme()
	b := ui.ButtonBase(c).Height(26).Padding(0, 10, 0, 8).Radius(13).Shrink(0).
		BorderWidth(1).BorderColor(pal.cardBorder).Label("Stack").
		Tooltip(fmt.Sprintf("Pull request %d of a stack of %d (⇧J, ⇧K)", at+1, len(s.Pulls)))
	if b.Hovered() || p.stackOpen {
		b.Background(ui.RGBA(127, 127, 127, 0.1))
	}
	if b.Clicked() {
		p.stackOpen = !p.stackOpen
	}
	b.Children(func() {
		ui.Row(c).Gap(5).Children(func() {
			ui.Icon(c, iconStack).FontSize(13).TextColor(t.TextMuted)
			ui.Textf(c, "%d/%d", at+1, len(s.Pulls)).FontFeatures("tnum").FontSize(12).FontWeight(600)
		})
	})
	ui.Popover(c, b, &p.stackOpen, func() {
		ui.Column(c).Width(420).Gap(2).Padding(4).Children(func() {
			ui.Row(c).Gap(6).Padding(4, 6, 6).Children(func() {
				ui.Icon(c, iconStack).FontSize(13).TextColor(t.TextMuted)
				ui.Textf(c, "Stack of %d pull requests", len(s.Pulls)).FontSize(12).FontWeight(600)
			})
			// The top of the stack first, the trunk last.
			for i := len(s.Pulls) - 1; i >= 0; i-- {
				sp := s.Pulls[i]
				current := i == at
				row := ui.Row(c).Gap(8).Padding(6, 8).Radius(6).AlignItems(ui.Start).Role(ui.RoleButton).Label(sp.Title)
				switch {
				case current:
					row.Background(ui.RGBA(127, 127, 127, 0.2))
				case row.Hovered():
					row.Background(ui.RGBA(127, 127, 127, 0.08))
				}
				if row.Clicked() {
					w.openStackPull(sp.Number)
				}
				row.Children(func() {
					icon, color := iconPull, pal.addBar
					if sp.Draft {
						icon, color = iconPullDraft, t.TextMuted
					}
					ui.Icon(c, icon).FontSize(14).TextColor(color).Margin(1, 0, 0).Shrink(0)
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						weight := 500
						if current {
							weight = 700
						}
						ui.Text(c, sp.Title).FontSize(12).FontWeight(weight).SingleLine()
						ui.RichText(c,
							ui.Span{Text: fmt.Sprintf("#%d", sp.Number), Color: pal.ref},
							ui.Span{Text: " · ", Color: t.TextMuted},
							ui.Span{Text: sp.HeadRef, Font: w.codeFont(), Color: t.TextMuted},
						).FontSize(11).SingleLine()
					})
				})
			}
			ui.Row(c).Gap(8).Padding(6, 8).Children(func() {
				ui.Icon(c, iconBranch).FontSize(14).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, s.Trunk).Font(w.codeFont()).FontSize(12).TextColor(pal.ref)
			})
			ui.Text(c, "⇧J and ⇧K move down and up the stack").FontSize(11).TextColor(t.TextMuted).
				Padding(8, 8, 4).BorderWidth(1, 0, 0, 0).BorderColor(pal.cardBorder)
		})
	})
}
