package main

import (
	"errors"
	"net/url"
	"path"
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

// previewing reports whether Changes shows a file rendered rather than
// its lines.
func (w *window) previewing(f *fileState) bool {
	return canPreview(f) && !w.mdDiff[f.Path]
}

// setMarkdownDiff shows file fi's lines in Changes (on), or its preview.
func (w *window) setMarkdownDiff(fi int, on bool) {
	if fi < 0 || fi >= len(w.files) {
		return
	}
	if w.mdDiff == nil {
		w.mdDiff = map[string]bool{}
	}
	w.mdDiff[w.files[fi].Path] = on
	// The hunk chosen with j and k does not show in the preview.
	if !on && w.selFile == fi {
		w.selFile, w.selHunk = -1, -1
	}
	// The preview's lines are not found by the find bar: find again.
	if w.searching() {
		w.matchesFor = "\x00"
	}
	w.rowsDirty = true
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

// markdownRow shows a changed Markdown file rendered, in its card.
func (w *window) markdownRow(c *ui.Context, pal *palette, f *fileState) {
	w.card(c, pal).Padding(14, 24, 18).Children(func() {
		switch {
		case !f.loaded:
			ui.Text(c, "Loading…").FontSize(12).TextColor(pal.gapText)
		case f.newLines == nil:
			ui.Text(c, "This file cannot be shown.").FontSize(12).TextColor(pal.gapText)
		default:
			markdown.View(c, strings.Join(f.newLines, "\n"), w.markdownOptions(f.Path)).Grow(1).MinWidth(0)
		}
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

// diffToggle is the Preview/Diff switch of a Markdown file's header; Diff
// counts the file's unresolved threads, which show there alone.
func (w *window) diffToggle(c *ui.Context, pal *palette, idx int, f *fileState) {
	choice := 0
	if w.mdDiff[f.Path] {
		choice = 1
	}
	diffText := "Diff"
	if n := w.openThreads(f.Path); n > 0 {
		diffText = "Diff · " + itoa(n)
	}
	previewToggle(c, pal, "Show "+f.Name()+" as", [2]string{"Preview", "Diff"}, [2]string{"Preview", diffText}, choice, func(to int) {
		w.setMarkdownDiff(idx, to == 1)
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
			markdown.View(c, strings.Join(f.newLines, "\n"), w.markdownOptions(f.Path))
		})
	})
}
