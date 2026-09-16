//go:build integration

package integration

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// This proxy cuts SSH transport without terminating the remote Herdr server.
type remoteProxy struct {
	listener    net.Listener
	target      string
	mu          sync.Mutex
	offline     bool
	delay       time.Duration
	stalled     bool
	connections map[net.Conn]bool
	wg          sync.WaitGroup
}

func newRemoteProxy(t *testing.T, target string) *remoteProxy {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	p := &remoteProxy{listener: l, target: target, connections: map[net.Conn]bool{}}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			client, err := l.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			if p.offline {
				p.mu.Unlock()
				_ = client.Close()
				continue
			}
			p.connections[client] = true
			p.wg.Add(1)
			p.mu.Unlock()
			go p.forward(client)
		}
	}()
	t.Cleanup(func() { _ = l.Close(); p.setOffline(true); p.wg.Wait() })
	return p
}

func (p *remoteProxy) forward(client net.Conn) {
	defer p.wg.Done()
	defer func() { _ = client.Close(); p.mu.Lock(); delete(p.connections, client); p.mu.Unlock() }()
	server, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		return
	}
	defer func() { _ = server.Close() }()
	p.mu.Lock()
	if p.offline {
		p.mu.Unlock()
		return
	}
	p.connections[server] = true
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.connections, server); p.mu.Unlock() }()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(transportWriter{proxy: p, writer: server}, client)
		_ = server.Close()
		close(done)
	}()
	_, _ = io.Copy(transportWriter{proxy: p, writer: client}, server)
	_ = client.Close()
	<-done
}

func (p *remoteProxy) setOffline(value bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offline = value
	if value {
		for conn := range p.connections {
			_ = conn.Close()
		}
	}
}

type transportWriter struct {
	proxy  *remoteProxy
	writer io.Writer
}

func (w transportWriter) Write(raw []byte) (int, error) {
	for {
		w.proxy.mu.Lock()
		offline, stalled, delay := w.proxy.offline, w.proxy.stalled, w.proxy.delay
		w.proxy.mu.Unlock()
		if offline {
			return 0, io.ErrClosedPipe
		}
		if stalled {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		return w.writer.Write(raw)
	}
}

func (p *remoteProxy) setTransport(delay time.Duration, stalled bool) {
	p.mu.Lock()
	p.delay = delay
	p.stalled = stalled
	p.mu.Unlock()
}
