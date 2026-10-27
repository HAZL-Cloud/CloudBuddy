// Package connections finds what the agent can reach from this computer:
// cloud CLIs on PATH and the user's MCP servers (section 4.7). It never reads
// or stores keys; it shows only what the tools say about who is signed in.
// The result is for display; the policy works out identity per call.
package connections

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/store"
)

type Scanner struct{ home string }

func New(home string) *Scanner { return &Scanner{home: home} }

type cliDef struct {
	bin, name, mark, adapter, note string
	identity                       func(ctx context.Context, home string) (account, detail string)
}

var cliDefs = []cliDef{
	{bin: "aws", name: "AWS", mark: "AW", adapter: "builtin", note: "One adapter covers every AWS service.", identity: awsIdentity},
	{bin: "gcloud", name: "Google Cloud", mark: "GC", adapter: "generic", identity: gcloudIdentity},
	{bin: "stripe", name: "Stripe", mark: "St", adapter: "builtin", note: "Knows live mode, refunds and payouts.", identity: stripeIdentity},
	{bin: "wrangler", name: "Cloudflare", mark: "Cf", adapter: "builtin", identity: simpleIdentity("wrangler", "whoami")},
	{bin: "vercel", name: "Vercel", mark: "Ve", adapter: "generic", identity: simpleIdentity("vercel", "whoami")},
	{bin: "gh", name: "GitHub", mark: "Gh", adapter: "generic", identity: ghIdentity},
	{bin: "az", name: "Azure", mark: "Az", adapter: "generic"},
	{bin: "kubectl", name: "Kubernetes", mark: "K8", adapter: "generic", identity: simpleIdentity("kubectl", "config", "current-context")},
	{bin: "flyctl", name: "Fly.io", mark: "Fl", adapter: "generic"},
	{bin: "heroku", name: "Heroku", mark: "He", adapter: "generic"},
	{bin: "doctl", name: "DigitalOcean", mark: "Do", adapter: "generic"},
	{bin: "supabase", name: "Supabase", mark: "Sb", adapter: "generic"},
	{bin: "firebase", name: "Firebase", mark: "Fb", adapter: "generic"},
	{bin: "netlify", name: "Netlify", mark: "Nl", adapter: "generic"},
	{bin: "terraform", name: "Terraform", mark: "Tf", adapter: "generic"},
	{bin: "pulumi", name: "Pulumi", mark: "Pu", adapter: "generic"},
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) string {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(c, name, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	s, _ := mask.String(strings.TrimSpace(string(out)))
	return s
}

var versionRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// Scan finds connections and saves them.
func (s *Scanner) Scan(ctx context.Context, st *store.Store) error {
	var mu sync.Mutex
	var conns []model.Connection
	var wg sync.WaitGroup
	for _, d := range cliDefs {
		path, err := exec.LookPath(d.bin)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(d cliDef, path string) {
			defer wg.Done()
			v := versionRe.FindString(run(ctx, 6*time.Second, path, "--version"))
			c := model.Connection{ID: "cli:" + d.bin, Kind: "cli", Name: d.name, Mark: d.mark,
				How: strings.TrimSpace(d.bin + " " + v), Account: "Not checked", Adapter: d.adapter, AdapterNote: d.note, Supported: true}
			if d.identity != nil {
				a, det := d.identity(ctx, s.home)
				if a != "" {
					c.Account = a
				}
				c.Detail = det
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}(d, path)
	}
	wg.Wait()
	conns = append(conns, s.mcpConnections()...)
	sort.Slice(conns, func(i, j int) bool { return conns[i].ID < conns[j].ID })
	return st.SaveConnections(ctx, conns)
}

func awsIdentity(ctx context.Context, home string) (string, string) {
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	region := run(ctx, 6*time.Second, "aws", "configure", "get", "region")
	detail := "profile " + profile
	if region != "" {
		detail += " · " + region
	}
	out := run(ctx, 10*time.Second, "aws", "sts", "get-caller-identity", "--output", "json")
	var id struct{ Account string }
	if json.Unmarshal([]byte(out), &id) == nil && id.Account != "" {
		a := id.Account
		if len(a) == 12 {
			a = a[:4] + "-" + a[4:8] + "-" + a[8:]
		}
		return "Account " + a, detail
	}
	return "Not signed in", detail
}

func gcloudIdentity(ctx context.Context, home string) (string, string) {
	out := run(ctx, 8*time.Second, "gcloud", "config", "list", "--format=json")
	var cfg struct {
		Core struct{ Account, Project string } `json:"core"`
	}
	if json.Unmarshal([]byte(out), &cfg) == nil && cfg.Core.Account != "" {
		d := ""
		if cfg.Core.Project != "" {
			d = "project " + cfg.Core.Project
		}
		return cfg.Core.Account, d
	}
	return "Not signed in", ""
}

// stripeIdentity reads only non-secret fields of the Stripe CLI config.
func stripeIdentity(ctx context.Context, home string) (string, string) {
	b, err := os.ReadFile(filepath.Join(home, ".config", "stripe", "config.toml"))
	if err != nil {
		return "Not signed in", ""
	}
	var cfg map[string]map[string]any
	if toml.Unmarshal(b, &cfg) != nil {
		return "Logged in", ""
	}
	if d, ok := cfg["default"]; ok {
		name, _ := d["display_name"].(string)
		acct, _ := d["account_id"].(string)
		detail := "test and live keys"
		if _, has := d["live_mode_api_key"]; !has {
			detail = "test mode key"
		}
		if name == "" {
			name = acct
		}
		return name, detail
	}
	return "Logged in", ""
}

func ghIdentity(ctx context.Context, home string) (string, string) {
	out := run(ctx, 8*time.Second, "gh", "auth", "status")
	m := regexp.MustCompile(`account (\S+)`).FindStringSubmatch(out)
	if len(m) == 2 {
		return m[1], "github.com"
	}
	return "Not signed in", ""
}

func simpleIdentity(bin string, args ...string) func(context.Context, string) (string, string) {
	return func(ctx context.Context, home string) (string, string) {
		out := run(ctx, 10*time.Second, bin, args...)
		lines := strings.Split(out, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && len(l) < 80 {
				return l, ""
			}
		}
		return "Unknown", ""
	}
}

// mcpConnections lists MCP servers from Claude Code and Codex settings.
func (s *Scanner) mcpConnections() []model.Connection {
	type srv struct {
		cmd, url string
		agents   []string
	}
	found := map[string]*srv{}
	add := func(name, cmd, url, agent string) {
		x := found[name]
		if x == nil {
			x = &srv{cmd: cmd, url: url}
			found[name] = x
		}
		x.agents = append(x.agents, agent)
	}
	if b, err := os.ReadFile(filepath.Join(s.home, ".claude.json")); err == nil {
		var cfg struct {
			MCPServers map[string]struct {
				Type, Command, URL string
			} `json:"mcpServers"`
		}
		if json.Unmarshal(b, &cfg) == nil {
			for n, v := range cfg.MCPServers {
				add(n, v.Command, v.URL, "Claude Code")
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(s.home, ".codex", "config.toml")); err == nil {
		var cfg struct {
			MCPServers map[string]struct {
				Command string `toml:"command"`
				URL     string `toml:"url"`
			} `toml:"mcp_servers"`
		}
		if toml.Unmarshal(b, &cfg) == nil {
			for n, v := range cfg.MCPServers {
				add(n, v.Command, v.URL, "Codex")
			}
		}
	}
	var out []model.Connection
	for name, x := range found {
		c := model.Connection{ID: "mcp:" + name, Kind: "mcp", Name: recognize.ProviderForMCP(name), Mark: markFor(name),
			Account: name, Detail: "set up in " + strings.Join(x.agents, " and "), Adapter: "hints", Supported: true}
		if x.cmd != "" {
			c.How = "MCP · local · " + filepath.Base(x.cmd)
		} else {
			c.How = "MCP · " + x.url
			c.Supported = false
			c.AdapterNote = "Servers reached by URL aren't supported in CloudBuddy runs yet."
		}
		if c.Name == "Stripe" || c.Name == "Resend" || c.Name == "Cloudflare" {
			c.Adapter = "builtin"
		}
		out = append(out, c)
	}
	return out
}

func markFor(name string) string {
	r := []rune(strings.TrimSpace(name))
	if len(r) == 0 {
		return "?"
	}
	if len(r) == 1 {
		return strings.ToUpper(string(r))
	}
	return strings.ToUpper(string(r[0])) + string(r[1])
}
