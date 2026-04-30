package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/Nikhil-Singh2745/rawth/internal/rql"
)

// rawth over tcp. you can telnet into this if you want. 
// it's like redis but the commands sound like a frat party (SHOVE, YEET).
type TCPServer struct {
	executor *rql.Executor
	listener net.Listener
	addr     string
}

func NewTCPServer(executor *rql.Executor, addr string) *TCPServer {
	return &TCPServer{
		executor: executor,
		addr:     addr,
	}
}

// start listening. if the port is taken we just die. 
// standard net.Listen stuff. nothing fancy.
func (s *TCPServer) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("rawth tcp: failed to listen on %s: %w", s.addr, err)
	}

	log.Printf("[tcp] listening on %s", s.addr)

	go s.acceptLoop()
	return nil
}

// wait for people to show up. 
func (s *TCPServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Printf("[tcp] accept error: %s", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// one connection, one goroutine. 
// we show a nice banner and then wait for them to type things.
func (s *TCPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[tcp] new connection from %s", remoteAddr)

	// say hi
	fmt.Fprintf(conn, "rawth v1.0.0 — your bytes, your disk, your rules\r\n")
	fmt.Fprintf(conn, "Type HELP for available commands, QUIT to disconnect\r\n")
	fmt.Fprintf(conn, "rawth> ")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			fmt.Fprintf(conn, "rawth> ")
			continue
		}

		if strings.ToUpper(line) == "QUIT" || strings.ToUpper(line) == "EXIT" {
			fmt.Fprintf(conn, "goodbye 👋\r\n")
			log.Printf("[tcp] %s disconnected", remoteAddr)
			return
		}

		// execute it and hope for the best.
		result := s.executor.Execute(line)
		fmt.Fprintf(conn, "%s\r\n", result.FormatText())
		fmt.Fprintf(conn, "rawth> ")
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[tcp] read error from %s: %s", remoteAddr, err)
	}

	log.Printf("[tcp] %s disconnected", remoteAddr)
}

func (s *TCPServer) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *TCPServer) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}
