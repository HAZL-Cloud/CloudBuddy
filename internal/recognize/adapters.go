package recognize

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"cloudbuddy/internal/model"
)

// Adapters are data files that describe one provider's CLI commands, MCP
// tools and HTTP API (D-012, docs/adapters.md). They only add precision: the
// generic layer still runs on every call and the most cautious result wins
// (D-026). A nil *Adapters means generic recognition only.
type Adapters struct {
	files []*adapterFile
}

type rule struct {
	Match     []string `yaml:"match"`
	Action    string   `yaml:"action"`
	Tags      []string `yaml:"tags"`
	LiveTags  []string `yaml:"live_tags"`
	Reviewed  bool     `yaml:"reviewed"`
	RunsCode  bool     `yaml:"runs_code"`
	Product   string   `yaml:"product"`
	Title     string   `yaml:"title"`
	Loss      []string `yaml:"loss"`
	LossTitle string   `yaml:"loss_title"`
}

type httpRule struct {
	Method    string   `yaml:"method"`
	Path      string   `yaml:"path"`
	Action    string   `yaml:"action"`
	Tags      []string `yaml:"tags"`
	Reviewed  bool     `yaml:"reviewed"`
	Product   string   `yaml:"product"`
	Loss      []string `yaml:"loss"`
	LossTitle string   `yaml:"loss_title"`
}

type adapterFile struct {
	Name      string                         `yaml:"-"`
	Provider  string                         `yaml:"provider"`
	CLI       []string                       `yaml:"cli"`
	LiveFlags []string                       `yaml:"live_flags"`
	Products  map[string]string              `yaml:"products"`
	Rules     []rule                         `yaml:"rules"`
	Generated map[string]map[string][]string `yaml:"generated_operations"`
	MCP       struct {
		Servers []string `yaml:"servers"`
		Tools   []struct {
			Match     string   `yaml:"match"`
			Action    string   `yaml:"action"`
			Tags      []string `yaml:"tags"`
			Reviewed  bool     `yaml:"reviewed"`
			Product   string   `yaml:"product"`
			Loss      []string `yaml:"loss"`
			LossTitle string   `yaml:"loss_title"`
		} `yaml:"tools"`
	} `yaml:"mcp"`
	HTTP struct {
		Hosts []string   `yaml:"hosts"`
		Rules []httpRule `yaml:"rules"`
	} `yaml:"http"`

	generated map[string]string // "service/operation" -> action
}

var validActions = map[string]bool{"read": true, "create": true, "change": true, "send": true, "delete": true, "unknown": true, "money": true}
var validTags = map[string]bool{"live": true, "irreversible": true, "money": true, "people": true}

// LoadAdapters reads the built-in adapters and, if dir exists, the user's
// own adapter files from it. A file that doesn't parse or validate is an
// error: a silently skipped adapter would change what asks.
func LoadAdapters(dir string, builtin fs.FS) (*Adapters, error) {
	a := &Adapters{}
	load := func(fsys fs.FS, name string) error {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		f := &adapterFile{Name: name}
		if err := yaml.Unmarshal(b, f); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := f.validate(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		a.files = append(a.files, f)
		return nil
	}
	if builtin != nil {
		names, _ := fs.Glob(builtin, "*.yaml")
		sort.Strings(names)
		for _, n := range names {
			if err := load(builtin, n); err != nil {
				return nil, err
			}
		}
	}
	if dir != "" {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			names, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
			sort.Strings(names)
			for _, n := range names {
				base := filepath.Base(n)
				if builtin != nil {
					if _, err := fs.Stat(builtin, base); err == nil {
						continue // the built-in copy is already loaded
					}
				}
				if err := load(os.DirFS(dir), base); err != nil {
					return nil, err
				}
			}
		}
	}
	// hand-written files first, so their rules win over generated ones
	sort.SliceStable(a.files, func(i, j int) bool { return len(a.files[i].Generated) == 0 && len(a.files[j].Generated) > 0 })
	return a, nil
}

func (f *adapterFile) validate() error {
	if f.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	check := func(action string, tags []string) error {
		if !validActions[action] {
			return fmt.Errorf("unknown action %q", action)
		}
		for _, t := range tags {
			if !validTags[t] {
				return fmt.Errorf("unknown tag %q", t)
			}
		}
		return nil
	}
	for _, r := range f.Rules {
		if len(r.Match) == 0 {
			return fmt.Errorf("a rule has no match")
		}
		if err := check(r.Action, append(append([]string{}, r.Tags...), r.LiveTags...)); err != nil {
			return err
		}
		if r.Reviewed && r.Action != "read" && r.Action != "create" && r.Action != "change" {
			return fmt.Errorf("rule %v: only reads, creates and changes can be reviewed", r.Match)
		}
		if r.Reviewed && r.RunsCode {
			return fmt.Errorf("rule %v: code that runs with network can't be reviewed", r.Match)
		}
	}
	for _, t := range f.MCP.Tools {
		if err := check(t.Action, t.Tags); err != nil {
			return err
		}
	}
	for _, r := range f.HTTP.Rules {
		if err := check(r.Action, r.Tags); err != nil {
			return err
		}
	}
	f.generated = map[string]string{}
	for svc, acts := range f.Generated {
		for act, ops := range acts {
			if !validActions[act] {
				return fmt.Errorf("generated: unknown action %q", act)
			}
			for _, op := range ops {
				f.generated[svc+"/"+op] = act
			}
		}
	}
	return nil
}

func toAction(a string) (model.Action, []model.Tag) {
	if a == "money" { // generated "Purchase…" operations
		return model.ActionCreate, []model.Tag{model.TagMoney, model.TagIrreversible}
	}
	return model.Action(a), nil
}

func matchWords(pattern, words []string) bool {
	if len(words) < len(pattern) {
		return false
	}
	for i, p := range pattern {
		if p != "*" && !strings.EqualFold(p, words[i]) {
			return false
		}
	}
	return true
}

func (a *Adapters) matchShell(s segment, env Env) (Draft, bool) {
	if a == nil {
		return Draft{}, false
	}
	pos := positional(s.Args)
	for _, f := range a.files {
		if !contains(f.CLI, s.Prog) {
			continue
		}
		base := func(product string, by string) Draft {
			d := Draft{Kind: "cloud", Provider: f.Provider, Network: "A", Identity: identityFor(s, env),
				Resource: resourceOf(s.Args), API: s.Prog + " " + strings.Join(firstN(pos, 3), " ")}
			if s.Prog == "aws" && len(pos) >= 2 {
				d.API = pos[0] + ":" + pascal(pos[1])
			}
			d.Product = product
			if d.Product == "" && len(pos) > 0 {
				if p, ok := f.Products[pos[0]]; ok {
					d.Product = p
				} else if p, ok := awsProducts[pos[0]]; ok && s.Prog == "aws" {
					d.Product = p // generated AWS file shares the hand file's names
				} else {
					d.Product = strings.ToUpper(pos[0][:1]) + pos[0][1:]
				}
			}
			if d.Product == "" {
				d.Product = f.Provider
			}
			d.RecognizedBy = []string{by}
			return d
		}
		for _, r := range f.Rules {
			if !matchWords(r.Match, pos) {
				continue
			}
			d := base(r.Product, "builtin")
			act, extra := toAction(r.Action)
			d.Action = act
			for _, t := range append(r.Tags, tagsOf(extra)...) {
				d.addTag(model.Tag(t))
			}
			live := isLive(s) || hasFlag(s.Args, f.LiveFlags...)
			if live {
				d.addTag(model.TagLive)
				for _, t := range r.LiveTags {
					d.addTag(model.Tag(t))
				}
			}
			d.ReviewedRead = r.Reviewed && act == model.ActionRead
			d.ReviewedChange = r.Reviewed && len(r.Tags) == 0 && (act == model.ActionCreate || act == model.ActionChange)
			if r.RunsCode {
				d.Network = "B"
			}
			if r.Title != "" {
				d.Title, d.TitleSource = r.Title, "adapter"
			}
			d.Loss, d.LossTitle = r.Loss, r.LossTitle
			if act == model.ActionDelete && d.LossTitle == "" && len(d.Loss) > 0 {
				d.LossTitle = "What will be lost"
			}
			return d, true
		}
		if len(pos) >= 2 && f.generated != nil {
			if act, ok := f.generated[pos[0]+"/"+pos[1]]; ok {
				d := base("", "generated")
				a, extra := toAction(act)
				d.Action = a
				for _, t := range extra {
					d.addTag(t)
				}
				// generated reads are reviewed: the generator only labels
				// operations whose verb is on its reviewed read list
				d.ReviewedRead = a == model.ActionRead
				if isLive(s) {
					d.addTag(model.TagLive)
				}
				return d, true
			}
		}
	}
	return Draft{}, false
}

func tagsOf(ts []model.Tag) []string {
	var out []string
	for _, t := range ts {
		out = append(out, string(t))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (a *Adapters) matchMCP(c Call) (Draft, bool) {
	if a == nil {
		return Draft{}, false
	}
	server := strings.ToLower(c.Server)
	for _, f := range a.files {
		hit := false
		for _, s := range f.MCP.Servers {
			if strings.Contains(server, strings.ToLower(s)) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		tool := strings.ToLower(c.Tool)
		for _, t := range f.MCP.Tools {
			if ok, _ := path.Match(strings.ToLower(t.Match), tool); !ok {
				continue
			}
			act, extra := toAction(t.Action)
			d := Draft{Kind: "cloud", Provider: f.Provider, Product: t.Product, Action: act, Network: "A",
				API: c.Server + " · " + c.Tool, Identity: Identity{Resolved: false, Label: "the MCP server's account"},
				RecognizedBy: []string{"builtin"}, Loss: t.Loss, LossTitle: t.LossTitle}
			if d.Product == "" {
				d.Product = c.Server
			}
			for _, tg := range append(t.Tags, tagsOf(extra)...) {
				d.addTag(model.Tag(tg))
			}
			d.ReviewedRead = t.Reviewed && act == model.ActionRead
			d.ReviewedChange = t.Reviewed && len(t.Tags) == 0 && (act == model.ActionCreate || act == model.ActionChange)
			return d, true
		}
	}
	return Draft{}, false
}

func (a *Adapters) matchHTTP(host, method, p string) (Draft, bool) {
	if a == nil {
		return Draft{}, false
	}
	for _, f := range a.files {
		hit := false
		for _, h := range f.HTTP.Hosts {
			if host == h || strings.HasSuffix(host, "."+h) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		for _, r := range f.HTTP.Rules {
			if !strings.EqualFold(r.Method, method) {
				continue
			}
			if ok, _ := path.Match(r.Path, p); !ok && r.Path != "*" && !(strings.HasSuffix(r.Path, "*") && strings.HasPrefix(p, strings.TrimSuffix(r.Path, "*"))) {
				continue
			}
			act, extra := toAction(r.Action)
			d := Draft{Kind: "cloud", Provider: f.Provider, Product: r.Product, Action: act, Network: "A",
				RecognizedBy: []string{"builtin"}, Loss: r.Loss, LossTitle: r.LossTitle}
			for _, t := range append(r.Tags, tagsOf(extra)...) {
				d.addTag(model.Tag(t))
			}
			d.ReviewedRead = r.Reviewed && act == model.ActionRead
			d.ReviewedChange = r.Reviewed && len(r.Tags) == 0 && (act == model.ActionCreate || act == model.ActionChange)
			return d, true
		}
	}
	return Draft{}, false
}

// Summary lists what each loaded adapter covers (for Connections).
func (a *Adapters) Summary() map[string]string {
	out := map[string]string{}
	if a == nil {
		return out
	}
	for _, f := range a.files {
		kind := "builtin"
		if len(f.Generated) > 0 {
			kind = "generated"
		}
		if _, ok := out[f.Provider]; !ok || kind == "builtin" {
			out[f.Provider] = kind
		}
	}
	return out
}
