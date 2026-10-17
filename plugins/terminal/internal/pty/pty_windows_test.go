package pty

import (
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/plugins/terminal/internal/conpty"
)

// TestMain runs the test binary as the program in the terminal, when
// $MYGO_PTY_CHILD says so: it writes what full-screen programs write.
func TestMain(m *testing.M) {
	if os.Getenv("MYGO_PTY_CHILD") == "modes" {
		os.Stdout.WriteString(fullScreenOutput)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fullScreenOutput is what a full-screen program writes, as Claude Code,
// Codex or a ratatui app do: the alternate screen, the mouse in SGR, then
// a frame in a synchronized update.
const fullScreenOutput = "\x1b[?1049h\x1b[?1000h\x1b[?1006h" +
	"\x1b[?2026hframe line 1\r\nframe line 2\r\n\x1b[?2026l" +
	"\x1b[?1000l\x1b[?1006l\x1b[?1049lDONE\r\n"

// run runs the test binary as the full-screen program in a console of
// api, and returns what the terminal reads of it, up to DONE.
func run(t *testing.T, api *conptyAPI) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := &PTY{exited: make(chan struct{})}
	o := Options{Path: exe, Args: []string{exe}, Cols: 80, Rows: 24, Env: append(os.Environ(), "MYGO_PTY_CHILD=modes")}
	if _, err := p.startWith(api, o); err != nil {
		t.Fatal(err)
	}
	go func() {
		p.code, p.waitErr = p.wait()
		close(p.exited)
	}()
	defer p.Close()
	var mu sync.Mutex
	var out strings.Builder
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := p.Read(buf)
			mu.Lock()
			out.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	got := func() string { mu.Lock(); defer mu.Unlock(); return out.String() }
	wait.Until(t, "the program's DONE", func() bool { return strings.Contains(got(), "DONE") })
	if err := p.Resize(100, 30, 0, 0); err != nil {
		t.Errorf("resize: %v", err)
	}
	return got()
}

// TestShippedConPTYPassesModes: the ConPTY apps ship passes the output of
// full-screen programs through, their modes and their synchronized
// updates whole, which Windows 10's own drops or splits.
func TestShippedConPTYPassesModes(t *testing.T) {
	api := shippedAPI()
	if api == nil {
		t.Fatal("the ConPTY of the app did not load (see the log)")
	}
	out := run(t, api)
	for _, want := range []string{
		"\x1b[?1049h", "\x1b[?1000h", "\x1b[?1006h",
		"\x1b[?2026hframe line 1\r\nframe line 2\r\n\x1b[?2026l",
		"\x1b[?1000l", "\x1b[?1006l",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
}

// TestInboxConPTY: Windows' own ConPTY still runs programs, as when the
// app's does not load.
func TestInboxConPTY(t *testing.T) {
	if !strings.Contains(run(t, inbox), "DONE") {
		t.Error("no output")
	}
}

// TestLoadShipped: MYGO_CONPTY=inbox keeps to Windows' ConPTY, and a
// conpty.dll that does not load is an error, which start passes over.
func TestLoadShipped(t *testing.T) {
	t.Setenv("MYGO_CONPTY", "inbox")
	if api, err := loadShipped(); err == nil || api != nil {
		t.Errorf("MYGO_CONPTY=inbox loaded %v", api)
	}
	t.Setenv("MYGO_CONPTY", `C:\nowhere\conpty.dll`)
	if api, err := loadShipped(); err == nil || api != nil {
		t.Errorf("a missing conpty.dll loaded %v", api)
	}
}

// TestShippedHostBesideDLL: OpenConsole.exe is beside the conpty.dll that
// loads, where conpty.dll starts it from.
func TestShippedHostBesideDLL(t *testing.T) {
	if shippedAPI() == nil {
		t.Fatal("the ConPTY of the app did not load")
	}
	dll, err := conpty.Path()
	if err != nil {
		t.Fatal(err)
	}
	host := dll[:strings.LastIndexAny(dll, `\/`)+1] + "OpenConsole.exe"
	f, err := os.Open(host)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	head := make([]byte, 2)
	if _, err := io.ReadFull(f, head); err != nil || string(head) != "MZ" {
		t.Errorf("%s is not a program: %q %v", host, head, err)
	}
}
