package uib

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

// loadFiles walks a directory tree up to maxDepth and returns relative file paths.
func loadFiles(dir string, maxDepth int) []string {
	var files []string
	baseDepth := strings.Count(filepath.Clean(dir), string(filepath.Separator))
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			depth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - baseDepth
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, rel)
		return nil
	})
	return files
}

// filterFiles returns files matching a case-insensitive query substring.
func filterFiles(files []string, query string) []string {
	if query == "" {
		return files
	}
	q := strings.ToLower(query)
	var matches []string
	for _, f := range files {
		if strings.Contains(strings.ToLower(f), q) {
			matches = append(matches, f)
		}
	}
	return matches
}

// indentBlock prefixes each non-empty line of text with the given prefix.
func indentBlock(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed != "" {
			lines[i] = prefix + trimmed
		}
	}
	return strings.Join(lines, "\n")
}

// ansiEscRe matches ANSI escape sequences (CSI and OSC).
var ansiEscRe = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]|\x1b\\].*?\x1b\\\\")

// wrapText word-wraps text to maxWidth columns, preserving ANSI escape sequences.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}

	type token struct {
		text   string
		isAnsi bool
		width  int
	}

	tokenise := func(input string) []token {
		var tokens []token
		for len(input) > 0 {
			loc := ansiEscRe.FindStringIndex(input)
			if loc != nil && loc[0] == 0 {
				tokens = append(tokens, token{text: input[:loc[1]], isAnsi: true})
				input = input[loc[1]:]
				continue
			}
			end := len(input)
			if loc != nil {
				end = loc[0]
			}
			plain := input[:end]
			input = input[end:]
			i := 0
			for i < len(plain) {
				if plain[i] == ' ' {
					j := i
					for j < len(plain) && plain[j] == ' ' {
						j++
					}
					tokens = append(tokens, token{text: plain[i:j], width: j - i})
					i = j
				} else {
					j := i
					for j < len(plain) && plain[j] != ' ' {
						j++
					}
					word := plain[i:j]
					tokens = append(tokens, token{text: word, width: uniseg.StringWidth(word)})
					i = j
				}
			}
		}
		return tokens
	}

	isSGR := func(t token) bool {
		if !t.isAnsi {
			return false
		}
		return len(t.text) > 2 && t.text[0] == 0x1b && t.text[1] == '[' && t.text[len(t.text)-1] == 'm'
	}

	isReset := func(t token) bool {
		return t.text == "\033[0m" || t.text == "\033[m"
	}

	var allLines []string
	for _, paragraph := range strings.Split(text, "\n") {
		tokens := tokenise(paragraph)
		if len(tokens) == 0 {
			allLines = append(allLines, "")
			continue
		}
		var lineBuf strings.Builder
		col := 0
		var activeSeqs []string

		flushLine := func() {
			allLines = append(allLines, lineBuf.String())
			lineBuf.Reset()
			col = 0
			for _, seq := range activeSeqs {
				lineBuf.WriteString(seq)
			}
		}

		for _, tok := range tokens {
			if tok.isAnsi {
				lineBuf.WriteString(tok.text)
				if isSGR(tok) {
					if isReset(tok) {
						activeSeqs = activeSeqs[:0]
					} else {
						activeSeqs = append(activeSeqs, tok.text)
					}
				}
				continue
			}
			if len(tok.text) > 0 && tok.text[0] == ' ' {
				if col == 0 {
					continue
				}
				if col+tok.width > maxWidth {
					flushLine()
					continue
				}
				lineBuf.WriteString(tok.text)
				col += tok.width
				continue
			}
			if tok.width == 0 {
				lineBuf.WriteString(tok.text)
				continue
			}
			if col+tok.width <= maxWidth {
				lineBuf.WriteString(tok.text)
				col += tok.width
				continue
			}
			if col > 0 {
				flushLine()
			}
			if tok.width > maxWidth {
				remaining := tok.text
				for len(remaining) > 0 {
					cluster, rest, cw, _ := uniseg.FirstGraphemeClusterInString(remaining, -1)
					if col+cw > maxWidth {
						flushLine()
					}
					lineBuf.WriteString(cluster)
					col += cw
					remaining = rest
				}
				continue
			}
			lineBuf.WriteString(tok.text)
			col += tok.width
		}
		if lineBuf.Len() > 0 {
			allLines = append(allLines, lineBuf.String())
		}
	}

	result := strings.Join(allLines, "\n")
	return strings.TrimRight(result, "\n")
}
