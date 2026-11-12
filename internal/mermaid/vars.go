// The resolution of CSS variables and color-mix() here is adapted from
// pele-go's raster module (github.com/Broderick-Westrope/pele-go/raster,
// vars.go and raster.go), MIT licensed: see LICENSE-pele-go.

package mermaid

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	errVarsTooDeep  = errors.New("mermaid: CSS variables nest too deep")
	errVarsTooLarge = errors.New("mermaid: CSS variables expand the drawing too much")
)

const (
	maxVarDepth  = 32
	minVarsLimit = 1 << 20
)

var rootStyle = regexp.MustCompile(`^<svg[^>]*?\sstyle="([^"]*)"`)

// resolveVars substitutes every CSS var() in svg's attribute values, then
// evaluates the color-mix() they leave: MyGo's SVG knows neither. Pele
// declares its variables in the root element's style alone; theme's
// declarations, named without their leading --, win over them. Text
// between tags is never touched, for labels may hold anything. The output
// is capped at a few times the input, so that chained variables cannot
// expand without bound.
func resolveVars(svg string, theme map[string]string) (string, error) {
	r := &varResolver{
		defs:      map[string]string{},
		resolved:  map[string]string{},
		resolving: map[string]bool{},
		limit:     max(4*len(svg), minVarsLimit),
	}
	if m := rootStyle.FindStringSubmatch(svg); m != nil {
		for _, decl := range splitTopLevel(m[1], ';') {
			name, value, ok := strings.Cut(decl, ":")
			name = strings.TrimSpace(name)
			if ok && strings.HasPrefix(name, "--") {
				r.defs[name[2:]] = strings.TrimSpace(value)
			}
		}
	}
	for name, value := range theme {
		r.defs[name] = value
	}

	var out strings.Builder
	inTag := false
	for svg != "" {
		// Every path consumes some of svg, so the loop always ends.
		if !inTag {
			i := strings.IndexByte(svg, '<')
			if i < 0 {
				out.WriteString(svg)
				return out.String(), nil
			}
			out.WriteString(svg[:i+1])
			svg, inTag = svg[i+1:], true
			continue
		}
		i := strings.IndexAny(svg, `>"'`)
		if i < 0 {
			out.WriteString(svg)
			return out.String(), nil
		}
		out.WriteString(svg[:i+1])
		quote := svg[i]
		svg = svg[i+1:]
		if quote == '>' {
			inTag = false
			continue
		}
		end := strings.IndexByte(svg, quote)
		if end < 0 {
			out.WriteString(svg)
			return out.String(), nil
		}
		value, err := r.substitute(svg[:end])
		if err != nil {
			return "", err
		}
		out.WriteString(resolveColorMix(value))
		out.WriteByte(quote)
		if out.Len() > r.limit {
			return "", errVarsTooLarge
		}
		svg = svg[end+1:]
	}
	return out.String(), nil
}

// splitTopLevel splits s at each sep outside parentheses.
func splitTopLevel(s string, sep rune) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// varResolver expands var() against a set of declarations, resolving each
// once. A declaration that refers back to itself, directly or not, falls
// back as if it were undeclared.
type varResolver struct {
	defs      map[string]string
	resolved  map[string]string
	resolving map[string]bool
	limit     int
}

func (r *varResolver) substitute(s string) (string, error) {
	var out strings.Builder
	for {
		idx := strings.Index(s, "var(")
		if idx < 0 {
			out.WriteString(s)
			return out.String(), nil
		}
		body, rest, ok := matchParen(s[idx+len("var("):])
		if !ok {
			out.WriteString(s)
			return out.String(), nil
		}
		out.WriteString(s[:idx])
		name, fallback, _ := strings.Cut(body, ",")
		value, err := r.lookup(strings.TrimPrefix(strings.TrimSpace(name), "--"), strings.TrimSpace(fallback))
		if err != nil {
			return "", err
		}
		out.WriteString(value)
		if out.Len() > r.limit {
			return "", errVarsTooLarge
		}
		s = rest
	}
}

func (r *varResolver) lookup(name, fallback string) (string, error) {
	if value, ok := r.resolved[name]; ok {
		return value, nil
	}
	raw, ok := r.defs[name]
	if !ok || r.resolving[name] {
		return r.substitute(fallback)
	}
	if len(r.resolving) >= maxVarDepth {
		return "", errVarsTooDeep
	}
	r.resolving[name] = true
	value, err := r.substitute(raw)
	delete(r.resolving, name)
	if err != nil {
		return "", err
	}
	r.resolved[name] = value
	return value, nil
}

// matchParen splits s at the ')' closing an already opened parenthesis.
func matchParen(s string) (body, rest string, ok bool) {
	depth := 1
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return "", "", false
}

var colorMix = regexp.MustCompile(`color-mix\(in srgb,\s*(#[0-9a-fA-F]{3,6})\s+([\d.]+)%\s*,\s*(#[0-9a-fA-F]{3,6})\s*\)`)

// resolveColorMix evaluates the srgb color-mix() Pele uses for nested
// cluster titles, in one attribute value.
func resolveColorMix(value string) string {
	if !strings.Contains(value, "color-mix(") {
		return value
	}
	return colorMix.ReplaceAllStringFunc(value, func(expr string) string {
		m := colorMix.FindStringSubmatch(expr)
		a, okA := parseHex(m[1])
		b, okB := parseHex(m[3])
		pct, err := strconv.ParseFloat(m[2], 64)
		if !okA || !okB || err != nil {
			return expr
		}
		w := pct / 100
		var out [3]uint8
		for i := range out {
			out[i] = uint8(math.Round(float64(a[i])*w + float64(b[i])*(1-w)))
		}
		return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
	})
}

// parseHex reads a #rgb or #rrggbb color.
func parseHex(s string) ([3]uint8, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return [3]uint8{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return [3]uint8{}, false
	}
	return [3]uint8{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}
