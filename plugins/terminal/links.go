package terminal

import (
	"regexp"
	"runtime"
	"strings"
)

// urlPattern finds the URLs programs print as text, which a Command+click
// opens as it does hyperlinks.
var urlPattern = regexp.MustCompile(`(?:https?|ftp|file)://[^\s"'<>` + "`" + `\x00-\x1f]+|mailto:[^\s"'<>` + "`" + `]+`)

// pathPattern finds the file paths programs print as text: absolute, from
// ~, ./ or ../, with a slash, or a file name with a :line; each may end in
// :line or :line:col. On Windows, those with a drive (C:\dir\file,
// C:/dir/file) or with backslashes (dir\file, .\file) too.
var pathPattern = pathPatternFor(runtime.GOOS == "windows")

// pathPatternFor returns pathPattern, with Windows' paths when windows is
// set.
func pathPatternFor(windows bool) *regexp.Regexp {
	const name = `[^\s"'<>` + "`" + `\x00-\x1f:]+`
	const line = `(?::\d+(?::\d+)?)?`
	const unix = `(?:(?:~|\.{1,2})?/` + name + `|[\w.@+-]+(?:/[\w.@+-]+)+/?)` + line + `|[\w@+-][\w.@+-]*\.\w+:\d+(?::\d+)?`
	if !windows {
		return regexp.MustCompile(unix)
	}
	// A drive's path first, which the others would take from its first
	// separator; then backslashes, from .\ or ..\ or with a folder.
	const drive = `[A-Za-z]:[\\/]` + name + line
	const backslash = `(?:(?:~|\.{1,2})\\` + name + `|[\w.@+-]+(?:\\[\w.@+-]+)+\\?)` + line
	return regexp.MustCompile(drive + `|` + backslash + `|` + unix)
}

// urlAt returns the URL of text, a row whose runes are in columns cols, at
// column col, or "".
func urlAt(text []rune, cols []int, col int) string {
	return matchAt(urlPattern, text, cols, col)
}

// pathAt returns the file path of text at column col, as urlAt, or "".
func pathAt(text []rune, cols []int, col int) string {
	return matchAt(pathPattern, text, cols, col)
}

func matchAt(re *regexp.Regexp, text []rune, cols []int, col int) string {
	s := string(text)
	for _, m := range re.FindAllStringIndex(s, -1) {
		u := trimURL(s[m[0]:m[1]])
		if u == "" {
			continue
		}
		start := len([]rune(s[:m[0]]))
		end := start + len([]rune(u))
		if start < len(cols) && cols[start] <= col && col <= cols[end-1] {
			return u
		}
	}
	return ""
}

// trimURL trims what likely ends the sentence around a URL rather than the
// URL: punctuation, and closing brackets that the URL did not open.
func trimURL(u string) string {
	for len(u) > 0 {
		switch last := u[len(u)-1]; last {
		case '.', ',', ';', ':', '!', '?':
			u = u[:len(u)-1]
			continue
		case ')', ']', '}':
			open := map[byte]string{')': "(", ']': "[", '}': "{"}[last]
			if strings.Count(u, open) < strings.Count(u, string(last)) {
				u = u[:len(u)-1]
				continue
			}
		}
		return u
	}
	return u
}
