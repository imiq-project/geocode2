package normalize

import (
	"strings"
	"unicode"
)

func Text(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
			}
			lastSpace = true
			continue
		}
		lastSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func SearchText(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func RemoveEmptyStrings(input []string) []string {
	result := make([]string, 0, len(input))

	for _, s := range input {
		if strings.TrimSpace(s) != "" {
			result = append(result, s)
		}
	}

	return result
}
