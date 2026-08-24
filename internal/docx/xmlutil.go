package docx

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	twipsPerInch = 1440
	emuPerTwip   = 635
	emuPerPixel  = 9525

	// A4 portrait with 20mm margins, matching the HTML/PDF export stylesheet.
	pageWidthTwips   = 11906
	pageHeightTwips  = 16838
	pageMarginTwips  = 1134
	contentWidthTwip = pageWidthTwips - 2*pageMarginTwips
	contentWidthEMU  = contentWidthTwip * emuPerTwip

	listIndentTwips  = 720
	listHangingTwips = 360
)

func escapeXML(value string) string {
	var out strings.Builder
	out.Grow(len(value) + 8)
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		case '\'':
			out.WriteString("&apos;")
		case '\t', '\n', '\r':
			out.WriteRune(r)
		default:
			// XML 1.0 forbids most control characters outright; dropping them
			// keeps Word from rejecting the package as corrupt.
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0xfffe || r == 0xffff {
				continue
			}
			out.WriteRune(r)
		}
	}
	return out.String()
}

func attr(name, value string) string {
	return ` ` + name + `="` + escapeXML(value) + `"`
}

func intAttr(name string, value int) string {
	return ` ` + name + `="` + strconv.Itoa(value) + `"`
}

// hexColor normalises CSS colour syntax into the RRGGBB form OOXML expects.
func hexColor(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "inherit" || value == "transparent" || value == "currentcolor" {
		return ""
	}
	if named, ok := namedColors[value]; ok {
		return named
	}
	if strings.HasPrefix(value, "#") {
		digits := value[1:]
		switch len(digits) {
		case 3:
			return strings.ToUpper(string([]byte{digits[0], digits[0], digits[1], digits[1], digits[2], digits[2]}))
		case 6, 8:
			if _, err := strconv.ParseUint(digits[:6], 16, 32); err == nil {
				return strings.ToUpper(digits[:6])
			}
		}
		return ""
	}
	if strings.HasPrefix(value, "rgb") {
		open := strings.Index(value, "(")
		close := strings.LastIndex(value, ")")
		if open < 0 || close < open {
			return ""
		}
		parts := strings.Split(value[open+1:close], ",")
		if len(parts) < 3 {
			return ""
		}
		channels := make([]int, 3)
		for i := 0; i < 3; i++ {
			field := strings.TrimSpace(parts[i])
			if strings.HasSuffix(field, "%") {
				percent, err := strconv.ParseFloat(strings.TrimSuffix(field, "%"), 64)
				if err != nil {
					return ""
				}
				channels[i] = int(percent*255/100 + 0.5)
			} else {
				number, err := strconv.ParseFloat(field, 64)
				if err != nil {
					return ""
				}
				channels[i] = int(number + 0.5)
			}
			if channels[i] < 0 {
				channels[i] = 0
			}
			if channels[i] > 255 {
				channels[i] = 255
			}
		}
		return strings.ToUpper(fmt.Sprintf("%02x%02x%02x", channels[0], channels[1], channels[2]))
	}
	return ""
}

var namedColors = map[string]string{
	"black": "000000", "silver": "C0C0C0", "gray": "808080", "grey": "808080",
	"white": "FFFFFF", "maroon": "800000", "red": "FF0000", "purple": "800080",
	"fuchsia": "FF00FF", "green": "008000", "lime": "00FF00", "olive": "808000",
	"yellow": "FFFF00", "navy": "000080", "blue": "0000FF", "teal": "008080",
	"aqua": "00FFFF", "cyan": "00FFFF", "magenta": "FF00FF", "orange": "FFA500",
	"pink": "FFC0CB", "brown": "A52A2A", "gold": "FFD700", "violet": "EE82EE",
	"indigo": "4B0082", "beige": "F5F5DC", "ivory": "FFFFF0", "khaki": "F0E68C",
	"salmon": "FA8072", "tomato": "FF6347", "coral": "FF7F50", "crimson": "DC143C",
	"lavender": "E6E6FA", "plum": "DDA0DD", "orchid": "DA70D6", "turquoise": "40E0D0",
}

// cssFontSizeHalfPoints converts a CSS font-size into OOXML half-points.
func cssFontSizeHalfPoints(value string) int {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0
	}
	unit := ""
	for _, suffix := range []string{"px", "pt", "rem", "em", "%"} {
		if strings.HasSuffix(value, suffix) {
			unit = suffix
			value = strings.TrimSuffix(value, suffix)
			break
		}
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || number <= 0 {
		return 0
	}
	var points float64
	switch unit {
	case "pt":
		points = number
	case "px", "":
		points = number * 0.75
	case "rem", "em":
		points = number * 11
	case "%":
		points = 11 * number / 100
	}
	halfPoints := int(points*2 + 0.5)
	if halfPoints < 2 {
		return 0
	}
	if halfPoints > 800 {
		halfPoints = 800
	}
	return halfPoints
}

// cssFontFamily picks the first concrete family name from a CSS font stack.
func cssFontFamily(value string) string {
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		name = strings.Trim(name, `"'`)
		switch strings.ToLower(name) {
		case "", "inherit", "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui":
			continue
		}
		return name
	}
	return ""
}
