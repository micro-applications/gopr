package mermaid

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

var samples = map[string]string{
	"flowchart": "flowchart LR\n  A[Start] --> B{Is it?}\n  B -->|Yes| C[OK]\n  B -.->|No| D((End))\n  subgraph S [Group]\n  C --> E[(DB)]\n  end",
	"sequence":  "sequenceDiagram\n  Alice->>Bob: Hello\n  Bob-->>Alice: Hi back\n  Note over Alice,Bob: a note",
	"class":     "classDiagram\n  Animal <|-- Duck\n  Animal : +int age\n  class Duck{\n    +swim()\n  }",
	"state":     "stateDiagram-v2\n  [*] --> Still\n  Still --> Moving\n  Moving --> [*]",
	"er":        "erDiagram\n  CUSTOMER ||--o{ ORDER : places",
	"gantt":     "gantt\n  title Plan\n  dateFormat YYYY-MM-DD\n  section A\n  Task 1 :a1, 2026-01-01, 30d",
	"pie":       "pie title Pets\n  \"Dogs\" : 386\n  \"Cats\" : 85",
	"mindmap":   "mindmap\n  root((GoPR))\n    Inbox\n    Tabs",
	"git":       "gitGraph\n  commit\n  branch dev\n  commit\n  checkout main\n  merge dev",
}

// texts are the texts of a diagram's labels.
func texts(d *Diagram) []string {
	var out []string
	for _, l := range d.Labels {
		out = append(out, l.Text())
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if strings.TrimSpace(x) == s {
			return true
		}
	}
	return false
}

func TestRenderTypes(t *testing.T) {
	for name, src := range samples {
		t.Run(name, func(t *testing.T) {
			d, err := Render(context.Background(), src, Light, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			shapes := string(d.Shapes)
			if strings.Contains(shapes, "<text") || strings.Contains(shapes, "<tspan") {
				t.Error("the shapes keep text")
			}
			if strings.Contains(shapes, "var(") || strings.Contains(shapes, "color-mix(") {
				t.Error("the shapes keep CSS they cannot draw")
			}
			if _, err := ui.ParseSVG(d.Shapes); err != nil {
				t.Errorf("MyGo cannot read the shapes: %v", err)
			}
			if d.Width <= 0 || d.Height <= 0 || len(d.Labels) == 0 {
				t.Fatalf("%v×%v, %d labels", d.Width, d.Height, len(d.Labels))
			}
			for _, l := range d.Labels {
				if l.X < -d.Width || l.X > 2*d.Width || l.Y < -d.Height || l.Y > 2*d.Height {
					t.Errorf("label %q at %v,%v, outside the %v×%v drawing", l.Text(), l.X, l.Y, d.Width, d.Height)
				}
				for _, r := range l.Runs {
					if !strings.HasPrefix(r.Fill, "#") || r.Size <= 0 {
						t.Errorf("label %q run %+v", l.Text(), r)
					}
				}
			}
		})
	}
}

func TestRenderFlowchart(t *testing.T) {
	d, err := Render(context.Background(), samples["flowchart"], Light, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := texts(d)
	for _, want := range []string{"Start", "Is it?", "OK", "End", "Group", "Yes", "No", "DB"} {
		if !has(got, want) {
			t.Errorf("no label %q in %q", want, got)
		}
	}
	for _, l := range d.Labels {
		if l.Text() == "Start" {
			if l.Anchor != Middle || l.Runs[0].Fill != Light["pele-fg"] {
				t.Errorf("Start: anchor %v, fill %s, want middle in %s", l.Anchor, l.Runs[0].Fill, Light["pele-fg"])
			}
		}
	}
	// The theme colors the shapes.
	if !strings.Contains(string(d.Shapes), Light["pele-line"]) {
		t.Error("the edges are not in the theme's line color")
	}
	dark, err := Render(context.Background(), samples["flowchart"], Dark, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dark.Shapes), Dark["pele-line"]) || strings.Contains(string(dark.Shapes), Light["pele-line"]) {
		t.Error("the dark theme does not color the edges")
	}
}

func TestRenderMeasurer(t *testing.T) {
	narrow, err := Render(context.Background(), "flowchart LR\n  A[A long label here]", Light, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	broad, err := Render(context.Background(), "flowchart LR\n  A[A long label here]", Light, measureFunc(func(s string, size float64, _ int) float64 {
		return float64(len([]rune(s))) * size
	}), 0)
	if err != nil {
		t.Fatal(err)
	}
	if broad.Width <= narrow.Width {
		t.Errorf("a wider measure lays out %v wide, %v with Arial's", broad.Width, narrow.Width)
	}
}

// measureFunc is a Measurer of a function.
type measureFunc func(s string, size float64, style int) float64

func (f measureFunc) Width(s string, size float64, style Style) float64 {
	return f(s, size, int(style))
}

func TestRenderError(t *testing.T) {
	if _, err := Render(context.Background(), "flowchart LR\n  A[unclosed --> ", Light, nil, 0); err == nil {
		t.Error("a broken diagram renders")
	}
	if _, err := Render(context.Background(), "not a diagram", Light, nil, 0); err == nil {
		t.Error("text that is no diagram renders")
	}
}

func TestConvertTransformsAndRuns(t *testing.T) {
	svg := `<svg viewBox="0 0 100 100" width="100" height="100" style="font:12px sans-serif;color:#112233" fill="#112233">` +
		`<g transform="translate(10,20)"><rect width="5" height="5" style="rx:2;fill:#abcdef"/>` +
		`<text x="5" y="6" text-anchor="end">Plain <tspan font-weight="bold" fill="#ff0000">bold</tspan></text>` +
		`<text class="t" xml:space="preserve" font-weight="600"><tspan x="1" y="2">one</tspan><tspan x="1" y="20">two</tspan></text>` +
		`<text x="0" y="0" transform="rotate(-45 0 0)" font-size="2em">turned</text>` +
		`<text x="0" y="0" fill="none">hidden</text></g></svg>`
	d, err := convert(svg, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(d); len(got) != 4 || got[0] != "Plain bold" || got[1] != "one" || got[2] != "two" || got[3] != "turned" {
		t.Fatalf("labels %q", got)
	}
	plain := d.Labels[0]
	if plain.X != 15 || plain.Y != 26 || plain.Anchor != End {
		t.Errorf("plain at %v,%v anchor %v", plain.X, plain.Y, plain.Anchor)
	}
	if r := plain.Runs[0]; r.Weight != 400 || r.Fill != "#112233" || r.Size != 12 {
		t.Errorf("plain run %+v", r)
	}
	if r := plain.Runs[1]; r.Weight != 700 || r.Fill != "#ff0000" {
		t.Errorf("bold run %+v", r)
	}
	if two := d.Labels[2]; two.X != 11 || two.Y != 40 || two.Runs[0].Weight != 600 {
		t.Errorf("two at %v,%v weight %d", two.X, two.Y, two.Runs[0].Weight)
	}
	turned := d.Labels[3]
	if math.Abs(turned.Angle+45) > 1e-9 || turned.X != 10 || turned.Y != 20 || turned.Runs[0].Size != 24 {
		t.Errorf("turned at %v,%v angle %v size %v", turned.X, turned.Y, turned.Angle, turned.Runs[0].Size)
	}
	shapes := string(d.Shapes)
	if !strings.Contains(shapes, `rx="2"`) || !strings.Contains(shapes, `fill="#abcdef"`) || strings.Contains(shapes, "style=") {
		t.Errorf("styles not made attributes: %s", shapes)
	}
	if strings.Contains(shapes, "hidden") || strings.Contains(shapes, "turned") {
		t.Errorf("text left in the shapes: %s", shapes)
	}
}

func TestResolveVars(t *testing.T) {
	svg := `<svg style="--_a:var(--pele-x,#000);--_b:var(--_a)" fill="var(--_b)"><path stroke="color-mix(in srgb,var(--_a) 50%,#ffffff)"/>var(--_a)</svg>`
	out, err := resolveVars(svg, Theme{"pele-x": "#ff0000"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `fill="#ff0000"`) || !strings.Contains(out, `stroke="#ff8080"`) {
		t.Errorf("not resolved: %s", out)
	}
	if !strings.Contains(out, ">var(--_a)<") {
		t.Errorf("text between tags changed: %s", out)
	}
	// Variables that refer to each other fall back.
	loop := `<svg style="--a:var(--b,#111);--b:var(--a,#222)" fill="var(--a)"></svg>`
	if out, err := resolveVars(loop, nil); err != nil || !strings.Contains(out, `fill="#`) {
		t.Errorf("loop: %s, %v", out, err)
	}
}
