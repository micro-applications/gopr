package main

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"

	"github.com/egoist/godiff/internal/diff"
	"github.com/egoist/godiff/internal/highlight"
	"github.com/egoist/mygo/ui"
)

// fileTab is a tab showing a whole file at the head of what is reviewed,
// with the find bar of its own.
type fileTab struct {
	path    string
	list    ui.ListState
	finding bool
	query   string
	match   int

	// The lines matching query, found in the lines of file.
	found      []int
	foundFor   string
	foundLines *fileState
}

// fileByPath returns the changed file at path, nil for none.
func (w *window) fileByPath(path string) *fileState {
	for _, f := range w.files {
		if f.Path == path {
			return f
		}
	}
	return nil
}

// canViewFull reports whether a file has a whole text to show at the head:
// deleted files, directories, binaries and pictures have none.
func canViewFull(f *fileState) bool {
	return f.Status != diff.Deleted && !f.Binary && !f.Directory && !isImage(f.Path)
}

// openFileTab shows the whole file at path in its tab, opening one for it
// unless it has one, scrolled to line (1-based; 0 is the top).
func (w *window) openFileTab(path string, line int) {
	i := slices.IndexFunc(w.fileTabs, func(t fileTab) bool { return t.path == path })
	if i < 0 {
		w.fileTabs = append(w.fileTabs, fileTab{path: path})
		i = len(w.fileTabs) - 1
	}
	w.activeTab = i + 1
	// A line to show is in the source of a Markdown file.
	if line > 0 && isMarkdown(path) {
		if w.mdSource == nil {
			w.mdSource = map[string]bool{}
		}
		w.mdSource[path] = true
	}
	// The list keeps the request until it is built, once the lines load.
	if line > 0 {
		w.fileTabs[i].list.ScrollTo(line-1, ui.Center)
	} else {
		w.fileTabs[i].list.ScrollTo(0, ui.Start)
	}
	if f := w.fileByPath(path); f != nil && !f.loaded && !w.loading {
		w.loadFile(f)
	}
}

// closeFileTab closes tab i (1 or more): the active tab falls to the one
// on its left.
func (w *window) closeFileTab(i int) {
	if i < 1 || i > len(w.fileTabs) {
		return
	}
	w.fileTabs = slices.Delete(w.fileTabs, i-1, i)
	if w.activeTab >= i {
		w.activeTab--
	}
}

// findOpen is the find bar of the tab shown: the diff's, or the file's.
func (w *window) findOpen() *bool {
	if w.activeTab > 0 && w.activeTab <= len(w.fileTabs) {
		return &w.fileTabs[w.activeTab-1].finding
	}
	return &w.finding
}

// loadFile reads a file's contents ahead of the others, which load in
// turn. The revisions are those of the last load, which is done.
func (w *window) loadFile(f *fileState) {
	repo, gen, oldRev, newRev := w.repo, w.gen, w.oldRev, w.newRev
	if repo == nil {
		return
	}
	w.background(func() {
		contents, err := repo.NewContents()
		if err != nil {
			return
		}
		defer contents.Close()
		l := read(repo, contents, f.File, oldRev, newRev)
		l.finish()
		w.update(func() {
			if gen != w.gen || f.loaded {
				return
			}
			l.file = f
			l.apply()
			w.rowsDirty = true
		})
	})
}

// changedNewLines returns the new side's numbers (1-based) of a file's
// added lines.
func changedNewLines(f *diff.File) map[int]bool {
	out := map[int]bool{}
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Kind == diff.Add {
				out[l.New] = true
			}
		}
	}
	return out
}

// findLines returns the indexes of the lines containing q, in any case.
func findLines(lines []string, q string) []int {
	var out []int
	for i, l := range lines {
		if findRanges(l, q) != nil {
			out = append(out, i)
		}
	}
	return out
}

// hunkLine is the line of a hunk to open the file at: its first added
// line, else its start.
func hunkLine(h diff.Hunk) int {
	for _, l := range h.Lines {
		if l.Kind == diff.Add {
			return l.New
		}
	}
	return h.NewStart
}

// viewSelectedHunk opens the file of the hunk chosen with j and k at it.
func (w *window) viewSelectedHunk() {
	if w.selFile < 0 || w.selFile >= len(w.files) {
		return
	}
	f := w.files[w.selFile]
	if !canViewFull(f) {
		return
	}
	line := 0
	if w.selHunk >= 0 && w.selHunk < len(f.Hunks) {
		line = hunkLine(f.Hunks[w.selHunk])
	}
	w.openFileTab(f.Path, line)
}

// fileTabRow shows the tabs above the diff: Changes, then the files open.
func (w *window) fileTabRow(c *ui.Context, pal *palette) {
	t := c.Theme()
	closing := 0
	tabs := ui.TabsBase(c, &w.activeTab, 1+len(w.fileTabs))
	tabs.List.Height(36).Padding(0, 8).Gap(2).BorderWidth(0, 0, 1, 0).BorderColor(pal.cardBorder).
		Background(pal.headerBg.Alpha(0.6)).Label("Tabs").Children(func() {
		for i := range 1 + len(w.fileTabs) {
			on := i == w.activeTab
			label, tip := "Changes", "The changes of the pull request"
			if i > 0 {
				label, tip = filepath.Base(w.fileTabs[i-1].path), w.fileTabs[i-1].path
			}
			tab := tabs.Tab(i).Height(36).Padding(0, 6, 0, 10).Gap(6).Tooltip(tip).TextColor(t.TextMuted).
				BorderWidth(0, 0, 2, 0).BorderColor(ui.Transparent)
			if i == 0 {
				tab.Padding(0, 10)
			}
			switch {
			case on:
				tab.TextColor(t.Text).BorderColor(t.Accent)
			case tab.Hovered():
				tab.TextColor(t.Text)
			}
			tab.Children(func() {
				if i == 0 {
					ui.Icon(c, iconFileDiff).FontSize(em(c, 14))
				} else if f := w.fileByPath(w.fileTabs[i-1].path); f != nil {
					fileIcon(c, f, 14)
				} else {
					ui.Icon(c, iconFile).FontSize(em(c, 14))
				}
				text := ui.Text(c, label).FontSize(em(c, 12)).SingleLine()
				if on {
					text.FontWeight(600)
				}
				if i > 0 && iconButton(c, iconClose, "Close "+label).Size(20, 20).Radius(5).Clicked() {
					closing = i
				}
			})
		}
	})
	if closing > 0 {
		w.closeFileTab(closing)
	}
}

// fileView shows the whole file of a tab, its changed lines marked.
func (w *window) fileView(c *ui.Context, pal *palette, tab *fileTab) {
	f := w.fileByPath(tab.path)
	var lines []string
	if f != nil {
		lines = f.newLines
	}
	if tab.foundFor != tab.query || tab.foundLines != f {
		tab.found = nil
		if f != nil && f.loaded {
			tab.found = findLines(lines, tab.query)
			tab.foundFor, tab.foundLines = tab.query, f
		}
	}
	matches := tab.found
	if !tab.finding {
		matches = nil
	}
	find := ui.FindBar(c, &tab.finding, &tab.query, len(matches), &tab.match).Label("Find in file")
	if find.FocusWithin() {
		w.typing = true
	}
	if find.Changed() && tab.match >= 0 && tab.match < len(matches) {
		tab.list.ScrollTo(matches[tab.match], ui.Center)
	}
	switch {
	case f == nil:
		emptyPanel(c, pal, "Not in the pull request anymore", tab.path, nil)
		return
	case !f.loaded:
		thinking(c, "Loading "+filepath.Base(tab.path)+"…")
		return
	case lines == nil:
		emptyPanel(c, pal, "This file cannot be shown", tab.path, nil)
		return
	}
	if isMarkdown(f.Path) {
		ui.Row(c).Height(40).Padding(0, 12).Gap(8).Shrink(0).Background(pal.codeBg).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.cardBorder).Children(func() {
			ui.Spacer(c)
			if !w.mdSource[f.Path] && !tab.finding {
				w.markdownFontButtons(c)
				w.markdownWidthButtons(c, pal)
			}
			w.sourceToggle(c, pal, f.Path)
		})
		// The find bar finds in the source.
		if !w.mdSource[f.Path] && !tab.finding {
			w.markdownTab(c, pal, f)
			return
		}
	}
	changed := changedNewLines(f.File)
	active := -1
	if tab.match >= 0 && tab.match < len(matches) {
		active = matches[tab.match]
	}
	digits := max(len(fmt.Sprint(len(lines))), 2)
	gutter := float32(digits)*w.charWidth(c) + 18
	ui.List(c, &tab.list, len(lines), func(i int) {
		w.fullLine(c, pal, f, tab, i, changed[i+1], i == active, gutter)
	}).Grow(1).Padding(8, 0, 24).Background(pal.codeBg).Label("Full file " + tab.path)
}

// fullLine shows line i of a whole file: its number, the bar of a changed
// line, and its code.
func (w *window) fullLine(c *ui.Context, pal *palette, f *fileState, tab *fileTab, i int, changed, active bool, gutter float32) {
	lh := w.lineHeight()
	key := "\x00full\x00" + f.Path
	e := ui.Row(c).AlignItems(ui.Stretch).MinHeight(lh).Background(pal.codeBg)
	if !w.settings.WordWrap {
		handle := ui.Local(e, "input-handle", func() ui.Handle { return ui.Handle{} })
		e.Bind(handle)
		charWidth := w.charWidth(c)
		e.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputScroll || math.Abs(float64(ev.DX)) <= math.Abs(float64(ev.DY)) {
				return false
			}
			b := handle.Bounds(c)
			limit := max(float32(f.metrics().maxCols)*charWidth-(b.W-gutter-24), 0)
			w.hscroll[key] = min(max(w.hscroll[key]+ev.DX, 0), limit)
			w.invalidate()
			return true
		})
	}
	bar, numColor := ui.Transparent, pal.lineNumber
	if changed {
		bar, numColor = pal.addBar, pal.addBar
	}
	e.Children(func() {
		ui.Row(c).Shrink(0).AlignItems(ui.Start).Children(func() {
			ui.Box(c).Width(3).AlignSelf(ui.Stretch).Background(bar)
			ui.Text(c, fmt.Sprint(i+1)).Font(w.codeFont()).FontSize(w.codeSize()-1).FixedLineHeight(lh).TextColor(numColor).
				Width(gutter-3).TextAlign(ui.End).Padding(0, 8, 0, 0).Shrink(0)
		})
		code := ui.Box(c).Grow(1).Basis(0).MinWidth(0).ClipX().Margin(0, 10)
		code.Children(func() {
			text := ui.RichText(c, w.fullLineSpans(f, tab, i, active, pal)...).Font(w.codeFont()).
				FontSize(w.codeSize()).FixedLineHeight(lh).TextColor(pal.code)
			if !w.settings.WordWrap {
				text.NoWrap().AlignSelf(ui.Start).Left(-w.hscroll[key])
			}
		})
	})
}

// fullLineSpans styles line i of a whole file, kept from frame to frame
// apart from the lines holding matches of the tab's find bar.
func (w *window) fullLineSpans(f *fileState, tab *fileTab, i int, active bool, pal *palette) []ui.Span {
	text := f.newLines[i]
	var hl []highlight.Seg
	if i < len(f.newHL) {
		hl = f.newHL[i]
	}
	if tab.finding {
		if found := findRanges(text, tab.query); found != nil {
			color := pal.match
			if active {
				color = pal.matchNow
			}
			marks := make([]mark, len(found))
			for j, rg := range found {
				marks[j] = mark{Range: rg, color: color}
			}
			return codeSpans(text, hl, marks, pal)
		}
	}
	// The diff's lines have hunks of 0 and more, and -1 for context.
	key := spanKey{hunk: -2, index: -2, num: int32(i + 1), side: sideNew, dark: pal == &darkPalette}
	if spans, ok := f.spans[key]; ok {
		return spans
	}
	spans := codeSpans(text, hl, nil, pal)
	if f.spans == nil {
		f.spans = map[spanKey][]ui.Span{}
	}
	f.spans[key] = spans
	return spans
}
