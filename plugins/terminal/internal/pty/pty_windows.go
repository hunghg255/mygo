package pty

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/egoist/mygo/plugins/terminal/internal/conpty"
)

var (
	kernel32                              = syscall.NewLazyDLL("kernel32.dll") // a KnownDLL
	procInitializeProcThreadAttributeList = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = kernel32.NewProc("DeleteProcThreadAttributeList")
)

const (
	procThreadAttributePseudoConsole = 0x00020016
	extendedStartupInfoPresent       = 0x00080000
	createUnicodeEnvironment         = 0x00000400
)

type startupInfoEx struct {
	syscall.StartupInfo
	attributes uintptr
}

type sys struct {
	mu      sync.Mutex
	api     *conptyAPI // the ConPTY that made the console
	console uintptr    // the HPCON, 0 once closed
	in      *os.File
	out     *os.File
	process syscall.Handle
	pid     int
}

// conptyAPI is a ConPTY's functions: Windows' own, in kernel32.dll, or
// those of the conpty.dll an app ships, which runs the console in its
// OpenConsole.exe.
type conptyAPI struct {
	name                  string // what errors call it
	create, resize, close *syscall.LazyProc
}

// inbox is Windows' own ConPTY. Windows 10's renders the screen again
// rather than pass the output through: full-screen programs lose their
// mouse, alternate screen and synchronized updates.
var inbox = &conptyAPI{name: "Windows' ConPTY",
	create: kernel32.NewProc("CreatePseudoConsole"),
	resize: kernel32.NewProc("ResizePseudoConsole"),
	close:  kernel32.NewProc("ClosePseudoConsole")}

var shipped struct {
	once sync.Once
	api  *conptyAPI
}

// shippedAPI returns the ConPTY the app ships (package conpty), nil when
// it has none or it does not load, which it logs once. MYGO_CONPTY=inbox
// keeps to Windows' own.
func shippedAPI() *conptyAPI {
	shipped.once.Do(func() {
		api, err := loadShipped()
		if err != nil {
			log.Printf("terminal: %v; using Windows' own ConPTY", err)
			return
		}
		shipped.api = api
	})
	return shipped.api
}

// loadShipped loads the ConPTY the app ships.
func loadShipped() (*conptyAPI, error) {
	if v := os.Getenv("MYGO_CONPTY"); v == "inbox" {
		return nil, errors.New("MYGO_CONPTY=inbox")
	}
	path, err := conpty.Path()
	if err != nil {
		return nil, err
	}
	dll := syscall.NewLazyDLL(path)
	api := &conptyAPI{name: "conpty.dll",
		create: dll.NewProc("ConptyCreatePseudoConsole"),
		resize: dll.NewProc("ConptyResizePseudoConsole"),
		close:  dll.NewProc("ConptyClosePseudoConsole")}
	for _, p := range []*syscall.LazyProc{api.create, api.resize, api.close} {
		if err := p.Find(); err != nil {
			return nil, fmt.Errorf("loading %s: %w", path, err)
		}
	}
	return api, nil
}

func (p *PTY) start(o Options) error {
	// The ConPTY the app ships, and Windows' own when it fails, as when
	// a security product stops OpenConsole.exe.
	if api := shippedAPI(); api != nil {
		created, err := p.startWith(api, o)
		if created || err == nil {
			return err // the program's own failure, as a missing one
		}
		log.Printf("terminal: %v; using Windows' own ConPTY", err)
	}
	if err := inbox.create.Find(); err != nil {
		return errors.New("pty: ConPTY needs Windows 10 1809 or later")
	}
	_, err := p.startWith(inbox, o)
	return err
}

// startWith starts the process in a console that api makes, and reports
// whether it made the console.
func (p *PTY) startWith(api *conptyAPI, o Options) (created bool, err error) {
	// The console reads what is typed from inR and writes the output to
	// outW; this side writes to inW and reads outR.
	var inR, inW, outR, outW syscall.Handle
	if err := syscall.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return false, fmt.Errorf("pty: %w", err)
	}
	if err := syscall.CreatePipe(&outR, &outW, nil, 0); err != nil {
		syscall.CloseHandle(inR)
		syscall.CloseHandle(inW)
		return false, fmt.Errorf("pty: %w", err)
	}
	var console uintptr
	r, _, _ := api.create.Call(coord(o.Cols, o.Rows), uintptr(inR), uintptr(outW), 0, uintptr(unsafe.Pointer(&console)))
	// The console has its own copies of its ends.
	syscall.CloseHandle(inR)
	syscall.CloseHandle(outW)
	if r != 0 {
		syscall.CloseHandle(inW)
		syscall.CloseHandle(outR)
		return false, fmt.Errorf("pty: CreatePseudoConsole of %s: %w", api.name, syscall.Errno(r&0xFFFF))
	}
	p.api, p.console = api, console
	p.in, p.out = os.NewFile(uintptr(inW), "conpty-in"), os.NewFile(uintptr(outR), "conpty-out")
	if err := p.spawn(o); err != nil {
		p.Close()
		p.out.Close()
		p.api, p.console, p.in, p.out = nil, 0, nil, nil
		return true, err
	}
	return true, nil
}

func (p *PTY) spawn(o Options) error {
	var size uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&size)))
	list := make([]byte, size)
	if r, _, err := procInitializeProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&list[0])), 1, 0, uintptr(unsafe.Pointer(&size))); r == 0 {
		return fmt.Errorf("pty: InitializeProcThreadAttributeList: %w", err)
	}
	defer procDeleteProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&list[0])))
	if r, _, err := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(&list[0])), 0, procThreadAttributePseudoConsole, p.console, unsafe.Sizeof(p.console), 0, 0); r == 0 {
		return fmt.Errorf("pty: UpdateProcThreadAttribute: %w", err)
	}
	si := startupInfoEx{attributes: uintptr(unsafe.Pointer(&list[0]))}
	si.Cb = uint32(unsafe.Sizeof(si))
	// The console's own standard handles: without invalid ones, a child of
	// a process whose output is redirected would write there instead.
	si.Flags = syscall.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = syscall.InvalidHandle, syscall.InvalidHandle, syscall.InvalidHandle

	cmdline, err := syscall.UTF16PtrFromString(commandLine(o.Path, o.Args))
	if err != nil {
		return err
	}
	var dir *uint16
	if o.Dir != "" {
		if dir, err = syscall.UTF16PtrFromString(o.Dir); err != nil {
			return err
		}
	}
	var env *uint16
	if o.Env != nil {
		block := environmentBlock(o.Env)
		env = &block[0]
	}
	var pi syscall.ProcessInformation
	err = syscall.CreateProcess(nil, cmdline, nil, nil, false, extendedStartupInfoPresent|createUnicodeEnvironment, env, dir, &si.StartupInfo, &pi)
	if err != nil {
		return fmt.Errorf("pty: starting %s: %w", o.Path, err)
	}
	syscall.CloseHandle(pi.Thread)
	p.process, p.pid = pi.Process, int(pi.ProcessId)
	return nil
}

func (p *PTY) wait() (int, error) {
	if _, err := syscall.WaitForSingleObject(p.process, syscall.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	err := syscall.GetExitCodeProcess(p.process, &code)
	syscall.CloseHandle(p.process)
	return int(code), err
}

// Read reads the output of the process; it returns io.EOF after Close, once
// the console wrote what it had.
func (p *PTY) Read(b []byte) (int, error) { return p.out.Read(b) }

// Write types b into the terminal.
func (p *PTY) Write(b []byte) (int, error) {
	n, err := p.in.Write(b)
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return n, err
}

// Resize sets the size of the terminal.
func (p *PTY) Resize(cols, rows, width, height int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.console == 0 {
		return ErrClosed
	}
	if r, _, _ := p.api.resize.Call(p.console, coord(cols, rows)); r != 0 {
		return fmt.Errorf("pty: ResizePseudoConsole: %w", syscall.Errno(r&0xFFFF))
	}
	return nil
}

// Pid returns the process's ID.
func (p *PTY) Pid() int { return p.pid }

// Close closes the console, which ends the processes attached to it. Read
// returns io.EOF once it wrote what it had.
func (p *PTY) Close() error {
	p.mu.Lock()
	console := p.console
	p.console = 0
	p.mu.Unlock()
	if console == 0 {
		return nil
	}
	p.in.Close()
	// ClosePseudoConsole waits for the output to be read on Windows
	// before 11 24H2: Read keeps reading meanwhile, until the end of the
	// output, then the pipe goes.
	api, out := p.api, p.out
	go func() {
		api.close.Call(console)
		out.Close()
	}()
	return nil
}

// Kill kills the process.
func (p *PTY) Kill() error { return syscall.TerminateProcess(p.process, 1) }

// coord packs a COORD, which ConPTY takes by value.
func coord(cols, rows int) uintptr {
	return uintptr(uint16(max(1, min(cols, 0x7FFF)))) | uintptr(uint16(max(1, min(rows, 0x7FFF))))<<16
}

// commandLine joins a program and its arguments as the C runtime splits
// them; args[0] is the program's name.
func commandLine(path string, args []string) string {
	parts := []string{syscall.EscapeArg(path)}
	if len(args) > 1 {
		for _, a := range args[1:] {
			parts = append(parts, syscall.EscapeArg(a))
		}
	}
	return strings.Join(parts, " ")
}

// environmentBlock encodes env as CreateProcess takes it: NUL-terminated
// UTF-16 strings, then another NUL.
func environmentBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	return append(b, 0)
}
