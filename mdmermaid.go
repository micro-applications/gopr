package main

import (
	"context"
	"math"
	"sync"

	"github.com/egoist/godiff/internal/mermaid"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// mermaidKey is what a diagram is drawn from: its source, the appearance
// and the size of its labels.
type mermaidKey struct {
	src    string
	dark   bool
	size   float32
	family string
}

// mermaidDiagram is a Mermaid diagram of a preview: laid out, or why not.
type mermaidDiagram struct {
	done    bool
	err     error
	d       *mermaid.Diagram
	shapes  *ui.SVG
	labels  []mermaidLabel
	codeFam string
	family  string // of the labels' text, the system's when empty
	// scaled are the labels shaped at the scales drawn other than 1, by the
	// scale in hundredths: glyphs scaled as images would blur.
	scaled map[int][]mermaidLabel
	// tiles are the shapes cut to the tiles of a large drawing, by scale
	// and place.
	tiles map[mermaidTileKey]*ui.SVG
}

// mermaidTileKey is a tile of a drawing: at a scale in thousandths, the
// tile x, y of the grid, w×h DIPs.
type mermaidTileKey struct{ scale, x, y, w, h int }

// mermaidLabel is a label shaped to draw.
type mermaidLabel struct {
	mermaid.Label
	runs  []mermaidRun
	width float32
}

type mermaidRun struct {
	glyphs []ui.Glyph
	x      float32 // from the label's start
	color  ui.Color
}

// maxMermaid is how many diagrams the window keeps drawn.
const maxMermaid = 64

// mermaidFont is the font a run of a label is measured and drawn in.
func mermaidFont(size float64, weight int, italic, mono bool, codeFamily, family string) ui.Font {
	f := ui.Font{Size: float32(size), Weight: weight, Italic: italic, Family: family}
	if mono {
		f.Family = codeFamily
	}
	return f
}

// glyphsWidth is how far glyphs reach.
func glyphsWidth(gs []ui.Glyph) float32 {
	var w float32
	for _, g := range gs {
		w = max(w, g.X+g.Advance)
	}
	return w
}

// mainMeasurer measures labels as the window draws them. MyGo's text
// system belongs to the main thread, where run takes each measure; it
// keeps what it measured, for diagrams measure the same text often.
type mainMeasurer struct {
	run        func(func())
	codeFamily string
	family     string
	mu         sync.Mutex
	widths     map[measureKey]float64
}

type measureKey struct {
	text  string
	size  float64
	style mermaid.Style
}

func (m *mainMeasurer) Width(text string, size float64, style mermaid.Style) float64 {
	k := measureKey{text, size, style}
	m.mu.Lock()
	w, ok := m.widths[k]
	m.mu.Unlock()
	if ok {
		return w
	}
	weight := 400
	if style&mermaid.Bold != 0 {
		weight = 700
	}
	m.run(func() {
		w = float64(glyphsWidth(ui.Shape(text, mermaidFont(size, weight, style&mermaid.Italic != 0, style&mermaid.Mono != 0, m.codeFamily, m.family))))
	})
	m.mu.Lock()
	m.widths[k] = w
	m.mu.Unlock()
	return w
}

// onMain runs fn on the main thread: at once in tests, which have no
// window and run the background work on it.
func (w *window) onMain(fn func()) {
	if w.win == nil {
		fn()
		return
	}
	mygo.RunOnMain(fn)
}

// markdownFamily is the family Markdown's text shows in: the settings'
// for Markdown, else the window's.
func (w *window) markdownFamily() string {
	if w.settings.MarkdownFontFamily != "" {
		return w.settings.MarkdownFontFamily
	}
	return w.settings.UIFontFamily
}

// mermaidBlock shows a Mermaid code block of a preview as its diagram:
// its code until the diagram is laid out, and with why when it cannot be.
func (w *window) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView) {
	if b.Open {
		markdown.DefaultCodeBlock(c, b)
		return
	}
	t := c.Theme()
	k := mermaidKey{src: b.Code(), dark: t.Dark, size: float32(w.settings.MarkdownFontSize), family: w.markdownFamily()}
	m := w.mermaid[k]
	if m == nil {
		m = w.layOutMermaid(k)
	}
	switch {
	case !m.done:
		markdown.DefaultCodeBlock(c, b)
	case m.err != nil:
		ui.Column(c).Gap(6).Children(func() {
			markdown.DefaultCodeBlock(c, b)
			ui.Text(c, "Mermaid: "+errorText(m.err)).FontSize(12).TextColor(t.Danger)
		})
	default:
		w.mermaidView(c, m)
	}
}

// layOutMermaid lays a diagram out in the background, and shapes its
// labels on the main thread once it is.
func (w *window) layOutMermaid(k mermaidKey) *mermaidDiagram {
	if w.mermaid == nil || len(w.mermaid) >= maxMermaid {
		w.mermaid = map[mermaidKey]*mermaidDiagram{}
	}
	codeFamily := w.settings.CodeFontFamily
	m := &mermaidDiagram{codeFam: codeFamily, family: k.family}
	w.mermaid[k] = m
	theme := mermaid.Light
	if k.dark {
		theme = mermaid.Dark
	}
	measure := &mainMeasurer{run: w.onMain, codeFamily: codeFamily, family: k.family, widths: map[measureKey]float64{}}
	w.background(func() {
		d, err := mermaid.Render(context.Background(), k.src, theme, measure, float64(k.size))
		w.update(func() {
			if err == nil {
				err = m.prepare(d)
			}
			m.done, m.err = true, err
			w.invalidate()
		})
	})
	return m
}

// prepare parses a diagram's shapes and shapes its labels, on the main
// thread.
func (m *mermaidDiagram) prepare(d *mermaid.Diagram) error {
	shapes, err := ui.ParseSVG(d.Shapes)
	if err != nil {
		return err
	}
	m.d, m.shapes = d, shapes
	m.labels = m.shape(1)
	return nil
}

// shape shapes the labels at scale s.
func (m *mermaidDiagram) shape(s float32) []mermaidLabel {
	var labels []mermaidLabel
	for _, l := range m.d.Labels {
		ml := mermaidLabel{Label: l}
		for _, r := range l.Runs {
			gs := ui.Shape(r.Text, mermaidFont(r.Size*float64(s), r.Weight, r.Italic, r.Mono, m.codeFam, m.family))
			color := ui.Hex(r.Fill)
			if r.Opacity < 1 {
				color = color.Alpha(float32(r.Opacity))
			}
			ml.runs = append(ml.runs, mermaidRun{glyphs: gs, x: ml.width, color: color})
			ml.width += glyphsWidth(gs)
		}
		labels = append(labels, ml)
	}
	return labels
}

// labelsAt returns the labels shaped at scale s, kept for the next frames.
func (m *mermaidDiagram) labelsAt(s float32) []mermaidLabel {
	k := int(math.Round(float64(s) * 100))
	if k == 100 {
		return m.labels
	}
	if l, ok := m.scaled[k]; ok {
		return l
	}
	if m.scaled == nil || len(m.scaled) >= 16 {
		m.scaled = map[int][]mermaidLabel{}
	}
	l := m.shape(float32(k) / 100)
	m.scaled[k] = l
	return l
}

// mermaidView draws a diagram at its size, or narrower to fit, its labels
// over its shapes; a click shows it in the viewer.
func (w *window) mermaidView(c *ui.Context, m *mermaidDiagram) {
	width, height := float32(m.d.Width), float32(m.d.Height)
	ui.Row(c).Justify(ui.Center).Children(func() {
		box := ui.Box(c).Width(width).Shrink(1).MinWidth(0).AspectRatio(width / height).
			Label("Mermaid diagram").Tooltip("Open the diagram to zoom").Cursor(ui.CursorPointer).
			Draw(func(p *ui.Painter, r ui.Rect) {
				// The viewer shows it, from here and back here, meanwhile.
				if w.viewer == nil || w.viewer.m != m {
					drawMermaid(p, m, r)
				}
			})
		if box.Clicked() {
			w.openMermaid(m, box.Bounds())
		}
	})
}

// drawMermaid draws a diagram in r, scaled to its width: the shapes as
// vectors, the labels shaped at the scale, for sharp text.
func drawMermaid(p *ui.Painter, m *mermaidDiagram, r ui.Rect) {
	s := r.W / float32(m.d.Width)
	m.drawShapes(p, r, s)
	labels := m.labelsAt(s)
	// The labels are shaped at a scale rounded to hundredths.
	k := float32(math.Round(float64(s)*100)) / 100
	for _, l := range labels {
		ax, ay := r.X+float32(l.X)*s, r.Y+float32(l.Y)*s
		x := ax
		switch l.Anchor {
		case mermaid.Middle:
			x -= l.width / 2 * s / k
		case mermaid.End:
			x -= l.width * s / k
		}
		draw := func() {
			for _, run := range l.runs {
				p.Glyphs(run.glyphs, x+run.x*s/k, ay, run.color)
			}
		}
		if l.Angle != 0 {
			p.Transform(ui.Rotation(float32(l.Angle)), ax, ay, draw)
		} else {
			draw()
		}
	}
}

// The drawing's size, in DIPs on each side, past which its shapes draw in
// tiles, of mermaidTile DIPs: MyGo draws an SVG as one picture, which it
// does not draw past 4096 pixels on a side, at two pixels a DIP or more.
const (
	mermaidWhole = 1024
	mermaidTile  = 512
)

// drawShapes draws the diagram's shapes in r, at scale s: as one picture
// when small, else the tiles of it that show, each its own picture.
func (m *mermaidDiagram) drawShapes(p *ui.Painter, r ui.Rect, s float32) {
	if r.W*max(p.Scale(), 1) <= 2*mermaidWhole && r.H*max(p.Scale(), 1) <= 2*mermaidWhole {
		p.Image(m.shapes, r, ui.Contain)
		return
	}
	for _, t := range mermaidTiles(r, p.Visible(), s) {
		k := mermaidTileKey{scale: int(math.Round(float64(s) * 1000)), x: t.col, y: t.row, w: int(t.rect.W), h: int(t.rect.H)}
		svg, ok := m.tiles[k]
		if !ok {
			if m.tiles == nil || len(m.tiles) >= 256 {
				m.tiles = map[mermaidTileKey]*ui.SVG{}
			}
			svg, _ = ui.ParseSVG(m.d.Crop(float64(t.box.X), float64(t.box.Y), float64(t.box.W), float64(t.box.H), float64(t.rect.W), float64(t.rect.H)))
			m.tiles[k] = svg
		}
		if svg != nil {
			p.Image(svg, t.rect, ui.FillBox)
		}
	}
}

// mermaidTileOf is a tile of a drawing: where it draws, in DIPs, and the
// part of the drawing it shows, in its units.
type mermaidTileOf struct {
	col, row int
	rect     ui.Rect
	box      ui.Rect
}

// mermaidTiles are the tiles of a drawing drawn in r at scale s that fall
// in visible: a grid of mermaidTile DIPs from r's corner, cut by r.
func mermaidTiles(r, visible ui.Rect, s float32) []mermaidTileOf {
	if s <= 0 {
		return nil
	}
	x0, y0 := max(r.X, visible.X), max(r.Y, visible.Y)
	x1, y1 := min(r.X+r.W, visible.X+visible.W), min(r.Y+r.H, visible.Y+visible.H)
	if x1 <= x0 || y1 <= y0 {
		return nil
	}
	var tiles []mermaidTileOf
	for row := int((y0 - r.Y) / mermaidTile); float32(row)*mermaidTile < y1-r.Y; row++ {
		for col := int((x0 - r.X) / mermaidTile); float32(col)*mermaidTile < x1-r.X; col++ {
			tx, ty := float32(col)*mermaidTile, float32(row)*mermaidTile
			tw, th := min(mermaidTile, r.W-tx), min(mermaidTile, r.H-ty)
			tiles = append(tiles, mermaidTileOf{
				col: col, row: row,
				rect: ui.Rect{X: r.X + tx, Y: r.Y + ty, W: tw, H: th},
				box:  ui.Rect{X: tx / s, Y: ty / s, W: tw / s, H: th / s},
			})
		}
	}
	return tiles
}
