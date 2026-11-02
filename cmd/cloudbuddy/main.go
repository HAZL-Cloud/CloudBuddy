// Command cloudbuddy runs the CloudBuddy server and its helper programs.
//
//	cloudbuddy [serve]        start the local web app (default)
//	cloudbuddy hook           Claude Code PreToolUse hook (started by Claude Code)
//	cloudbuddy exec           run one gate-released command (started by Claude Code)
//	cloudbuddy shim <cli>     stand in for a cloud CLI inside a command (D-065)
//	cloudbuddy mcp-gateway    MCP proxy in front of one server (started by an agent)
//	cloudbuddy dashboards-mcp CloudBuddy's own MCP tools for dashboards (started by Claude Code)
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
	"cloudbuddy/internal/dashmcp"
	"cloudbuddy/internal/execwrap"
	"cloudbuddy/internal/hookcmd"
	"cloudbuddy/internal/mcpgw"
	"cloudbuddy/internal/policy"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/run"
	"cloudbuddy/internal/server"
	"cloudbuddy/internal/shellenv"
	"cloudbuddy/internal/shim"
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
	case "shim":
		shim.Decide = shimDecide
		os.Exit(shim.Main(args))
	case "mcp-gateway":
		os.Exit(mcpgw.Main(args))
	case "dashboards-mcp":
		os.Exit(dashmcp.Main(args))
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
	// Programs CloudBuddy runs outside the sandbox are found once, here, and
	// the folders they and the user's PATH live in are kept out of commands'
	// reach (Codex review of D-065).
	loginPath := shellenv.Login()["PATH"]
	claudeExe, _ := exec.LookPath("claude")
	direnvExe := shellenv.Find(loginPath, "direnv")
	if direnvExe == "" {
		direnvExe, _ = exec.LookPath("direnv")
	}
	eng.SetClaude(claudeExe)
	eng.SetDirenv(direnvExe)
	programs := []string{claudeExe, direnvExe}
	for _, name := range recognize.ShimCLIs() {
		programs = append(programs, shellenv.Find(loginPath, name))
	}
	eng.Protect([]string{loginPath, os.Getenv("PATH")}, programs)
	agents := detectAgents(eng)
	srv := server.New(cfg, eng, b, webFS, func() []server.Agent { return agents }, scan)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	if err != nil {
		log.Printf("Port %d is busy. Is CloudBuddy already running? (%v)", cfg.Port, err)
		return 1
	}
	// CLI shims and their socket (D-065), outside CloudBuddy's protected
	// folder so commands can use them.
	rt := filepath.Join(os.TempDir(), "cloudbuddy-"+cfg.Epoch)
	if err := os.MkdirAll(filepath.Join(rt, "sock"), 0o700); err == nil {
		sock := filepath.Join(rt, "sock", "shim.sock")
		if sl, err := net.Listen("unix", sock); err == nil {
			go func() { _ = (&http.Server{Handler: srv.ShimHandler(), ReadHeaderTimeout: 10 * time.Second}).Serve(sl) }()
			if err := eng.SetShims(filepath.Join(rt, "shims"), sock); err != nil {
				log.Println("shims:", err)
			}
			defer os.RemoveAll(rt)
		} else {
			log.Println("shim socket:", err)
		}
	}
	// Dashboards refresh on their timers while CloudBuddy runs (D-067).
	eng.StartDashboards(ctx, 10*time.Second)
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println(err)
		}
	}()
	url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port)
	fmt.Printf("\nCloudBuddy %s is running.\nOpen: %s\n\n", config.Version, url)
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
	// Codex is paused while CloudBuddy works like native Claude Code (D-066)
	out = append(out, server.Agent{Name: "codex", Label: "Codex", ComingSoon: true})
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

// shimDecide is how a shim decides a call when it can't reach CloudBuddy:
// the gate's own recognizer and rules, with the default connection rule.
func shimDecide(name string, args []string, cwd string) bool {
	home, _ := os.UserHomeDir()
	ads, _ := recognize.LoadAdapters("", adapters.FS())
	d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: shim.Join(name, args), WantsNetwork: true, Cwd: cwd},
		recognize.Env{Home: home, Workdir: cwd}, ads)
	return policy.Decide(d, policy.Context{CanWrapCode: true}).Verdict == policy.Allow
}
