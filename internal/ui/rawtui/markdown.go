package rawtui

import (
	"strings"
	"unicode/utf8"
)

// ProcessMarkdownLine handles one line of assistant text, tracking code-block
// state across calls.
func ProcessMarkdownLine(line string, inCodeBlock bool) (result string, newState bool, skip bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "```") {
		return "", !inCodeBlock, true
	}
	if inCodeBlock {
		return "\033[48;5;236m\033[38;5;248m" + line + "\033[0m", true, false
	}
	if strings.HasPrefix(trimmed, "# ") {
		return "\033[1;4m" + trimmed[2:] + "\033[0m", false, false
	}
	if strings.HasPrefix(trimmed, "## ") {
		return "\033[1m" + trimmed[3:] + "\033[0m", false, false
	}
	if strings.HasPrefix(trimmed, "### ") {
		return "\033[1m" + trimmed[4:] + "\033[0m", false, false
	}
	if strings.HasPrefix(trimmed, "#### ") {
		return "\033[1m" + trimmed[5:] + "\033[0m", false, false
	}
	return renderInlineMarkdown(line), false, false
}

func renderInlineMarkdown(s string) string {
	var buf strings.Builder
	b := []byte(s)
	n := len(b)
	i := 0
	for i < n {
		if b[i] == '\033' {
			j := i + 1
			if j < n && b[j] == '[' {
				j++
				for j < n && b[j] != 'm' && !isCSIFinal(b[j]) {
					j++
				}
				if j < n {
					j++
				}
			} else if j < n && b[j] == ']' {
				j++
				for j+1 < n {
					if b[j] == '\033' && b[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
			}
			buf.Write(b[i:j])
			i = j
			continue
		}
		if b[i] == '`' {
			end := indexByte(b, '`', i+1)
			if end > i+1 {
				buf.WriteString("\033[7m")
				buf.Write(b[i+1 : end])
				buf.WriteString("\033[27m")
				i = end + 1
				continue
			}
		}
		if i+1 < n && b[i] == '*' && b[i+1] == '*' {
			end := indexDouble(b, '*', i+2)
			if end > i+2 {
				content := renderInlineMarkdown(string(b[i+2 : end]))
				buf.WriteString("\033[1m")
				buf.WriteString(content)
				buf.WriteString("\033[22m")
				i = end + 2
				continue
			}
		}
		if b[i] == '*' && (i == 0 || b[i-1] != '*') && i+1 < n && b[i+1] != '*' && b[i+1] != ' ' {
			end := indexSingleStar(b, i+1)
			if end > i+1 {
				content := renderInlineMarkdown(string(b[i+1 : end]))
				buf.WriteString("\033[3m")
				buf.WriteString(content)
				buf.WriteString("\033[23m")
				i = end + 1
				continue
			}
		}
		if i+1 < n && b[i] == '~' && b[i+1] == '~' {
			end := indexDouble(b, '~', i+2)
			if end > i+2 {
				content := renderInlineMarkdown(string(b[i+2 : end]))
				buf.WriteString("\033[9m")
				buf.WriteString(content)
				buf.WriteString("\033[29m")
				i = end + 2
				continue
			}
		}
		if b[i] == '[' {
			closeBracket := indexPair(b, ']', '(', i+1)
			if closeBracket > i+1 {
				closeParen := indexByte(b, ')', closeBracket+2)
				if closeParen > closeBracket+2 {
					text := string(b[i+1 : closeBracket])
					url := string(b[closeBracket+2 : closeParen])
					buf.WriteString("\033]8;;")
					buf.WriteString(url)
					buf.WriteString("\033\\")
					buf.WriteString("\033[4m")
					buf.WriteString(text)
					buf.WriteString("\033[24m")
					buf.WriteString("\033]8;;\033\\")
					i = closeParen + 1
					continue
				}
			}
		}
		r, size := utf8.DecodeRune(b[i:])
		buf.WriteRune(r)
		i += size
	}
	return buf.String()
}

func indexByte(b []byte, target byte, start int) int {
	for i := start; i < len(b); i++ {
		if b[i] == target {
			return i
		}
	}
	return -1
}

func indexPair(buf []byte, a, bChar byte, start int) int {
	for i := start; i+1 < len(buf); i++ {
		if buf[i] == a && buf[i+1] == bChar {
			return i
		}
	}
	return -1
}

func indexDouble(b []byte, target byte, start int) int {
	for i := start; i+1 < len(b); i++ {
		if b[i] == target && b[i+1] == target {
			return i
		}
	}
	return -1
}

func indexSingleStar(b []byte, start int) int {
	for i := start; i < len(b); i++ {
		if b[i] == '*' {
			if i+1 >= len(b) || b[i+1] != '*' {
				return i
			}
			i++
		}
	}
	return -1
}
