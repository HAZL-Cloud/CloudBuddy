// Command cloudbuddy runs the CloudBuddy server and its helper programs.
//
//	cloudbuddy [serve]        start the local web app (default)
//	cloudbuddy hook           Claude Code PreToolUse hook (started by Claude Code)
//	cloudbuddy exec           run one gate-released command (started by Claude Code)
//	cloudbuddy mcp-gateway    MCP proxy in front of one server (started by an agent)
//	cloudbuddy version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"cloudbuddy/adapters"
	"cloudbuddy/internal/agent"
	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/config"
	"cloudbuddy/internal/connections"
	"cloudbuddy/internal/execwrap"
	"cloudbuddy/internal/hookcmd"
	"cloudbuddy/internal/mcpgw"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/run"
	"cloudbuddy/internal/server"
	"cloudbuddy/internal/store"
	"cloudbuddy/web"
)

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		os.Exit(serve(args))
	case "hook":
		os.Exit(hookcmd.Main(args))
	case "exec":
		os.Exit(execwrap.Main(args))
	case "mcp-gateway":
		os.Exit(mcpgw.Main(args))
	case "version":
		fmt.Println("cloudbuddy", config.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (try: serve, version)\n", cmd)
		os.Exit(2)
	}
}

func serve(args []string) int {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	dev := fl.Bool("dev", false, "also accept the Vite dev server origin (127.0.0.1:5173)")
	noOpen := fl.Bool("no-open", false, "don't open the browser")
	webDir := fl.String("web", "", "serve the frontend from this folder instead of the built-in copy")
	fake := fl.String("fake", "", "enable the demo agent with this scenario file")
	adaptersDir := fl.String("adapters", "", "load adapter files from this folder (default: next to the binary)")
	_ = fl.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		log.Println("CloudBuddy can't start:", err)
		return 1
	}
	cfg.Dev = *dev
	if strings.ContainsAny(cfg.Bin, " \t'\"") {
		log.Printf("CloudBuddy needs to live in a folder without spaces or quotes (now: %s).", cfg.Bin)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DBURL)
	if err != nil {
		log.Println(err)
		log.Println("Start the database with: docker compose up -d")
		return 1
	}
	defer st.Close()

	var loaded *recognize.Adapters
	dir := *adaptersDir
	if dir == "" {
		dir = filepath.Join(filepath.Dir(filepath.Dir(cfg.Bin)), "adapters")
	}
	if a, err := recognize.LoadAdapters(dir, adapters.FS()); err == nil {
		loaded = a
	} else {
		log.Println("adapters:", err)
	}

	b := bus.New()
	eng := run.New(cfg, st, b, loaded)
	eng.ClaudeCanary = true
	if *fake != "" {
		eng.UseFake(&agent.Fake{Scenario: *fake, Delay: 700 * time.Millisecond})
	}
	if err := eng.Recover(ctx); err != nil {
		log.Println("recover:", err)
	}
	scanner := connections.New(cfg.Home)
	scan := func(c context.Context) error { return scanner.Scan(c, st) }
	go func() {
		if err := scan(context.Background()); err != nil {
			log.Println("connections scan:", err)
		}
		b.Publish(bus.Event{Type: "connections"})
	}()

	var webFS fs.FS
	switch {
	case *webDir != "":
		webFS = os.DirFS(*webDir)
	case web.Built() != nil:
		webFS = web.Built()
	default:
		guess := filepath.Join(filepath.Dir(filepath.Dir(cfg.Bin)), "web", "dist")
		if _, err := os.Stat(filepath.Join(guess, "index.html")); err == nil {
			webFS = os.DirFS(guess)
		}
	}
	agents := detectAgents(eng)
	srv := server.New(cfg, eng, b, webFS, func() []server.Agent { return agents }, scan)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	if err != nil {
		log.Printf("Port %d is busy. Is CloudBuddy already running? (%v)", cfg.Port, err)
		return 1
	}
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println(err)
		}
	}()
	url := fmt.Sprintf("http://127.0.0.1:%d/launch/%s", cfg.Port, srv.NewLaunchCode())
	fmt.Printf("\nCloudBuddy %s is running.\nOpen: %s\n(The link works once and expires in 60 seconds. Restart CloudBuddy for a new one.)\n\n", config.Version, url)
	if !*noOpen {
		openBrowser(url)
	}
	<-ctx.Done()
	fmt.Println("\nStopping CloudBuddy…")
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutdown)
	return 0
}

func detectAgents(eng *run.Engine) []server.Agent {
	version := func(bin string) (string, bool) {
		out, err := exec.Command(bin, "--version").Output()
		if err != nil {
			return "", false
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		for _, w := range f {
			if len(w) > 0 && w[0] >= '0' && w[0] <= '9' {
				return w, true
			}
		}
		return strings.TrimSpace(string(out)), true
	}
	var out []server.Agent
	v, ok := version("claude")
	out = append(out, server.Agent{Name: "claude", Label: "Claude Code", Version: v, Available: ok})
	v, ok = version("codex")
	out = append(out, server.Agent{Name: "codex", Label: "Codex", Version: v, Available: ok})
	for _, a := range eng.Agents() {
		if a == "fake" {
			out = append(out, server.Agent{Name: "fake", Label: "Demo agent", Version: "replay", Available: true})
		}
	}
	return out
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "linux":
		c = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = c.Start()
}
