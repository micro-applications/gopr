package main

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/egoist/godiff/internal/diff"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// isMarkdown reports whether a path names a Markdown file: .md,
// .markdown or .mdx, in any case.
func isMarkdown(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}

// canPreview reports whether a changed file is Markdown with a new
// version to render.
func canPreview(f *fileState) bool {
	return isMarkdown(f.Path) && f.Status != diff.Deleted && !f.Binary && !f.Directory
}

// errNoImage is why a picture of a preview does not load.
var errNoImage = errors.New("no such picture in the pull request")

// imagePath resolves the address of a picture in the Markdown file in dir
// to its path in the repository: addresses with a scheme or a host,
// absolute paths and paths out of the repository have none.
func imagePath(dir, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	p := u.Path
	if u.Scheme != "" || u.Host != "" || u.Opaque != "" || strings.HasPrefix(raw, "//") ||
		p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) {
		return "", errNoImage
	}
	clean := path.Clean(path.Join(dir, p))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", errNoImage
	}
	return clean, nil
}

// markdownOptions returns the options of the preview of the Markdown file
// at path, kept while the head reviewed stays, for they keep its pictures:
// they load from the clone at the head, and links open in the browser, or
// in a tab for the files of the pull request.
func (w *window) markdownOptions(p string) *markdown.Options {
	repo, rev := w.repo, w.newRev
	key := rev
	if repo != nil {
		key = repo.Root + "\x00" + rev
	}
	if w.mdOptsFor != key {
		w.mdOpts, w.mdOptsFor = nil, key
	}
	if o := w.mdOpts[p]; o != nil {
		return o
	}
	dir := path.Dir(p)
	o := &markdown.Options{
		Code: map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": w.mermaidBlock},
		// It runs on a goroutine of its own: repo and rev were taken on the
		// main thread.
		LoadImage: func(raw string) (ui.ImageSource, error) {
			file, err := imagePath(dir, raw)
			if err != nil {
				return nil, err
			}
			if repo == nil {
				return nil, errNoImage
			}
			var data []byte
			if rev == "" {
				data = repo.ReadWorkTree(file)
			} else {
				contents, err := repo.NewContents()
				if err != nil {
					return nil, err
				}
				data, err = contents.Read(rev, file)
				contents.Close()
				if err != nil {
					return nil, err
				}
			}
			if data == nil {
				return nil, errNoImage
			}
			if strings.EqualFold(path.Ext(file), ".svg") {
				return ui.ParseSVG(data)
			}
			return ui.DecodeBitmap(data)
		},
		OnLink: func(raw string) { w.openMarkdownLink(dir, raw) },
	}
	if w.mdOpts == nil {
		w.mdOpts = map[string]*markdown.Options{}
	}
	w.mdOpts[p] = o
	return o
}

// openMarkdownLink opens a link of the Markdown file in dir: web and mail
// addresses outside, and files of the pull request in their tabs.
func (w *window) openMarkdownLink(dir, raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		mygo.Shell.OpenExternal(raw)
		return
	case "":
	default:
		return
	}
	if u.Host != "" || u.Path == "" {
		return
	}
	// A path from the root is from the repository's, as on GitHub.
	target := path.Clean(path.Join(dir, u.Path))
	if strings.HasPrefix(u.Path, "/") {
		target = path.Clean(strings.TrimPrefix(u.Path, "/"))
	}
	if f := w.fileByPath(target); f != nil && canViewFull(f) {
		w.openFileTab(target, 0)
	}
}

// markdownWidth is a width Markdown previews read at: its setting's
// value, its name, and its widest, 0 for the whole view.
type markdownWidth struct {
	key, name string
	max       float32
	icon      *ui.SVG
}

// markdownWidths are the widths a preview may read at, narrowest first.
var markdownWidths = []markdownWidth{
	{"narrow", "Narrow", 640, iconWidthNarrow},
	{"medium", "Medium", 820, iconWidthMedium},
	{"wide", "Wide", 1080, iconWidthWide},
	{"full", "Full Width", 0, iconWidthFull},
}

// markdownWidthOf is the width of a setting's value, nil for none.
func markdownWidthOf(key string) *markdownWidth {
	for i := range markdownWidths {
		if markdownWidths[i].key == key {
			return &markdownWidths[i]
		}
	}
	return nil
}

// markdownMax is how wide the previews read at most, 0 for no limit.
func (w *window) markdownMax() float32 {
	if mw := markdownWidthOf(w.settings.MarkdownWidth); mw != nil {
		return mw.max
	}
	return markdownWidthOf(defaultSettings().MarkdownWidth).max
}

// setMarkdownWidth makes previews read at the width of key.
func setMarkdownWidth(key string) { cfg.Update(func(s *Settings) { s.MarkdownWidth = key }) }

// markdownWidthButtons choose the width previews read at, one icon
// button each, the current one pressed.
func (w *window) markdownWidthButtons(c *ui.Context, pal *palette) {
	t := c.Theme()
	current := markdownWidthOf(w.settings.MarkdownWidth)
	if current == nil {
		current = markdownWidthOf(defaultSettings().MarkdownWidth)
	}
	choice := slices.IndexFunc(markdownWidths, func(mw markdownWidth) bool { return mw.key == current.key })
	shown := choice
	seg := ui.SegmentedBase(c, &choice, len(markdownWidths))
	seg.Track.Padding(2).Gap(2).Radius(8).Background(ui.RGBA(127, 127, 127, 0.1)).Shrink(0).Label("Markdown width").Children(func() {
		for i, mw := range markdownWidths {
			s := seg.Segment(i).Size(30, 24).Radius(6).Center().Label(mw.name).Tooltip(mw.name).TextColor(t.TextMuted)
			if i == shown {
				s.Background(pal.headerBg).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12)).TextColor(t.Text)
			}
			s.OnClick(func() {
				if i != shown {
					setMarkdownWidth(mw.key)
				}
			})
			s.Children(func() { ui.Icon(c, mw.icon).FontSize(15) })
		}
	})
}

// The sizes of Markdown previews' text: the default, and the least and
// most it goes to.
const (
	defaultMarkdownFontSize = 14
	minMarkdownFontSize     = 11
	maxMarkdownFontSize     = 28
)

// changeMarkdownFontSize makes previews' text bigger (1) or smaller (-1),
// or of the default size (0).
func changeMarkdownFontSize(delta int) {
	cfg.Update(func(s *Settings) {
		if delta == 0 {
			s.MarkdownFontSize = defaultMarkdownFontSize
			return
		}
		s.MarkdownFontSize = min(max(s.MarkdownFontSize+delta, minMarkdownFontSize), maxMarkdownFontSize)
	})
}

// markdownFontButtons make previews' text smaller and bigger.
func (w *window) markdownFontButtons(c *ui.Context) {
	size := w.settings.MarkdownFontSize
	ui.Row(c).Gap(2).Shrink(0).Label("Markdown text size").Children(func() {
		smaller := iconButton(c, iconTextSmaller, fmt.Sprintf("Smaller Text (%d)", size)).Size(26, 24).Label("Smaller Text")
		if size <= minMarkdownFontSize {
			smaller.Disabled(true)
		}
		if smaller.Clicked() {
			changeMarkdownFontSize(-1)
		}
		bigger := iconButton(c, iconTextLarger, fmt.Sprintf("Bigger Text (%d)", size)).Size(26, 24).Label("Bigger Text")
		if size >= maxMarkdownFontSize {
			bigger.Disabled(true)
		}
		if bigger.Clicked() {
			changeMarkdownFontSize(1)
		}
	})
}

// markdownColumn shows a preview centered, no wider than the width chosen.
func (w *window) markdownColumn(c *ui.Context, f *fileState) {
	ui.Row(c).Justify(ui.Center).Grow(1).MinWidth(0).Children(func() {
		col := ui.Column(c).Grow(1).MinWidth(0).Label("Preview column")
		if mw := w.markdownMax(); mw > 0 {
			col.MaxWidth(mw)
		}
		col.Children(func() {
			o := w.markdownOptions(f.Path)
			if size := float32(w.settings.MarkdownFontSize); o.Theme == nil || o.Theme.FontSize != size {
				o.Theme = markdownTheme(size)
			}
			markdown.View(c, strings.Join(f.newLines, "\n"), o)
		})
	})
}

// previewToggle switches between a Markdown file rendered (0) and its
// lines (1), named names; texts are the segments' texts. choose takes a
// new choice.
func previewToggle(c *ui.Context, pal *palette, label string, names, texts [2]string, choice int, choose func(int)) {
	t := c.Theme()
	shown := choice
	seg := ui.SegmentedBase(c, &choice, 2)
	seg.Track.Padding(2).Gap(2).Radius(8).Background(ui.RGBA(127, 127, 127, 0.1)).Shrink(0).Label(label).Children(func() {
		for i := range 2 {
			s := seg.Segment(i).Height(24).Padding(0, 9).Radius(6).Center().Label(names[i]).TextColor(t.TextMuted)
			if i == choice {
				s.Background(pal.headerBg).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12)).TextColor(t.Text)
			}
			s.OnClick(func() {
				if i != shown {
					choose(i)
				}
			})
			s.Children(func() { ui.Text(c, texts[i]).FontSize(12).FontWeight(600) })
		}
	})
}

// sourceToggle is the Preview/Source switch of a Markdown file's tab.
func (w *window) sourceToggle(c *ui.Context, pal *palette, p string) {
	choice := 0
	if w.mdSource[p] {
		choice = 1
	}
	previewToggle(c, pal, "Show "+path.Base(p)+" as", [2]string{"Preview", "Source"}, [2]string{"Preview", "Source"}, choice, func(to int) {
		if w.mdSource == nil {
			w.mdSource = map[string]bool{}
		}
		w.mdSource[p] = to == 1
	})
}

// markdownTab shows a Markdown file of a tab rendered.
func (w *window) markdownTab(c *ui.Context, pal *palette, f *fileState) {
	ui.Scroll(c).Grow(1).MinHeight(0).Background(pal.codeBg).Label("Preview " + f.Path).Children(func() {
		ui.Column(c).Padding(16, 28, 32).Children(func() {
			w.markdownColumn(c, f)
		})
	})
}

// previewFile shows a Markdown file rendered, in its tab.
func (w *window) previewFile(p string) {
	if w.mdSource != nil {
		delete(w.mdSource, p)
	}
	w.openFileTab(p, 0)
}

// The rhythm of Markdown previews, as a document reads rather than a
// chat: lines a little over one and a half times the text apart, a
// tight list's items set off a little, and headings with more room above
// them than below. markdownLineHeight is times the font's own line
// height, which is about 1.2 times its size.
var (
	markdownLineHeight   float32 = 1.6
	markdownBlockSpacing float32 = 0.95
	markdownItemSpacing  float32 = 0.4
	markdownHeadings             = [6]markdown.HeadingStyle{
		{Size: 1.6, Weight: 650, Top: 2.2, Bottom: 0.75},
		{Size: 1.33, Weight: 600, Top: 2.5, Bottom: 0.8},
		{Size: 1.15, Weight: 600, Top: 2.1, Bottom: 0.7},
		{Size: 1.05, Weight: 600, Top: 1.9, Bottom: 0.6},
		{Size: 1, Weight: 600, Top: 1.8, Bottom: 0.55},
		{Size: 0.95, Weight: 600, Top: 1.8, Bottom: 0.55},
	}
)

// markdownTheme is the theme of previews whose text is size DIPs.
func markdownTheme(size float32) *markdown.Theme {
	return &markdown.Theme{
		FontSize:     size,
		LineHeight:   markdownLineHeight,
		BlockSpacing: markdownBlockSpacing,
		ItemSpacing:  markdownItemSpacing,
		Headings:     markdownHeadings,
	}
}
