package main

import (
	"strings"
	"testing"
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
