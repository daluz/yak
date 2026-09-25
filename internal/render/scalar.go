package render

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// floatSpelling names the three floats that are not written as digits. JSON
// has no spelling for them at all, which the zero value stands for.
type floatSpelling struct {
	nan    string
	inf    string
	negInf string
}

var (
	yamlFloats = floatSpelling{nan: ".nan", inf: ".inf", negInf: "-.inf"}
	tomlFloats = floatSpelling{nan: "nan", inf: "inf", negInf: "-inf"}
)

// formatFloat spells f for a format whose non-finite floats are named by s.
func formatFloat(f float64, s floatSpelling) (string, error) {
	var special string
	switch {
	case math.IsNaN(f):
		special = s.nan
	case math.IsInf(f, 1):
		special = s.inf
	case math.IsInf(f, -1):
		special = s.negInf
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		if special == "" {
			return "", fmt.Errorf("cannot write %s, which has no spelling in this format", strconv.FormatFloat(f, 'g', -1, 64))
		}
		return special, nil
	}

	out := strconv.FormatFloat(f, 'g', -1, 64)
	// A float that formats without a decimal point would read back as an
	// integer, so keep it unambiguous.
	if !strings.ContainsAny(out, ".eE") {
		out += ".0"
	}
	return out, nil
}

// stringWriter is the part of bytes.Buffer and strings.Builder that escaping
// needs.
type stringWriter interface {
	WriteString(string) (int, error)
	WriteByte(byte) error
	WriteRune(rune) (int, error)
}

// escapeInto writes s as the body of a double-quoted string, without the
// quotes. It uses only the escapes that JSON, YAML and TOML all understand,
// so one routine serves every format here.
func escapeInto(w stringWriter, s string) {
	for i, r := range s {
		switch r {
		case '"':
			w.WriteString(`\"`)
		case '\\':
			w.WriteString(`\\`)
		case '\b':
			w.WriteString(`\b`)
		case '\f':
			w.WriteString(`\f`)
		case '\n':
			w.WriteString(`\n`)
		case '\r':
			w.WriteString(`\r`)
		case '\t':
			w.WriteString(`\t`)
		case utf8.RuneError:
			// A decoding failure is one byte wide; a real U+FFFD is not.
			if _, width := utf8.DecodeRuneInString(s[i:]); width == 1 {
				w.WriteString(`\ufffd`)
				continue
			}
			w.WriteRune(r)
		default:
			// YAML reads NEL and the Unicode line separators as line
			// breaks, so they cannot be left bare either.
			if r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
				const hex = "0123456789abcdef"
				w.WriteString(`\u`)
				for shift := 12; shift >= 0; shift -= 4 {
					w.WriteByte(hex[(r>>shift)&0xf])
				}
				continue
			}
			w.WriteRune(r)
		}
	}
}

// quote returns s as a double-quoted string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	escapeInto(&b, s)
	b.WriteByte('"')
	return b.String()
}
