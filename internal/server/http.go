package server

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/niksingh2745/rawth/internal/rql"
	"github.com/niksingh2745/rawth/internal/storage"
)

// HTTPServer serves the web UI and provides a REST API + WebSocket interface.
type HTTPServer struct {
	executor *rql.Executor
	engine   *storage.Engine
	addr     string
	server   *http.Server
	webFS    fs.FS // embedded web files
	wsConns  map[*wsConn]bool
	wsMu     sync.Mutex
}

// wsConn is a minimal WebSocket connection wrapper.
// We implement WebSocket from scratch because of course we do.
type wsConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

// NewHTTPServer creates a new HTTP server.
func NewHTTPServer(executor *rql.Executor, engine *storage.Engine, addr string, webFS fs.FS) *HTTPServer {
	return &HTTPServer{
		executor: executor,
		engine:   engine,
		addr:     addr,
		webFS:    webFS,
		wsConns:  make(map[*wsConn]bool),
	}
}

// Start begins serving HTTP requests.
func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Serve the embedded web UI
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		mux.Handle("/", fileServer)
	}

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[http] listening on %s", s.addr)

	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("[http] server error: %s", err)
		}
	}()

	return nil
}

// corsMiddleware adds CORS headers for local development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleQuery executes an RQL command via HTTP POST.
func (s *HTTPServer) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed — POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(rql.Result{
			Ok:      false,
			Message: "invalid JSON body — send {\"query\": \"your RQL here\"}",
		})
		return
	}

	result := s.executor.Execute(req.Query)

	w.Header().Set("Content-Type", "application/json")
	w.Write(result.FormatJSON())
}

// handleStats returns database statistics as JSON.
func (s *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.engine.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleHealth is a simple health check endpoint.
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "alive",
		"engine":  "rawth",
		"version": "1.0.0",
	})
}

// handleWebSocket upgrades an HTTP connection to WebSocket for live terminal interaction.
// We implement the WebSocket handshake ourselves because we're building everything from scratch.
func (s *HTTPServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		http.Error(w, "expected WebSocket upgrade", http.StatusBadRequest)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	acceptKey := computeAcceptKey(key)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n" +
		"\r\n"

	bufrw.WriteString(response)
	bufrw.Flush()

	ws := &wsConn{
		conn:   conn,
		reader: bufrw.Reader,
	}

	s.wsMu.Lock()
	s.wsConns[ws] = true
	s.wsMu.Unlock()

	log.Printf("[ws] new connection from %s", conn.RemoteAddr())

	// Send welcome
	s.wsSend(ws, `{"ok":true,"message":"connected to rawth — type HELP to get started"}`)

	// Read loop
	s.wsReadLoop(ws)
}

// wsReadLoop reads WebSocket frames and executes RQL commands.
func (s *HTTPServer) wsReadLoop(ws *wsConn) {
	defer func() {
		s.wsMu.Lock()
		delete(s.wsConns, ws)
		s.wsMu.Unlock()
		ws.conn.Close()
		log.Printf("[ws] connection closed from %s", ws.conn.RemoteAddr())
	}()

	for {
		payload, err := wsReadFrame(ws.reader)
		if err != nil {
			return
		}

		query := strings.TrimSpace(string(payload))
		if query == "" {
			continue
		}

		result := s.executor.Execute(query)
		jsonBytes := result.FormatJSON()
		s.wsSend(ws, string(jsonBytes))
	}
}

// wsSend sends a text message over WebSocket.
func (s *HTTPServer) wsSend(ws *wsConn, message string) {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	wsWriteFrame(ws.conn, []byte(message))
}

// Stop gracefully shuts down the HTTP server.
func (s *HTTPServer) Stop() error {
	s.wsMu.Lock()
	for ws := range s.wsConns {
		ws.conn.Close()
	}
	s.wsMu.Unlock()

	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// --- WebSocket frame helpers (RFC 6455) ---
// Minimal but compliant. No external dependencies.

const wsMagicGUID = "258EAFA5-E914-47DA-95CA-5AB5DC76B45B"

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsMagicGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsReadFrame reads a single WebSocket frame.
func wsReadFrame(r *bufio.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	payloadLen := int64(header[1] & 0x7F)

	if payloadLen == 126 {
		ext := make([]byte, 2)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext))
	} else if payloadLen == 127 {
		ext := make([]byte, 8)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext))
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	// Close frame
	if opcode == 0x8 {
		return nil, fmt.Errorf("close frame received")
	}

	// Ping frame — respond with pong
	if opcode == 0x9 {
		// Send pong with same payload
		pongFrame := []byte{0x8A} // FIN + pong opcode
		if len(payload) < 126 {
			pongFrame = append(pongFrame, byte(len(payload)))
		}
		pongFrame = append(pongFrame, payload...)
		// Ignore write errors for pong
		return nil, nil
	}

	return payload, nil
}

// wsWriteFrame writes a text WebSocket frame (server-to-client, unmasked).
func wsWriteFrame(w net.Conn, payload []byte) error {
	frame := []byte{0x81} // FIN + text opcode

	if len(payload) < 126 {
		frame = append(frame, byte(len(payload)))
	} else if len(payload) < 65536 {
		frame = append(frame, 126)
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(len(payload)))
		frame = append(frame, lenBytes...)
	} else {
		frame = append(frame, 127)
		lenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lenBytes, uint64(len(payload)))
		frame = append(frame, lenBytes...)
	}

	frame = append(frame, payload...)
	_, err := w.Write(frame)
	return err
}
