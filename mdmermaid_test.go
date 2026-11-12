package main

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

const mermaidReadme = "# App\n\nThe flow:\n\n```mermaid\nflowchart LR\n  A[Review] --> B{Approve?}\n  B -->|Yes| C[Merge]\n```\n\nAnd a broken one:\n\n```mermaid\nflowchart LR\n  A[unclosed -->\n```\n"

func TestMermaidPreview(t *testing.T) {
	w, tt := openedReadme(t, `[]`, mermaidReadme)
	w.previewFile("README.md")
	tt.SetSize(1600, 1200)
	for range 3 {
		tt.Frame()
	}
	snapshot(t, tt, "mermaid")
	if len(w.mermaid) != 2 {
		t.Fatalf("%d diagrams laid out, want 2", len(w.mermaid))
	}
	var drawn, failed *mermaidDiagram
	for k, m := range w.mermaid {
		if !m.done {
			t.Fatalf("diagram not laid out: %q", k.src)
		}
		if strings.Contains(k.src, "Approve") {
			drawn = m
		} else {
			failed = m
		}
	}
	if drawn == nil || drawn.err != nil || drawn.shapes == nil {
		t.Fatalf("the flowchart: %+v", drawn)
	}
	r, ok := tt.Find("Mermaid diagram")
	if !ok {
		t.Fatal("no diagram shows")
	}
	if r.W != float32(drawn.d.Width) || r.H != float32(drawn.d.Height) {
		t.Errorf("diagram %v×%v, laid out %v×%v", r.W, r.H, drawn.d.Width, drawn.d.Height)
	}
	var texts []string
	for _, l := range drawn.labels {
		texts = append(texts, l.Text())
		if l.width <= 0 {
			t.Errorf("label %q has no width", l.Text())
		}
	}
	for _, want := range []string{"Review", "Approve?", "Merge", "Yes"} {
		if !strings.Contains(strings.Join(texts, "|"), want) {
			t.Errorf("no label %q in %q", want, texts)
		}
	}
	// The flowchart's source gives way to it; the broken one shows its own,
	// and why.
	if tt.HasText("A[Review] --> B{Approve?}") {
		t.Error("the flowchart's source shows")
	}
	if failed == nil || failed.err == nil || !tt.HasText("A[unclosed -->") {
		t.Errorf("the broken diagram: %+v, texts %q", failed, tt.Texts())
	}
	found := false
	for _, s := range tt.Texts() {
		found = found || strings.HasPrefix(s, "Mermaid: ")
	}
	if !found {
		t.Error("no error shows for the broken diagram")
	}

	// A narrow window draws the diagram narrower, as tall as it keeps.
	setMarkdownWidth("narrow")
	w.applySettings(cfg.Get())
	w.sidebarShown, w.filesShown = false, false
	tt.SetSize(420, 1200)
	tt.Frame()
	if r, ok := tt.Find("Mermaid diagram"); !ok || r.W >= float32(drawn.d.Width) || r.W <= 0 {
		t.Errorf("narrow diagram %+v, laid out %v wide", r, drawn.d.Width)
	} else if ratio := r.W / r.H; ratio < float32(drawn.d.Width/drawn.d.Height)*0.98 || ratio > float32(drawn.d.Width/drawn.d.Height)*1.02 {
		t.Errorf("narrow diagram %v×%v, out of proportion", r.W, r.H)
	}
}

func TestMermaidViewer(t *testing.T) {
	w, tt := openedReadme(t, `[]`, mermaidReadme)
	w.previewFile("README.md")
	tt.SetSize(1400, 900)
	for range 3 {
		tt.Frame()
	}
	if err := tt.Click("Mermaid diagram"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	v := w.viewer
	if v == nil || !tt.HasText("Diagram") {
		t.Fatal("the viewer did not open")
	}
	// It fits the window: the diagram, 441 wide, grows to fill it.
	canvas, ok := tt.Find("Diagram canvas")
	if !ok || !v.fit || v.zoom <= 1 {
		t.Fatalf("fit zoom %v (fit %v), canvas %+v", v.zoom, v.fit, canvas)
	}
	if got := float32(v.m.d.Width) * v.zoom; got > canvas.W {
		t.Errorf("fitted diagram %v wide in a %v wide view", got, canvas.W)
	}
	snapshot(t, tt, "mermaid-viewer-fit")
	fit := v.zoom

	if err := tt.Click("Zoom In"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if v.fit || math.Abs(float64(v.zoom-clampZoom(fit*mermaidZoomStep))) > 1e-4 {
		t.Errorf("zoom %v after Zoom In from %v", v.zoom, fit)
	}
	tt.Key(0, ui.KeyMinus)
	tt.Frame()
	if math.Abs(float64(v.zoom-fit)) > 1e-4 {
		t.Errorf("zoom %v after -, want %v", v.zoom, fit)
	}
	tt.Key(0, ui.Key0)
	tt.Frame()
	if v.zoom != 1 {
		t.Errorf("zoom %v after 0, want 1", v.zoom)
	}
	for range 12 {
		tt.Key(ui.Shift, ui.KeyEqual)
	}
	tt.Frame()
	if v.zoom != maxMermaidZoom {
		t.Errorf("zoom %v after zooming far in, want %v", v.zoom, maxMermaidZoom)
	}
	tt.Frame()
	snapshot(t, tt, "mermaid-viewer-zoomed")

	// Dragging pans.
	before := v.scroll
	tt.Press(700, 500)
	tt.Move(600, 450)
	tt.Move(500, 400)
	tt.Release(500, 400)
	tt.Frame()
	if v.scroll.X <= before.X {
		t.Errorf("drag scrolled from %+v to %+v", before, v.scroll)
	}

	tt.Key(0, ui.KeyF)
	tt.Frame()
	if !v.fit || v.zoom != fit {
		t.Errorf("F: zoom %v fit %v, want %v", v.zoom, v.fit, fit)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if w.viewer != nil || tt.HasText("Diagram canvas") {
		t.Error("Escape did not close the viewer")
	}
}

func TestMermaidTiles(t *testing.T) {
	r := ui.Rect{X: 100, Y: 50, W: 1300, H: 600}
	tiles := mermaidTiles(r, ui.Rect{X: 0, Y: 0, W: 900, H: 400}, 2)
	// Columns 0 and 1 (100..1124 shows 100..900), row 0 (50..562 shows 50..400).
	if len(tiles) != 2 {
		t.Fatalf("%d tiles: %+v", len(tiles), tiles)
	}
	if tl := tiles[1]; tl.col != 1 || tl.row != 0 || tl.rect != (ui.Rect{X: 612, Y: 50, W: 512, H: 512}) || tl.box != (ui.Rect{X: 256, Y: 0, W: 256, H: 256}) {
		t.Errorf("tile %+v", tl)
	}
	// The last column is cut by the drawing.
	all := mermaidTiles(r, ui.Rect{X: 0, Y: 0, W: 5000, H: 5000}, 2)
	if len(all) != 3*2 {
		t.Fatalf("%d tiles of the whole drawing", len(all))
	}
	last := all[len(all)-1]
	if last.rect.W != 1300-1024 || last.rect.H != 600-512 || last.box.W != (1300-1024)/2 {
		t.Errorf("last tile %+v", last)
	}
	if got := mermaidTiles(r, ui.Rect{X: 2000, Y: 0, W: 10, H: 10}, 2); len(got) != 0 {
		t.Errorf("tiles out of view: %+v", got)
	}
}

// A diagram larger than MyGo draws an SVG at once still shows its shapes.
func TestMermaidViewerLarge(t *testing.T) {
	src := "# Wide\n\n```mermaid\nflowchart LR\n  A[One] --> B[Two] --> C[Three] --> D[Four] --> E[Five] --> F[Six] --> G[Seven] --> H[Eight]\n```\n"
	w, tt := openedReadme(t, `[]`, src)
	w.previewFile("README.md")
	tt.SetSize(1400, 900)
	for range 3 {
		tt.Frame()
	}
	if err := tt.Click("Mermaid diagram"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	v := w.viewer
	v.setZoom(maxMermaidZoom)
	v.scroll.X, v.scroll.Y = 0, 0
	for range 3 {
		tt.Frame()
	}
	if width := float32(v.m.d.Width) * v.zoom; width <= 4096 {
		t.Fatalf("the drawing is %v wide, not past MyGo's 4096", width)
	}
	snapshot(t, tt, "mermaid-viewer-large")
	// The first node's fill, the theme's surface, covers much of the view.
	img := tt.Image()
	fill := ui.Hex("#f6f8fa")
	canvas, _ := tt.Find("Diagram canvas")
	n := 0
	for y := int(canvas.Y) + 60; y < int(canvas.Y)+600; y += 4 {
		for x := 0; x < 1400; x += 4 {
			c := img.RGBAAt(x, y)
			if absDiff(c.R, fill.R) <= 2 && absDiff(c.G, fill.G) <= 2 && absDiff(c.B, fill.B) <= 2 {
				n++
			}
		}
	}
	if n < 1000 {
		t.Errorf("%d samples of the nodes' fill: the shapes do not show", n)
	}

	// Fitted to a large window on a Retina screen, as the viewer opens.
	tt.SetScale(2)
	tt.SetSize(2560, 1400)
	v.fit = true
	for range 4 {
		tt.Frame()
	}
	if width := float32(v.m.d.Width) * v.zoom * 2; width <= 4096 {
		t.Fatalf("the fitted drawing is %v pixels wide, not past MyGo's 4096", width)
	}
	snapshot(t, tt, "mermaid-viewer-retina")
	img = tt.Image()
	n = 0
	for y := 0; y < img.Bounds().Dy(); y += 6 {
		for x := 0; x < img.Bounds().Dx(); x += 6 {
			c := img.RGBAAt(x, y)
			if absDiff(c.R, fill.R) <= 2 && absDiff(c.G, fill.G) <= 2 && absDiff(c.B, fill.B) <= 2 {
				n++
			}
		}
	}
	if n < 1000 {
		t.Errorf("%d samples of the nodes' fill on Retina: the shapes do not show", n)
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
