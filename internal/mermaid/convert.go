package mermaid

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Diagram is a drawing split for MyGo, whose SVG draws shapes but not
// text: Shapes is the drawing without its labels, which Labels hold, to
// draw as text over it, in the drawing's units.
type Diagram struct {
	Shapes        []byte
	Width, Height float64
	Labels        []Label
}

// Anchor is where a label's point is along it, as SVG's text-anchor.
type Anchor int

const (
	Start Anchor = iota
	Middle
	End
)

// Label is a line of text: its runs, from the point X, Y on its baseline,
// placed by Anchor, turned by Angle degrees around that point.
type Label struct {
	X, Y   float64
	Angle  float64
	Anchor Anchor
	Runs   []Run

	// localX and localY are the point before the transform, where a tspan
	// placed on one axis alone takes the other.
	localX, localY float64
}

// Run is text of one style in a label. Fill is its color as the drawing
// has it, as #rrggbb, never empty.
type Run struct {
	Text   string
	Size   float64
	Weight int
	Italic bool
	Mono   bool
	Fill   string
	// Opacity is that of the text and of its fill together, 1 for opaque.
	Opacity float64
}

// Text is all the label's text.
func (l Label) Text() string {
	var b strings.Builder
	for _, r := range l.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// textStyle is what text inherits from its ancestors.
type textStyle struct {
	fill    string
	opacity float64
	size    float64
	weight  int
	italic  bool
	mono    bool
	anchor  Anchor
	m       matrix
}

var rootFont = regexp.MustCompile(`font:\s*(\d+(?:\.\d+)?)px`)

// convert splits an SVG Pele drew, its variables resolved, into the
// shapes and the labels of a Diagram.
func convert(svg string, width, height float64) (*Diagram, error) {
	d := &Diagram{Width: width, Height: height}
	dec := xml.NewDecoder(strings.NewReader(svg))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var out bytes.Buffer
	stack := []textStyle{{fill: "#000000", opacity: 1, size: 16, weight: 400, m: identity}}
	// The text being read, and the label of it the text goes into.
	inText := 0
	var label *Label
	root := true
	current := "#000000" // the root's color, which currentColor names
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mermaid: reading the drawing: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			st := stack[len(stack)-1]
			attrs := attrMap(t.Attr)
			if root {
				if m := rootFont.FindStringSubmatch(attrs["style"]); m != nil {
					st.size, _ = strconv.ParseFloat(m[1], 64)
				}
				if c := colorOf(styleProps(attrs["style"])["color"]); c != "" {
					st.fill = c
				}
			}
			st = inherit(st, attrs)
			stack = append(stack, st)
			name := t.Name.Local
			if inText > 0 || name == "text" {
				inText++
				if name == "text" || attrs["x"] != "" || attrs["y"] != "" {
					// A text, or a tspan placed on its own, starts a label.
					x, y := num(attrs["x"]), num(attrs["y"])
					if label != nil && name == "tspan" {
						if attrs["x"] == "" {
							x = label.localX
						}
						if attrs["y"] == "" {
							y = label.localY
						}
					}
					d.flush(label)
					px, py := st.m.apply(x, y)
					label = &Label{X: px, Y: py, Angle: st.m.angle(), Anchor: st.anchor}
					label.localX, label.localY = x, y
				}
				continue
			}
			if root {
				current = st.fill
				if c := colorOf(styleProps(attrs["style"])["color"]); c != "" {
					current = c
				}
			}
			writeStart(&out, t, attrs, root, current)
			root = false
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			if inText > 0 {
				inText--
				if inText == 0 {
					d.flush(label)
					label = nil
				}
				continue
			}
			out.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			if inText > 0 {
				if label != nil && len(t) > 0 {
					st := stack[len(stack)-1]
					label.Runs = append(label.Runs, Run{Text: string(t), Size: st.size, Weight: st.weight,
						Italic: st.italic, Mono: st.mono, Fill: st.fill, Opacity: st.opacity})
				}
				continue
			}
			xml.EscapeText(&out, t)
		}
	}
	if root {
		return nil, errors.New("mermaid: the drawing is empty")
	}
	d.Shapes = out.Bytes()
	return d, nil
}

// flush keeps a label read, when it has visible text.
func (d *Diagram) flush(l *Label) {
	if l == nil {
		return
	}
	runs := l.Runs[:0]
	for _, r := range l.Runs {
		if r.Text != "" && r.Fill != "" && r.Opacity > 0 {
			runs = append(runs, r)
		}
	}
	l.Runs = runs
	if strings.TrimSpace(l.Text()) != "" {
		d.Labels = append(d.Labels, *l)
	}
}

// inherit applies an element's attributes, and its style's, to what its
// text inherits.
func inherit(st textStyle, attrs map[string]string) textStyle {
	props := styleProps(attrs["style"])
	get := func(name string) string {
		if v, ok := props[name]; ok {
			return v
		}
		return attrs[name]
	}
	if t := attrs["transform"]; t != "" {
		st.m = st.m.mul(parseTransform(t))
	}
	switch f := strings.TrimSpace(get("fill")); f {
	case "":
	case "none", "transparent":
		st.fill = ""
	default:
		if c := colorOf(f); c != "" {
			st.fill = c
		}
	}
	for _, name := range []string{"opacity", "fill-opacity"} {
		if v := get(name); v != "" {
			if o, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				st.opacity *= math.Max(0, math.Min(1, o))
			}
		}
	}
	if v := get("font-size"); v != "" {
		if size, ok := fontSize(v, st.size); ok {
			st.size = size
		}
	}
	if v := strings.TrimSpace(get("font-weight")); v != "" {
		switch v {
		case "normal":
			st.weight = 400
		case "bold", "bolder":
			st.weight = 700
		case "lighter":
			st.weight = 300
		default:
			if w, err := strconv.Atoi(v); err == nil {
				st.weight = w
			}
		}
	}
	if v := get("font-style"); v != "" {
		st.italic = strings.Contains(v, "italic") || strings.Contains(v, "oblique")
	}
	if v := get("font-family"); v != "" {
		st.mono = strings.Contains(strings.ToLower(v), "mono")
	}
	switch get("text-anchor") {
	case "start":
		st.anchor = Start
	case "middle":
		st.anchor = Middle
	case "end":
		st.anchor = End
	}
	return st
}

// fontSize reads a font size, in pixels or relative to the size around it.
func fontSize(v string, around float64) (float64, bool) {
	v = strings.TrimSpace(v)
	switch v {
	case "smaller":
		return around / 1.2, true
	case "larger":
		return around * 1.2, true
	}
	scale := 1.0
	switch {
	case strings.HasSuffix(v, "px"):
		v = strings.TrimSuffix(v, "px")
	case strings.HasSuffix(v, "em"):
		v, scale = strings.TrimSuffix(v, "em"), around
	case strings.HasSuffix(v, "%"):
		v, scale = strings.TrimSuffix(v, "%"), around/100
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return f * scale, true
}

// presentation are the CSS properties of an element's style that MyGo's
// SVG takes as attributes.
var presentation = map[string]bool{
	"rx": true, "ry": true, "fill": true, "fill-opacity": true, "fill-rule": true,
	"stroke": true, "stroke-width": true, "stroke-opacity": true, "stroke-dasharray": true,
	"stroke-dashoffset": true, "stroke-linecap": true, "stroke-linejoin": true,
	"stroke-miterlimit": true, "opacity": true, "display": true, "visibility": true,
}

// writeStart writes an element's start tag, its style's presentation
// properties made attributes, and currentColor the root's color.
func writeStart(out *bytes.Buffer, t xml.StartElement, attrs map[string]string, root bool, current string) {
	out.WriteString("<" + t.Name.Local)
	write := func(name, value string) {
		out.WriteString(" " + name + `="`)
		xml.EscapeText(out, []byte(value))
		out.WriteString(`"`)
	}
	props := styleProps(attrs["style"])
	for _, a := range t.Attr {
		name := a.Name.Local
		if a.Name.Space == "xmlns" || name == "xmlns" {
			continue
		}
		if a.Name.Space != "" && a.Name.Space != "http://www.w3.org/2000/svg" {
			continue // xml:space, aria and the like
		}
		switch {
		case name == "style", name == "class", name == "role", strings.HasPrefix(name, "data-"), strings.HasPrefix(name, "aria-"):
			continue
		case presentation[name] && props[name] != "":
			continue // the style wins
		}
		value := a.Value
		if strings.EqualFold(strings.TrimSpace(value), "currentColor") {
			value = current
		}
		write(name, value)
	}
	for _, name := range sortedKeys(props) {
		if presentation[name] && !root {
			value := props[name]
			if strings.EqualFold(value, "currentColor") {
				value = current
			}
			write(name, value)
		}
	}
	if root {
		out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
	}
	out.WriteString(">")
}

// styleProps reads the declarations of a style attribute.
func styleProps(style string) map[string]string {
	props := map[string]string{}
	for _, decl := range splitTopLevel(style, ';') {
		name, value, ok := strings.Cut(decl, ":")
		if ok {
			props[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return props
}

func attrMap(attrs []xml.Attr) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Name.Local] = a.Value
	}
	return m
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// colorOf reads a color as #rrggbb, "" for one it cannot read.
func colorOf(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := parseHex(s); ok && strings.HasPrefix(s, "#") {
		return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2])
	}
	switch s {
	case "black":
		return "#000000"
	case "white":
		return "#ffffff"
	}
	return ""
}
