package terminal

import "testing"

func TestURLAt(t *testing.T) {
	row := "see https://example.com/a_(b)?q=1. and (http://x.org/y), mailto:me@x.org!"
	text := []rune(row)
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	for col, want := range map[int]string{
		0:  "",
		4:  "https://example.com/a_(b)?q=1",
		20: "https://example.com/a_(b)?q=1",
		33: "", // the period after it
		40: "http://x.org/y",
		58: "mailto:me@x.org",
	} {
		if got := urlAt(text, cols, col); got != want {
			t.Errorf("urlAt(%d) = %q, want %q", col, got, want)
		}
	}
}

func TestPathAt(t *testing.T) {
	for _, c := range []struct {
		row  string
		col  int
		want string
	}{
		{"Updated internal/app/embed.go:51 and more", 10, "internal/app/embed.go:51"},
		{"  ⎿  Read /Users/me/repo/main.go (12 lines)", 12, "/Users/me/repo/main.go"},
		{"see ./notes/todo.txt.", 8, "./notes/todo.txt"},
		{"open ~/Downloads/clip.mp4", 10, "~/Downloads/clip.mp4"},
		{"main.go:250:3: undefined: x", 2, "main.go:250:3"},
		{"(src/a.go)", 3, "src/a.go"},
		{"just words here", 5, ""},
		{"version 1.2.3 ok", 9, ""},
	} {
		text := []rune(c.row)
		cols := make([]int, len(text))
		for i := range cols {
			cols[i] = i
		}
		if got := pathAt(text, cols, c.col); got != c.want {
			t.Errorf("pathAt(%q, %d) = %q, want %q", c.row, c.col, got, c.want)
		}
	}
}

func TestPathAtWindows(t *testing.T) {
	windows, unix := pathPatternFor(true), pathPatternFor(false)
	for _, c := range []struct {
		row  string
		col  int
		want string
	}{
		{`  ⎿  Read D:\projects\mycode\main.go (12 lines)`, 20, `D:\projects\mycode\main.go`},
		{`C:\src\app\main.go:250:3: undefined: x`, 5, `C:\src\app\main.go:250:3`},
		{`see C:/Users/me/notes.txt.`, 8, `C:/Users/me/notes.txt`},
		{`Updated cmd\mayoi\panel.go:51 and more`, 10, `cmd\mayoi\panel.go:51`},
		{`run .\build\app.exe`, 6, `.\build\app.exe`},
		{`go to ..\other\x.go`, 8, `..\other\x.go`},
		{`PS D:\projects\mycode> git status`, 8, `D:\projects\mycode`},
		{`Updated internal/app/embed.go:51 and more`, 10, `internal/app/embed.go:51`},
		{`main.go:250:3: undefined: x`, 2, `main.go:250:3`},
		{`just words here`, 5, ``},
	} {
		text := []rune(c.row)
		cols := make([]int, len(text))
		for i := range cols {
			cols[i] = i
		}
		if got := matchAt(windows, text, cols, c.col); got != c.want {
			t.Errorf("windows: pathAt(%q, %d) = %q, want %q", c.row, c.col, got, c.want)
		}
	}
	// Elsewhere a backslash is no separator.
	text := []rune(`cmd\mayoi\panel.go`)
	cols := make([]int, len(text))
	for i := range cols {
		cols[i] = i
	}
	if got := matchAt(unix, text, cols, 5); got != "" {
		t.Errorf("unix: took %q", got)
	}
}
