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
