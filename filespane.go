package main

import "github.com/egoist/mygo/ui"

// The files pane's width: its default, its bounds, and the width below
// which dragging hides it.
const (
	filesDefault  = 240
	filesMin      = 160
	filesMax      = 480
	filesCollapse = 80
)

// filesPane shows the changed files left of the diff: a filter, their
// tree, and the total of their changes.
func (w *window) filesPane(c *ui.Context, pal *palette) {
	ui.Column(c).Width(w.filesWidth).Shrink(0).MinHeight(0).Label("Files").Children(func() {
		ui.Column(c).Padding(8, 10, 6).Children(func() {
			if searchInput(c, &w.filter, "Filter files", &w.filterFocus, &w.typing) {
				w.matchesFor = "\x00"
				w.buildTree()
				w.rowsDirty = true
			}
		})
		w.fileTree(c)
		w.filesFooter(c, pal)
	})
}

// filesFooter shows the total of the changes of the files that show.
func (w *window) filesFooter(c *ui.Context, pal *palette) {
	t := c.Theme()
	adds, dels, counted := 0, 0, false
	for i, f := range w.files {
		if w.fileVisible(i) && countable(f) {
			adds += f.Additions
			dels += f.Deletions
			counted = true
		}
	}
	if !counted {
		return
	}
	ui.Row(c).MinHeight(40).Padding(6, 10).Gap(6).BorderWidth(1, 0, 0, 0).BorderColor(pal.cardBorder).
		Tooltip("Total change: " + lines(adds, "added") + ", " + lines(dels, "removed")).Children(func() {
		ui.Text(c, "Total:").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
		ui.Text(c, "+"+thousands(adds)).FontFeatures("tnum").FontSize(11).FontWeight(500).TextColor(pal.addText)
		ui.Text(c, "−"+thousands(dels)).FontFeatures("tnum").FontSize(11).FontWeight(500).TextColor(pal.delText)
	})
}

// filesResizer is the edge of the files pane the user drags.
func (w *window) filesResizer(c *ui.Context, pal *palette) {
	ui.Box(c).Width(1).Shrink(0).Background(pal.cardBorder).Children(func() {
		handle := ui.Box(c).Absolute().Top(0).Bottom(0).Left(-4).Width(9).Cursor(ui.CursorResizeEW)
		if dx, _, held := handle.Dragged(); held {
			w.filesDragging = true
			w.filesDragWidth += dx
			switch {
			case w.filesDragWidth < filesCollapse:
				// The resizer goes with the pane: no release comes to save it.
				w.filesShown = false
				w.filesDragging = false
				w.filesDragWidth = w.filesWidth
				w.saveFilesLayout()
			default:
				w.filesShown = true
				w.filesWidth = min(max(w.filesDragWidth, filesMin), filesMax)
			}
		} else {
			if w.filesDragging {
				w.filesDragging = false
				w.saveFilesLayout()
			}
			w.filesDragWidth = w.filesWidth
		}
	})
}

// toggleFiles shows and hides the files pane.
func (w *window) toggleFiles() {
	w.focus = false
	w.filesShown = !w.filesShown
	if w.filesShown && w.filesWidth < filesMin {
		w.filesWidth = filesDefault
	}
	w.filesDragWidth = w.filesWidth
	w.saveFilesLayout()
}

// focusMaxWidth is how wide the review reads in focus mode.
const focusMaxWidth = 1600

// toggleFocus hides the sidebar and the files pane, leaving the review
// centered, or shows them again as they were. It keeps the layout saved
// as it was.
func (w *window) toggleFocus() {
	if w.focus {
		w.focus = false
		w.sidebarShown, w.filesShown = w.focusPrev[0], w.focusPrev[1]
		return
	}
	w.focus = true
	w.focusPrev = [2]bool{w.sidebarShown, w.filesShown}
	w.sidebarShown, w.filesShown = false, false
}

// saveFilesLayout remembers the files pane's layout, off the main thread.
func (w *window) saveFilesLayout() {
	width, shown := w.filesWidth, w.filesShown
	w.background(func() { state.setFilesLayout(width, shown) })
}
