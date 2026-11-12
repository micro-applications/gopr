package main

import (
	"context"
	"sync"

	"github.com/egoist/godiff/internal/mermaid"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// mermaidKey is what a diagram is drawn from: its source, the appearance
// and the size of its labels.
type mermaidKey struct {
	src  string
	dark bool
	size float32
}

// mermaidDiagram is a Mermaid diagram of a preview: laid out, or why not.
type mermaidDiagram struct {
	done    bool
	err     error
	d       *mermaid.Diagram
	shapes  *ui.SVG
	labels  []mermaidLabel
	codeFam string
}

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
func mermaidFont(size float64, weight int, italic, mono bool, codeFamily string) ui.Font {
	f := ui.Font{Size: float32(size), Weight: weight, Italic: italic}
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
		w = float64(glyphsWidth(ui.Shape(text, mermaidFont(size, weight, style&mermaid.Italic != 0, style&mermaid.Mono != 0, m.codeFamily))))
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

// mermaidBlock shows a Mermaid code block of a preview as its diagram:
// its code until the diagram is laid out, and with why when it cannot be.
func (w *window) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView) {
	if b.Open {
		markdown.DefaultCodeBlock(c, b)
		return
	}
	t := c.Theme()
	k := mermaidKey{src: b.Code(), dark: t.Dark, size: float32(w.settings.MarkdownFontSize)}
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
	m := &mermaidDiagram{codeFam: codeFamily}
	w.mermaid[k] = m
	theme := mermaid.Light
	if k.dark {
		theme = mermaid.Dark
	}
	measure := &mainMeasurer{run: w.onMain, codeFamily: codeFamily, widths: map[measureKey]float64{}}
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
	for _, l := range d.Labels {
		ml := mermaidLabel{Label: l}
		for _, r := range l.Runs {
			gs := ui.Shape(r.Text, mermaidFont(r.Size, r.Weight, r.Italic, r.Mono, m.codeFam))
			color := ui.Hex(r.Fill)
			if r.Opacity < 1 {
				color = color.Alpha(float32(r.Opacity))
			}
			ml.runs = append(ml.runs, mermaidRun{glyphs: gs, x: ml.width, color: color})
			ml.width += glyphsWidth(gs)
		}
		m.labels = append(m.labels, ml)
	}
	return nil
}

// mermaidView draws a diagram at its size, or narrower to fit, its labels
// over its shapes.
func (w *window) mermaidView(c *ui.Context, m *mermaidDiagram) {
	d := m.d
	width, height := float32(d.Width), float32(d.Height)
	ui.Row(c).Justify(ui.Center).Children(func() {
		ui.Box(c).Width(width).Shrink(1).MinWidth(0).AspectRatio(width / height).Label("Mermaid diagram").
			Draw(func(p *ui.Painter, r ui.Rect) {
				p.Image(m.shapes, r, ui.Contain)
				s := r.W / width
				p.Transform(ui.Scaling(s, s), r.X, r.Y, func() {
					for _, l := range m.labels {
						x, y := r.X+float32(l.X), r.Y+float32(l.Y)
						switch l.Anchor {
						case mermaid.Middle:
							x -= l.width / 2
						case mermaid.End:
							x -= l.width
						}
						draw := func() {
							for _, run := range l.runs {
								p.Glyphs(run.glyphs, x+run.x, y, run.color)
							}
						}
						if l.Angle != 0 {
							p.Transform(ui.Rotation(float32(l.Angle)), r.X+float32(l.X), r.Y+float32(l.Y), draw)
						} else {
							draw()
						}
					}
				})
			})
	})
}
