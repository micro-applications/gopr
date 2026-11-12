package mermaid

import (
	"bytes"
	"regexp"
	"strconv"
)

var (
	rootTag      = regexp.MustCompile(`^<svg\b[^>]*>`)
	sizeAttr     = regexp.MustCompile(`\s(viewBox|width|height|preserveAspectRatio)="[^"]*"`)
	viewBoxValue = regexp.MustCompile(`\sviewBox="([^"]*)"`)
)

// Crop returns the shapes of the part of the drawing at x, y, w×h in its
// units, drawn pw×ph: a tile of a drawing too large to draw at once.
func (d *Diagram) Crop(x, y, w, h, pw, ph float64) []byte {
	root := rootTag.Find(d.Shapes)
	if root == nil {
		return d.Shapes
	}
	// The drawing's own origin, which Pele leaves at 0 0.
	var ox, oy float64
	if m := viewBoxValue.FindSubmatch(root); m != nil {
		if v := numbers(string(m[1])); len(v) == 4 {
			ox, oy = v[0], v[1]
		}
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	tag := sizeAttr.ReplaceAll(root, nil)
	tag = append(tag[:len(tag)-1:len(tag)-1], []byte(` viewBox="`+f(ox+x)+` `+f(oy+y)+` `+f(w)+` `+f(h)+
		`" width="`+f(pw)+`" height="`+f(ph)+`" preserveAspectRatio="none">`)...)
	return bytes.Join([][]byte{tag, d.Shapes[len(root):]}, nil)
}
