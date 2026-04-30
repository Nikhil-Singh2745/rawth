package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nikhil-Singh2745/rawth/internal/rql"
	"github.com/Nikhil-Singh2745/rawth/internal/server"
	"github.com/Nikhil-Singh2745/rawth/internal/storage"
	"github.com/Nikhil-Singh2745/rawth/web"
)

const version = "1.0.0"

const banner = `
                          __  __  
   _________ __      __ / /_/ /_ 
  / ___/ __ '/ | /| / / __/ __ \
 / /  / /_/ /| |/ |/ / /_/ / / /
/_/   \__,_/ |__/|__/\__/_/ /_/ 
                                 
  your bytes. your disk. your rules.
  v%s
`

// starts the whole circus. 
func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe()
	case "version":
		fmt.Printf("rawth v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(banner, version)
	fmt.Print(`
Usage:
  rawth serve [options]    Start the rawth server
  rawth version            Print version info
  rawth help               Show this help

Serve Options:
  --http PORT   HTTP port (default: 8080)
  --tcp PORT    TCP port (default: 6379)
  --data PATH   Database file path (default: rawth.db)
`)
}

// this is where we actually start the servers. 
// we open the engine, seed some data if it's empty, 
// and then start the TCP and HTTP servers. 
// if any of this fails i'm going back to bed.
func cmdServe() {
	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	httpPort := serveFlags.Int("http", 8080, "HTTP server port")
	tcpPort := serveFlags.Int("tcp", 6379, "TCP server port")
	dataPath := serveFlags.String("data", "rawth.db", "Database file path")
	serveFlags.Parse(os.Args[2:])

	fmt.Printf(banner, version)

	log.Printf("[engine] opening database at %s", *dataPath)
	engine, err := storage.OpenEngine(*dataPath)
	if err != nil {
		log.Fatalf("[engine] failed to open database: %s", err)
	}
	defer engine.Close()

	// if the db is empty i put some stuff in so it doesn't look sad. 
	// first impressions matter.
	stats := engine.Stats()
	if stats.KeyCount == 0 {
		seedDemoData(engine)
	}

	executor := rql.NewExecutor(engine)

	// tcp server for the redis-wannabes
	tcpAddr := fmt.Sprintf(":%d", *tcpPort)
	tcpServer := server.NewTCPServer(executor, tcpAddr)
	if err := tcpServer.Start(); err != nil {
		log.Printf("[tcp] warning: could not start TCP server: %s", err)
	} else {
		defer tcpServer.Stop()
	}

	// http server for the web terminal. embedded files are magic.
	webFS := web.GetFS()

	httpAddr := fmt.Sprintf(":%d", *httpPort)
	httpServer := server.NewHTTPServer(executor, engine, httpAddr, webFS)
	if err := httpServer.Start(); err != nil {
		log.Fatalf("[http] failed to start HTTP server: %s", err)
	}
	defer httpServer.Stop()

	log.Printf("[rawth] ready — web UI at http://localhost:%d", *httpPort)

	// wait for someone to hit ctrl-c or kill us. 
	// graceful shutdown? sure, why not.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	log.Printf("[rawth] received %s — shutting down", sig)
}

// put some stuff in so users have something to look at. 
// if you delete these... well, you do you.
func seedDemoData(engine *storage.Engine) {
	log.Println("[engine] seeding demo data...")

	demos := map[string]string{
		"greeting":    "hello from rawth — a database built from scratch",
		"author":      "nikhil singh",
		"language":    "go",
		"storage":     "B+Tree on disk with 4KB pages",
		"file_format": "custom binary — magic bytes RAWT",
		"query_lang":  "RQL (Rawth Query Language)",
		"philosophy":  "your bytes, your disk, your rules",
		"built_with":  "no ORM, no Postgres, no SQLite — just raw Go",
		"manifesto":   "while you were abstracting, I was byte-shifting",
	}

	for k, v := range demos {
		if err := engine.Put([]byte(k), []byte(v), 0); err != nil {
			log.Printf("[engine] warning: failed to seed %q: %s", k, err)
		}
	}

	log.Printf("[engine] seeded %d demo entries", len(demos))
}
