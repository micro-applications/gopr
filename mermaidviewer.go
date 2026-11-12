package main

import (
	"fmt"
	"math"
	"runtime"

	"github.com/egoist/mygo/ui"
)

// The zoom of the diagram viewer: its bounds, and the step of a zoom in or
// out.
const (
	minMermaidZoom  = 0.25
	maxMermaidZoom  = 6
	mermaidZoomStep = 1.25
)

// mermaidViewer is the diagram shown over the window, to zoom and pan.
type mermaidViewer struct {
	m *mermaidDiagram
	// zoom is the scale shown; fit, set, has it fit the window, as it
	// opens and as the window resizes.
	zoom   float32
	fit    bool
	scroll ui.ScrollState
	// view is the size of the scrolling area, as the last frame laid it.
	view ui.Rect
}

// openMermaid shows a diagram in the viewer, fitting the window.
func (w *window) openMermaid(m *mermaidDiagram) {
	w.viewer = &mermaidViewer{m: m, zoom: 1, fit: true}
}

// fitZoom is the zoom that shows the whole diagram in the view.
func (v *mermaidViewer) fitZoom() float32 {
	if v.view.W <= 0 || v.view.H <= 0 {
		return 1
	}
	const pad = 48
	z := min((v.view.W-pad)/float32(v.m.d.Width), (v.view.H-pad)/float32(v.m.d.Height))
	return clampZoom(z)
}

func clampZoom(z float32) float32 {
	return min(max(z, minMermaidZoom), maxMermaidZoom)
}

// setZoom zooms to z, keeping the point at the view's center there.
func (v *mermaidViewer) setZoom(z float32) {
	z = clampZoom(z)
	if z == v.zoom {
		return
	}
	cx := (v.scroll.X + v.view.W/2) / v.zoom
	cy := (v.scroll.Y + v.view.H/2) / v.zoom
	v.zoom, v.fit = z, false
	v.scroll.X = max(cx*z-v.view.W/2, 0)
	v.scroll.Y = max(cy*z-v.view.H/2, 0)
}

// zoomBy zooms in (1) or out (-1) a step.
func (v *mermaidViewer) zoomBy(dir int) {
	v.setZoom(v.zoom * float32(math.Pow(mermaidZoomStep, float64(dir))))
}

// mermaidViewer shows the viewer, when a diagram is open, over the window.
func (w *window) mermaidViewerView(c *ui.Context, pal *palette) {
	v := w.viewer
	if v == nil {
		return
	}
	if v.fit {
		v.zoom = v.fitZoom()
	}
	t := c.Theme()
	ui.Overlay(c, func() {
		panel := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(pal.appBg).Label("Diagram viewer")
		panel.Children(func() {
			left := float32(12)
			if runtime.GOOS == "darwin" {
				left = max(c.TitleBar().Left+8, 12)
			}
			ui.Row(c).Height(titleBarHeight).Padding(0, 12, 0, left).Gap(6).AlignItems(ui.Center).Shrink(0).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.cardBorder).DragWindow().Children(func() {
				ui.Text(c, "Diagram").FontSize(13).FontWeight(600)
				ui.Spacer(c)
				if iconButton(c, iconZoomOut, "Zoom Out (−)").Size(28, 26).Label("Zoom Out").Clicked() {
					v.zoomBy(-1)
				}
				ui.Text(c, fmt.Sprintf("%d%%", int(math.Round(float64(v.zoom*100))))).FontSize(12).
					Font(w.codeFont()).Width(48).TextAlign(ui.Center).TextColor(t.TextMuted).Label("Zoom")
				if iconButton(c, iconZoomIn, "Zoom In (+)").Size(28, 26).Label("Zoom In").Clicked() {
					v.zoomBy(1)
				}
				if ui.Button(c, "Fit").Height(26).Tooltip("Fit the window (F)").Clicked() {
					v.fit = true
				}
				if ui.Button(c, "100%").Height(26).Tooltip("Actual size (0)").Clicked() {
					v.setZoom(1)
				}
				if iconButton(c, iconClose, "Close (Esc)").Size(28, 26).Label("Close Diagram").Clicked() {
					w.viewer = nil
				}
			})
			if w.viewer == nil {
				return
			}
			width, height := float32(v.m.d.Width)*v.zoom, float32(v.m.d.Height)*v.zoom
			body := ui.ScrollBoth(c).Grow(1).MinHeight(0).TrackScroll(&v.scroll).Background(pal.codeBg)
			body.Children(func() {
				// The diagram stays centered while smaller than the view.
				area := ui.Box(c).Size(max(width, v.view.W), max(height, v.view.H)).Cursor(ui.CursorGrab).Label("Diagram canvas")
				if area.Dragging() {
					area.Cursor(ui.CursorGrabbing)
				}
				if dx, dy, ok := area.Dragged(); ok {
					v.scroll.X -= dx
					v.scroll.Y -= dy
				}
				area.Draw(func(p *ui.Painter, r ui.Rect) {
					x := r.X + max(r.W-width, 0)/2
					y := r.Y + max(r.H-height, 0)/2
					drawMermaid(p, v.m, ui.Rect{X: x, Y: y, W: width, H: height})
				})
			})
			if b := body.Bounds(); b.W > 0 && (b.W != v.view.W || b.H != v.view.H) {
				v.view = b
				c.After(0)
			}
		})
		switch {
		case panel.OverlayShortcut(0, ui.KeyEscape):
			w.viewer = nil
		case panel.OverlayShortcut(0, ui.KeyEqual), panel.OverlayShortcut(ui.Shift, ui.KeyEqual):
			v.zoomBy(1)
		case panel.OverlayShortcut(0, ui.KeyMinus):
			v.zoomBy(-1)
		case panel.OverlayShortcut(0, ui.Key0):
			v.setZoom(1)
		case panel.OverlayShortcut(0, ui.KeyF):
			v.fit = true
		}
	})
}
