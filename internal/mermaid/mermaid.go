// Package mermaid lays Mermaid diagrams out with pele-go, a pure Go port
// of Pele, and splits its SVG into what MyGo draws: the shapes, as SVG
// without text, and the labels, to draw as text over them.
package mermaid

import (
	"context"
	"errors"
	"time"

	pele "github.com/Broderick-Westrope/pele-go"
	_ "github.com/Broderick-Westrope/pele-go/all" // every diagram type
)

// Measurer measures the advance width of text in pixels, at a size, bold,
// italic or monospaced as style says (pele.Bold, pele.Italic, pele.Mono).
// It must be safe for concurrent use.
type Measurer = pele.TextMeasurer

// Style is the emphasis a Measurer measures text in.
type Style = pele.Style

// The style bits a Measurer gets.
const (
	Bold   = pele.Bold
	Italic = pele.Italic
	Mono   = pele.Mono
)

// Theme is the colors a diagram is drawn in, Pele's CSS variables without
// their leading --, as "pele-fg": "#1f1f1f".
type Theme map[string]string

// Light and Dark are the themes of light and dark appearances.
var (
	Light = Theme{
		"pele-bg": "#ffffff", "pele-fg": "#1f2328", "pele-muted": "#656d76",
		"pele-line": "#57606a", "pele-surface": "#f6f8fa", "pele-surface-alt": "#eaeef2",
		"pele-border": "#8c959f", "pele-accent": "#0969da",
		"pele-critical": "#cf222e", "pele-error": "#cf222e",
	}
	Dark = Theme{
		"pele-bg": "#0d1117", "pele-fg": "#e6edf3", "pele-muted": "#8d96a0",
		"pele-line": "#8d96a0", "pele-surface": "#161b22", "pele-surface-alt": "#21262d",
		"pele-border": "#484f58", "pele-accent": "#4493f8",
		"pele-critical": "#f85149", "pele-error": "#f85149",
	}
)

// Timeout is the longest a diagram is laid out for.
const Timeout = 5 * time.Second

// ErrTimeout is returned when a diagram takes longer than Timeout.
var ErrTimeout = errors.New("mermaid: the diagram took too long to lay out")

// Render lays out src, Mermaid source, and splits it for MyGo. measure
// measures the labels, as they will be drawn, so that their boxes fit
// them; nil measures them as Arial. fontSize is the size of labels in
// pixels, 0 for Pele's 16. Links and images are left out: the source may
// be anyone's.
func Render(ctx context.Context, src string, theme Theme, measure Measurer, fontSize float64) (*Diagram, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	opts := pele.Options{
		Measurer:   measure,
		Responsive: pele.Ptr(false),
		Links:      pele.Ptr(false),
		Images:     pele.Ptr(false),
	}
	if fontSize > 0 {
		opts.FontSize = pele.Ptr(fontSize)
	}
	res, err := pele.Render(ctx, src, opts)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, err
	}
	svg, err := resolveVars(res.SVG, theme)
	if err != nil {
		return nil, err
	}
	return convert(svg, res.Width, res.Height)
}
