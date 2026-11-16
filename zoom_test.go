package main

import "testing"

func TestNextZoom(t *testing.T) {
	for _, c := range []struct {
		z    float64
		step int
		want float64
	}{
		{1, 1, 1.1}, {1, -1, 0.9}, {1.1, 1, 1.25}, {0.9, -1, 0.8},
		{3, 1, 3}, {0.5, -1, 0.5}, {1.3, -1, 1.25}, {1.3, 1, 1.5}, {2.5, 0, 1},
	} {
		if got := nextZoom(c.z, c.step); got != c.want {
			t.Errorf("nextZoom(%v, %d) = %v, want %v", c.z, c.step, got, c.want)
		}
	}
	s := Settings{Zoom: 9}
	s.normalize()
	if s.Zoom != 3 {
		t.Errorf("zoom 9 normalized to %v", s.Zoom)
	}
	s = Settings{}
	s.normalize()
	if s.Zoom != 1 {
		t.Errorf("no zoom normalized to %v", s.Zoom)
	}
}

// Zooming shows everything larger: the UI's text and room, the code, and
// the Markdown preview alike.
func TestZoomWindow(t *testing.T) {
	w, tt := openedReadmePull(t, `[]`)
	w.previewFile("README.md")
	tt.SetSize(1400, 900)
	tt.Frame()
	size := func(label string) (float32, float32) {
		t.Helper()
		tt.Frame()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q", label)
		}
		return r.W, r.H
	}
	filterW, filterH := size("Filter files")
	_, previewH := size("Markdown width")
	tabW, _ := size("Changes")

	w.settings.Zoom = 1.5
	tt.Frame()
	tt.Frame()
	snapshot(t, tt, "zoom-150")
	// The window shows everything 1.5 times larger.
	near := func(got, want float32) bool { return got > want*0.99-0.5 && got < want*1.01+0.5 }
	if fw, fh := size("Filter files"); !near(fw, filterW*1.5) || !near(fh, filterH*1.5) {
		t.Errorf("the filter is %v×%v zoomed, %v×%v at 100%%", fw, fh, filterW, filterH)
	}
	if got, _ := size("Changes"); !near(got, tabW*1.5) {
		t.Errorf("the Changes tab is %v wide zoomed, %v at 100%%", got, tabW)
	}
	if _, got := size("Markdown width"); !near(got, previewH*1.5) {
		t.Errorf("the preview's width buttons are %v tall zoomed, %v at 100%%", got, previewH)
	}
	if w.settings.Zoom != 1.5 {
		t.Fatal("zoom reset")
	}
}
