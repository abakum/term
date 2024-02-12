package term

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	windowsconsole "github.com/abakum/term/windows"
	"golang.org/x/sys/windows"
)

// terminalState holds the platform-specific state / console mode for the terminal.
type terminalState struct {
	mode uint32
}

// vtInputSupported is true if winterm.ENABLE_VIRTUAL_TERMINAL_INPUT is supported by the console
var vtInputSupported bool

func stdStreams() (stdIn io.ReadCloser, stdOut, stdErr io.Writer) {
	// Turn on VT handling on all std handles, if possible. This might
	// fail, in which case we will fall back to terminal emulation.
	var (
		emulateStdin, emulateStdout, emulateStderr bool

		mode uint32
	)

	fd := windows.Handle(os.Stdin.Fd())
	if err := windows.GetConsoleMode(fd, &mode); err == nil {
		// Validate that winterm.ENABLE_VIRTUAL_TERMINAL_INPUT is supported, but do not set it.
		if err = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
			emulateStdin = true
		} else {
			vtInputSupported = true
		}
		// Unconditionally set the console mode back even on failure because SetConsoleMode
		// remembers invalid bits on input handles.
		_ = windows.SetConsoleMode(fd, mode)
	}

	fd = windows.Handle(os.Stdout.Fd())
	if err := windows.GetConsoleMode(fd, &mode); err == nil {
		// Validate winterm.DISABLE_NEWLINE_AUTO_RETURN is supported, but do not set it.
		if err = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN); err != nil {
			emulateStdout = true
		} else {
			_ = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		}
	}

	fd = windows.Handle(os.Stderr.Fd())
	if err := windows.GetConsoleMode(fd, &mode); err == nil {
		// Validate winterm.DISABLE_NEWLINE_AUTO_RETURN is supported, but do not set it.
		if err = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN); err != nil {
			emulateStderr = true
		} else {
			_ = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		}
	}

	if emulateStdin {
		h := uint32(windows.STD_INPUT_HANDLE)
		stdIn = windowsconsole.NewAnsiReader(int(h))
	} else {
		stdIn = os.Stdin
	}

	if emulateStdout {
		h := uint32(windows.STD_OUTPUT_HANDLE)
		stdOut = windowsconsole.NewAnsiWriter(int(h))
	} else {
		stdOut = os.Stdout
	}

	if emulateStderr {
		h := uint32(windows.STD_ERROR_HANDLE)
		stdErr = windowsconsole.NewAnsiWriter(int(h))
	} else {
		stdErr = os.Stderr
	}

	return stdIn, stdOut, stdErr
}

func getFdInfo(in interface{}) (uintptr, bool) {
	return windowsconsole.GetHandleInfo(in)
}

func getWinsize(fd uintptr) (*Winsize, error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info); err != nil {
		return nil, err
	}

	winsize := &Winsize{
		Width:  uint16(info.Window.Right - info.Window.Left + 1),
		Height: uint16(info.Window.Bottom - info.Window.Top + 1),
	}

	return winsize, nil
}

func setWinsize(fd uintptr, ws *Winsize) error {
	return fmt.Errorf("not implemented on Windows")
}

func isTerminal(fd uintptr) bool {
	var mode uint32
	err := windows.GetConsoleMode(windows.Handle(fd), &mode)
	return err == nil
}

func restoreTerminal(fd uintptr, state *State) error {
	return windows.SetConsoleMode(windows.Handle(fd), state.mode)
}

func saveState(fd uintptr) (*State, error) {
	var mode uint32

	if err := windows.GetConsoleMode(windows.Handle(fd), &mode); err != nil {
		return nil, err
	}

	return &State{mode: mode}, nil
}

func disableEcho(fd uintptr, state *State) error {
	// See https://msdn.microsoft.com/en-us/library/windows/desktop/ms683462(v=vs.85).aspx
	mode := state.mode
	mode &^= windows.ENABLE_ECHO_INPUT
	mode |= windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT
	err := windows.SetConsoleMode(windows.Handle(fd), mode)
	if err != nil {
		return err
	}

	// Register an interrupt handler to catch and restore prior state
	restoreAtInterrupt(fd, state)
	return nil
}

func setRawTerminal(fd uintptr) (*State, error) {
	oldState, err := MakeRaw(fd)
	if err != nil {
		return nil, err
	}

	// Register an interrupt handler to catch and restore prior state
	restoreAtInterrupt(fd, oldState)
	return oldState, err
}

func setRawTerminalOutput(fd uintptr) (*State, error) {
	oldState, err := saveState(fd)
	if err != nil {
		return nil, err
	}

	// Ignore failures, since winterm.DISABLE_NEWLINE_AUTO_RETURN might not be supported on this
	// version of Windows.
	_ = windows.SetConsoleMode(windows.Handle(fd), oldState.mode|windows.DISABLE_NEWLINE_AUTO_RETURN)
	return oldState, err
}

func restoreAtInterrupt(fd uintptr, state *State) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)

	go func() {
		<-sigchan
		RestoreTerminal(fd, state)
		os.Exit(0)
	}()
}

func AllowVTI() (ok bool, err error) {
	src := os.Stdin
	var (
		mode uint32
	)
	fd := windows.Handle(src.Fd())
	if err := windows.GetConsoleMode(fd, &mode); err == nil {
		// isConsole
		// Validate that winterm.ENABLE_VIRTUAL_TERMINAL_INPUT is supported, but do not set it.
		ok = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_INPUT) == nil
		// Unconditionally set the console mode back even on failure because SetConsoleMode
		// remembers invalid bits on input handles.
		_ = windows.SetConsoleMode(fd, mode)
	}
	return
}

var ErrWin10 = errors.New("no need emulate")

func StdOE(src *os.File) (io.Writer, *os.File, error) {
	var (
		mode    uint32
		emulate bool
	)

	fd := windows.Handle(src.Fd())
	if err := windows.GetConsoleMode(fd, &mode); err == nil {
		// Validate winterm.DISABLE_NEWLINE_AUTO_RETURN is supported, but do not set it.
		if err = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN); err != nil {
			emulate = true
		} else {
			_ = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		}
	}

	if emulate {
		return windowsconsole.NewAnsiWriterFileDuplicate(src)
	}
	return nil, nil, ErrWin10
}

func AllowVTP(src *os.File) (ok bool, err error) {
	var (
		mode uint32
	)
	if src == nil {
		src = os.Stdout
	}
	fd := windows.Handle(src.Fd())
	err = windows.GetConsoleMode(fd, &mode)
	if err == nil {
		// isConsole
		ok = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN) == nil
		_ = windows.SetConsoleMode(fd, mode)
	}
	return
}

func EnableVTP() (ok bool) {
	var (
		mode uint32
	)
	fd := windows.Handle(os.Stdout.Fd())
	if err := windows.GetConsoleMode(fd, &mode); err == nil {
		// isConsole
		ok = windows.SetConsoleMode(fd, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
	}
	return
}

type IOE struct {
	i, o, e *State
	rc      io.ReadCloser
}

func NewIOE() (s *IOE) {
	s = &IOE{}

	stdStreams()
	s.i, _ = setRawTerminal(os.Stdin.Fd())
	s.o, _ = setRawTerminalOutput(os.Stdout.Fd())
	s.e, _ = setRawTerminalOutput(os.Stderr.Fd())
	// for Win10 no need emulation VTP but need close dublicate of os.Stdin
	// to unblock input after return
	s.rc, _ = windowsconsole.NewAnsiReaderDuplicate(os.Stdin)
	return
}

func (s *IOE) Close() {
	if s == nil {
		return
	}
	if s.rc != nil {
		if s.rc.Close() == nil {
			s.rc = nil
		}
	}
	if s.e != nil {
		if restoreTerminal(os.Stderr.Fd(), s.e) == nil {
			s.e = nil
		}
	}
	if s.o != nil {
		if restoreTerminal(os.Stdout.Fd(), s.o) == nil {
			s.o = nil
		}
	}
	if s.i != nil {
		if restoreTerminal(os.Stdin.Fd(), s.i) == nil {
			s.i = nil
		}
	}
	s = nil
}
