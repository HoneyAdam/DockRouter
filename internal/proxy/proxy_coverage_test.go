package proxy

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWebSocketProxyDialError tests WebSocket proxy when backend is unreachable
func TestWebSocketProxyDialError(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	req := httptest.NewRequest("GET", "/ws", nil)
	rw := &hijackableResponseWriter{conn: serverConn}

	errCh := make(chan error, 1)
	go func() {
		errCh <- wp.ServeHTTP(rw, req, "127.0.0.1:19999")
		serverConn.Close()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error when backend is unreachable")
		}
		if !strings.Contains(err.Error(), "failed to connect") {
			t.Errorf("error = %v, want connection error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for dial error")
	}
}

// TestWebSocketProxyEndToEnd tests the full WebSocket proxy flow with real TCP connections
func TestWebSocketProxyEndToEnd(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer backend.Close()

	go func() {
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		var reqLines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			reqLines = append(reqLines, line)
			if strings.TrimSpace(line) == "" {
				break
			}
		}

		if len(reqLines) == 0 || !strings.Contains(reqLines[0], "GET") {
			return
		}

		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"\r\n"
		conn.Write([]byte(resp))

		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := reader.Read(buf)
		if err != nil {
			return
		}
		conn.Write(buf[:n])
	}()

	backendAddr := backend.Addr().String()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Protocol", "chat")
	req.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")
	req.Header.Set("Origin", "http://example.com")

	errCh := make(chan error, 1)
	go func() {
		rw := &hijackableResponseWriter{conn: serverConn}
		errCh <- wp.ServeHTTP(rw, req, backendAddr)
		serverConn.Close()
	}()

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReader(clientConn)
	var respLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading response: %v", err)
		}
		respLines = append(respLines, line)
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	if len(respLines) == 0 || !strings.Contains(respLines[0], "101") {
		t.Fatalf("expected 101 response, got: %v", respLines)
	}

	testData := "hello websocket"
	clientConn.Write([]byte(testData))

	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("reading echo: %v", err)
	}
	if string(buf[:n]) != testData {
		t.Errorf("echo = %q, want %q", string(buf[:n]), testData)
	}

	clientConn.Close()
	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("proxy error (expected on close): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for proxy to finish")
	}
}

// hijackableResponseWriter is a mock ResponseWriter that supports Hijack
type hijackableResponseWriter struct {
	conn net.Conn
}

func (h *hijackableResponseWriter) Header() http.Header         { return http.Header{} }
func (h *hijackableResponseWriter) Write(b []byte) (int, error) { return h.conn.Write(b) }
func (h *hijackableResponseWriter) WriteHeader(code int)        {}
func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(h.conn)), nil
}

// TestHijackConnection tests the HijackConnection helper
func TestHijackConnection(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	rw := &hijackableResponseWriter{conn: conn1}
	req := httptest.NewRequest("GET", "/ws", nil)

	hijackedConn, buf, err := HijackConnection(rw, req)
	if err != nil {
		t.Fatalf("HijackConnection error: %v", err)
	}
	if hijackedConn == nil {
		t.Error("connection should not be nil")
	}
	if buf == nil {
		t.Error("buffer should not be nil")
	}
}

// TestHijackConnectionNotSupported tests HijackConnection with non-hijackable writer
func TestHijackConnectionNotSupported(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws", nil)

	_, _, err := HijackConnection(rw, req)
	if err == nil {
		t.Error("HijackConnection should fail with non-hijackable writer")
	}
	if !strings.Contains(err.Error(), "hijacking not supported") {
		t.Errorf("error = %v, want hijacking not supported", err)
	}
}

// TestBuildErrorPageXSS tests that the error page escapes HTML
func TestBuildErrorPageXSS(t *testing.T) {
	page := buildErrorPage(500, "<script>alert('xss')</script>", "test", "<script>")
	if strings.Contains(page, "<script>") {
		t.Error("should escape HTML in error page")
	}
}

// TestRemoveHopHeadersComplete tests all hop-by-hop headers are removed
func TestRemoveHopHeadersComplete(t *testing.T) {
	h := http.Header{}
	h.Set("Connection", "keep-alive")
	h.Set("Keep-Alive", "timeout=5")
	h.Set("Proxy-Authenticate", "Basic")
	h.Set("Proxy-Authorization", "Basic abc")
	h.Set("Te", "trailers")
	h.Set("Trailer", "X-Foo")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Upgrade", "websocket")
	h.Set("Content-Type", "application/json")
	h.Set("X-Custom", "value")

	removeHopHeaders(h)

	for _, name := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"} {
		if v := h.Get(name); v != "" {
			t.Errorf("hop header %s should be removed, got %q", name, v)
		}
	}

	if h.Get("Content-Type") != "application/json" {
		t.Error("Content-Type should not be removed")
	}
	if h.Get("X-Custom") != "value" {
		t.Error("X-Custom should not be removed")
	}
}

// TestWebSocketProxyNotHijackable tests error when writer doesn't support hijack
func TestWebSocketProxyNotHijackable(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws", nil)

	err := wp.ServeHTTP(w, req, "127.0.0.1:19999")
	if err == nil {
		t.Error("expected error with non-hijackable writer or unreachable backend")
	}
}
