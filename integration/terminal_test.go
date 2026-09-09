//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type terminal struct {
	s         *scenario
	cmd       *exec.Cmd
	file      *os.File
	log       *os.File
	mu        sync.Mutex
	writeMu   sync.Mutex
	output    []byte
	readErr   error
	exitErr   error
	readDone  chan struct{}
	exitDone  chan struct{}
	closeOnce sync.Once
}

func (s *scenario) terminal(label, binary string, args ...string) *terminal {
	s.t.Helper()
	// #nosec G204 -- The scenario selects the binary and arguments; no shell evaluates them.
	cmd := exec.Command(binary, args...)
	cmd.Env, cmd.Dir = environment(s.env), s.root+"/work"
	// #nosec G304 G703 -- The test controls both the artifact directory and log label.
	log, err := os.Create(s.artifacts + "/" + label + ".pty.log")
	must(s.t, err)
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 160, Rows: 40})
	if err != nil {
		_ = log.Close()
		s.t.Fatal(err)
	}
	pollable, err := pollablePTY(file)
	if err != nil {
		_ = killGroup(cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		_ = file.Close()
		_ = log.Close()
		s.t.Fatal(err)
	}
	_ = file.Close()
	p := &terminal{s: s, cmd: cmd, file: pollable, log: log, readDone: make(chan struct{}), exitDone: make(chan struct{})}
	s.t.Cleanup(p.close)
	go p.read()
	go func() { p.exitErr = cmd.Wait(); close(p.exitDone) }()
	return p
}

func pollablePTY(file *os.File) (*os.File, error) {
	conn, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	fd := -1
	var dupErr error
	err = conn.Control(func(original uintptr) { fd, dupErr = unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0) })
	if err = errors.Join(err, dupErr); err != nil {
		return nil, err
	}
	if fd < 0 {
		return nil, fmt.Errorf("invalid duplicated PTY descriptor %d", fd)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	// NewFile must see O_NONBLOCK when registering the descriptor with Go's poller.
	return os.NewFile(uintptr(fd), file.Name()), nil
}

func (p *terminal) read() {
	defer close(p.readDone)
	buf := make([]byte, 32<<10)
	offset := 0
	for {
		n, err := p.file.Read(buf)
		if n > 0 {
			if _, writeErr := p.log.Write(buf[:n]); writeErr != nil {
				p.setReadError(writeErr)
				return
			}
			p.mu.Lock()
			p.output = append(p.output, buf[:n]...)
			if len(p.output) > 8<<20 {
				p.mu.Unlock()
				p.setReadError(errors.New("PTY output exceeded 8 MiB"))
				return
			}
			start := max(0, offset-8)
			tail := append([]byte(nil), p.output[start:]...)
			end := len(p.output)
			p.mu.Unlock()
			for _, query := range []struct{ request, response string }{
				{"\x1b[6n", "\x1b[1;1R"}, {"\x1b[c", "\x1b[?1;2c"}, {"\x1b[?u", "\x1b[?0u"},
			} {
				for i := 0; i < len(tail); {
					found := bytes.Index(tail[i:], []byte(query.request))
					if found < 0 {
						break
					}
					i += found + len(query.request)
					if start+i > offset {
						if err := p.sendRaw(query.response); err != nil {
							p.setReadError(err)
							return
						}
					}
				}
			}
			offset = end
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EIO) && !errors.Is(err, os.ErrClosed) {
				p.setReadError(err)
			}
			return
		}
	}
}

func (p *terminal) setReadError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readErr = err
}

func (p *terminal) offset() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.output)
}

func (p *terminal) textSince(start int) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ansi.Strip(string(p.output[min(start, len(p.output)):]))
}

func (p *terminal) after(marker string) int {
	p.s.t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	offset := bytes.LastIndex(p.output, []byte(marker))
	if offset < 0 {
		p.s.t.Fatalf("missing output marker %q", marker)
	}
	return offset + len(marker)
}

func (p *terminal) sendRaw(text string) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err := p.file.WriteString(text)
	return err
}

func (p *terminal) send(text string) {
	p.s.t.Helper()
	must(p.s.t, p.sendRaw(text))
}

func (p *terminal) expect(text string, start int) {
	p.s.t.Helper()
	var stopped error
	err := waitFor(suiteContext, 20*time.Second, func() bool {
		if bytes.Contains([]byte(p.textSince(start)), []byte(text)) {
			return true
		}
		p.mu.Lock()
		stopped = p.readErr
		p.mu.Unlock()
		if stopped != nil {
			return true
		}
		select {
		case <-p.readDone:
			select {
			case <-p.exitDone:
				stopped = errors.New("TUI exited before the expected output")
				if p.exitErr != nil {
					stopped = fmt.Errorf("TUI exited: %w", p.exitErr)
				}
				return true
			default:
			}
		default:
		}
		return false
	})
	if err != nil || stopped != nil {
		tail := p.textSince(max(0, p.offset()-3000))
		p.s.t.Fatalf("waiting for %q: %v\n%s", text, errors.Join(err, stopped), tail)
	}
}

func (p *terminal) sendExpect(keys, text string) {
	p.s.t.Helper()
	start := p.offset()
	p.send(keys)
	p.expect(text, start)
}

func (p *terminal) resize(cols, rows uint16) {
	p.s.t.Helper()
	conn, err := p.file.SyscallConn()
	must(p.s.t, err)
	var resizeErr error
	err = conn.Control(func(fd uintptr) {
		value, err := descriptorInt(fd)
		if err != nil {
			resizeErr = err
			return
		}
		resizeErr = unix.IoctlSetWinsize(value, unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows})
	})
	must(p.s.t, errors.Join(err, resizeErr))
	must(p.s.t, killGroup(p.cmd.Process.Pid, syscall.SIGWINCH))
}

func (p *terminal) quit() {
	p.s.t.Helper()
	p.send("q")
	select {
	case <-p.exitDone:
		must(p.s.t, p.exitErr)
	case <-suiteContext.Done():
		p.s.t.Fatal(suiteContext.Err())
	case <-time.After(10 * time.Second):
		p.s.t.Fatal("TUI did not quit")
	}
	p.close()
}

func (p *terminal) close() {
	p.closeOnce.Do(func() {
		select {
		case <-p.exitDone:
		default:
			if err := killGroup(p.cmd.Process.Pid, syscall.SIGTERM); err != nil {
				p.s.t.Errorf("terminate TUI: %v", err)
			}
			select {
			case <-p.exitDone:
			case <-time.After(3 * time.Second):
				if err := killGroup(p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
					p.s.t.Errorf("kill TUI: %v", err)
				}
				select {
				case <-p.exitDone:
				case <-time.After(3 * time.Second):
					p.s.t.Error("TUI did not exit after SIGKILL")
				}
			}
		}
		_ = p.file.Close()
		select {
		case <-p.readDone:
		case <-time.After(3 * time.Second):
			p.s.t.Error("PTY reader did not exit")
		}
		if err := p.log.Close(); err != nil {
			p.s.t.Errorf("close PTY log: %v", err)
		}
	})
}

func processRunning(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G204 -- ps receives a positive PID from the isolated test shell.
	raw, err := exec.CommandContext(ctx, "/bin/ps", "-o", "stat=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return len(bytes.TrimSpace(raw)) > 0 && !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("Z")), nil
}

func descriptorInt(fd uintptr) (int, error) {
	maxInt := uintptr(^uint(0) >> 1)
	if fd > maxInt {
		return 0, fmt.Errorf("file descriptor exceeds int range")
	}
	return int(fd), nil
}
