package mermaid

import (
	"math"
	"strconv"
	"strings"
)

// matrix is an affine transform, as SVG's matrix(a b c d e f).
type matrix struct{ a, b, c, d, e, f float64 }

var identity = matrix{a: 1, d: 1}

// mul is m then n applied first: the transform of a child, n, within m.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// angle is how far m turns, in degrees.
func (m matrix) angle() float64 {
	deg := math.Atan2(m.b, m.a) * 180 / math.Pi
	if math.Abs(deg) < 1e-9 {
		return 0
	}
	return deg
}

// parseTransform reads a transform attribute: translate, rotate, scale,
// skewX, skewY and matrix, in order.
func parseTransform(s string) matrix {
	m := identity
	for {
		open := strings.IndexByte(s, '(')
		if open < 0 {
			return m
		}
		end := strings.IndexByte(s[open:], ')')
		if end < 0 {
			return m
		}
		name := strings.TrimSpace(strings.Trim(s[:open], " ,\t\n"))
		args := numbers(s[open+1 : open+end])
		s = s[open+end+1:]
		arg := func(i int, def float64) float64 {
			if i < len(args) {
				return args[i]
			}
			return def
		}
		var n matrix
		switch name {
		case "translate":
			n = matrix{a: 1, d: 1, e: arg(0, 0), f: arg(1, 0)}
		case "scale":
			sx := arg(0, 1)
			n = matrix{a: sx, d: arg(1, sx)}
		case "rotate":
			r := arg(0, 0) * math.Pi / 180
			cos, sin := math.Cos(r), math.Sin(r)
			cx, cy := arg(1, 0), arg(2, 0)
			n = matrix{a: 1, d: 1, e: cx, f: cy}.
				mul(matrix{a: cos, b: sin, c: -sin, d: cos}).
				mul(matrix{a: 1, d: 1, e: -cx, f: -cy})
		case "skewX":
			n = matrix{a: 1, c: math.Tan(arg(0, 0) * math.Pi / 180), d: 1}
		case "skewY":
			n = matrix{a: 1, b: math.Tan(arg(0, 0) * math.Pi / 180), d: 1}
		case "matrix":
			if len(args) != 6 {
				continue
			}
			n = matrix{args[0], args[1], args[2], args[3], args[4], args[5]}
		default:
			continue
		}
		m = m.mul(n)
	}
}

func numbers(s string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		if v, err := strconv.ParseFloat(f, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}
