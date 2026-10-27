// Package recognize turns one agent tool call into a draft step: provider,
// product, action, tags, identity and what kind of network access it needs
// (plan section 4.4). Every layer runs and the most cautious result wins
// (D-026); only reviewed rules may mark a call as a read.
package recognize

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"cloudbuddy/internal/model"
)

// Call is one tool call as seen by the gate.
type Call struct {
	Agent        string         // claude | codex
	Via          string         // shell | mcp | file | tool
	Command      string         // shell command line
	WantsNetwork bool           // asked to leave the no-network sandbox
	Tool         string         // agent tool name (Bash, Read, WebFetch) or MCP tool name
	Server       string         // MCP server name
	Args         map[string]any // tool input or MCP arguments
	Hints        *Hints         // MCP annotations reported by the gateway
	Paths        []string       // files a file tool touches
	Cwd          string
	Description  string // the agent's own words for this call
}

// Hints are MCP tool annotations. Nil means "not declared".
type Hints struct {
	ReadOnly    *bool `json:"readOnlyHint,omitempty"`
	Destructive *bool `json:"destructiveHint,omitempty"`
}

// Env describes the machine the call runs on.
type Env struct {
	Home         string
	DataDir      string // CloudBuddy's own folder
	Port         int    // CloudBuddy's port
	Workdir      string
	Protected    []string          // extra protected paths
	AgentEnv     map[string]string // environment the agent was started with
	CloudBuddyBin string
}

type Identity struct {
	Resolved bool   `json:"resolved"`
	Label    string `json:"label"`
}

// Draft is the recognizer's answer.
type Draft struct {
	Kind         string // cloud | local
	Provider     string
	Product      string
	Action       model.Action
	Tags         []model.Tag
	Title        string
	TitleSource  string
	Resource     string
	API          string
	Facts        [][2]string
	RecognizedBy []string
	Generic      bool
	GuessWord    string
	ReviewedRead bool
	Identity     Identity
	// Network is "" when the call stays inside the no-network sandbox, "A" for
	// a plain cloud CLI, HTTP client or MCP call, "B" when it runs project or
	// third-party code with network (D-029).
	Network             string
	Background          bool
	Opaque              bool
	Protected           []string
	OutsideWorkdirWrite bool
	Loss                []string
	LossTitle           string
}

func (d *Draft) addTag(t model.Tag) {
	for _, x := range d.Tags {
		if x == t {
			return
		}
	}
	d.Tags = append(d.Tags, t)
	sort.Slice(d.Tags, func(i, j int) bool { return d.Tags[i] < d.Tags[j] })
}

func (d *Draft) by(layer string) {
	for _, x := range d.RecognizedBy {
		if x == layer {
			return
		}
	}
	d.RecognizedBy = append(d.RecognizedBy, layer)
}

// raise makes the draft at least as cautious as a.
func (d *Draft) raise(a model.Action) {
	if a.Caution() > d.Action.Caution() {
		d.Action = a
	}
	if a != model.ActionRead {
		d.ReviewedRead = false
	}
}

// Recognize classifies one call. Adapters (Phase 4) plug in through the
// adapters argument; nil means generic only.
func Recognize(c Call, env Env, adapters *Adapters) Draft {
	var d Draft
	switch c.Via {
	case "shell":
		d = shellDraft(c, env, adapters)
	case "mcp":
		d = mcpDraft(c, adapters)
	case "file":
		d = fileDraft(c, env)
	default:
		d = toolDraft(c, env)
	}
	if d.Title == "" {
		if c.Description != "" {
			d.Title, d.TitleSource = oneLine(c.Description), "agent"
		} else {
			d.Title, d.TitleSource = genericTitle(c), "generic"
		}
	}
	d.Protected = append(d.Protected, protectedTargets(c, env)...)
	if len(d.Protected) > 0 {
		d.Protected = dedupe(d.Protected)
	}
	if len(d.Tags) == 0 {
		d.Tags = []model.Tag{}
	}
	return d
}

func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 140 {
		s = s[:137] + "…"
	}
	return s
}

func genericTitle(c Call) string {
	switch c.Via {
	case "shell":
		return "Run " + oneLine(c.Command)
	case "mcp":
		return fmt.Sprintf("Call %s on %s", c.Tool, c.Server)
	}
	return "Use " + c.Tool
}

// ---------- shell ----------

type cliInfo struct {
	Provider string
	RunsCode bool // runs project code (IaC, deploy with build step)
}

var cloudCLIs = map[string]cliInfo{
	"aws": {Provider: "AWS"}, "gcloud": {Provider: "Google Cloud"}, "gsutil": {Provider: "Google Cloud"},
	"bq": {Provider: "Google Cloud"}, "az": {Provider: "Azure"}, "stripe": {Provider: "Stripe"},
	"wrangler": {Provider: "Cloudflare"}, "vercel": {Provider: "Vercel"}, "netlify": {Provider: "Netlify"},
	"flyctl": {Provider: "Fly.io"}, "fly": {Provider: "Fly.io"}, "heroku": {Provider: "Heroku"},
	"doctl": {Provider: "DigitalOcean"}, "gh": {Provider: "GitHub"}, "supabase": {Provider: "Supabase"},
	"firebase": {Provider: "Firebase"}, "kubectl": {Provider: "Kubernetes"}, "helm": {Provider: "Kubernetes"},
	"railway": {Provider: "Railway"}, "render": {Provider: "Render"}, "resend": {Provider: "Resend"},
	"terraform": {Provider: "Terraform", RunsCode: true}, "tofu": {Provider: "OpenTofu", RunsCode: true},
	"pulumi": {Provider: "Pulumi", RunsCode: true}, "cdk": {Provider: "AWS", RunsCode: true},
	"cdktf": {Provider: "Terraform", RunsCode: true}, "sam": {Provider: "AWS", RunsCode: true},
	"serverless": {Provider: "Serverless", RunsCode: true}, "sls": {Provider: "Serverless", RunsCode: true},
}

var httpClients = map[string]bool{"curl": true, "wget": true, "http": true, "https": true, "xh": true}

var codeRunners = map[string]bool{"node": true, "npx": true, "npm": true, "pnpm": true, "yarn": true, "bun": true,
	"bunx": true, "deno": true, "python": true, "python3": true, "pip": true, "pip3": true, "uv": true, "uvx": true,
	"poetry": true, "go": true, "cargo": true, "make": true, "ruby": true, "bundle": true, "gem": true, "php": true,
	"composer": true, "java": true, "mvn": true, "gradle": true, "dotnet": true, "perl": true, "rake": true}

var hostProviders = []struct{ suffix, provider string }{
	{"api.stripe.com", "Stripe"}, {"api.resend.com", "Resend"}, {"api.cloudflare.com", "Cloudflare"},
	{"amazonaws.com", "AWS"}, {"api.github.com", "GitHub"}, {"api.vercel.com", "Vercel"},
	{"googleapis.com", "Google Cloud"}, {"api.netlify.com", "Netlify"}, {"api.heroku.com", "Heroku"},
	{"api.digitalocean.com", "DigitalOcean"}, {"supabase.co", "Supabase"}, {"api.fly.io", "Fly.io"},
	{"management.azure.com", "Azure"}, {"api.sendgrid.com", "SendGrid"}, {"api.twilio.com", "Twilio"},
	{"slack.com", "Slack"}, {"api.openai.com", "OpenAI"}, {"api.anthropic.com", "Anthropic"},
}

func shellDraft(c Call, env Env, adapters *Adapters) Draft {
	segs := expand(splitShell(c.Command))
	d := Draft{Kind: "local", Provider: "Local", Product: "Shell", Action: model.ActionRead, ReviewedRead: true,
		Identity: Identity{Resolved: true, Label: "this computer"}, API: "shell", RecognizedBy: []string{}}
	if len(segs) == 0 {
		d.Action, d.ReviewedRead = model.ActionUnknown, false
	}
	cloudSeen := false
	for _, s := range segs {
		sd := segmentDraft(s, c, env, adapters)
		if s.Background {
			d.Background = true
		}
		if s.Opaque {
			d.Opaque = true
		}
		if sd.Kind == "cloud" {
			if !cloudSeen {
				// the first cloud segment names the step
				cloudSeen = true
				d.Kind, d.Provider, d.Product, d.Resource, d.API = "cloud", sd.Provider, sd.Product, sd.Resource, sd.API
				d.Identity = sd.Identity
				d.Title, d.TitleSource = sd.Title, sd.TitleSource
				d.GuessWord = sd.GuessWord
				d.Facts = sd.Facts
				d.Action, d.ReviewedRead = sd.Action, sd.ReviewedRead
			} else {
				d.raise(sd.Action)
				d.ReviewedRead = d.ReviewedRead && sd.ReviewedRead
				if !sd.Identity.Resolved {
					d.Identity = sd.Identity
				}
			}
			d.Generic = d.Generic || sd.Generic
		} else {
			// local segments only matter for caution when the call gets network
			if c.WantsNetwork || cloudSeen {
				d.raise(sd.Action)
				d.ReviewedRead = d.ReviewedRead && sd.ReviewedRead
			}
		}
		for _, t := range sd.Tags {
			d.addTag(t)
		}
		for _, l := range sd.RecognizedBy {
			d.by(l)
		}
		if sd.Network > d.Network {
			d.Network = sd.Network
		}
	}
	if c.WantsNetwork && d.Network == "" {
		d.Network = "B" // an unknown program asked for network
	}
	if cloudSeen && d.Network == "" {
		d.Network = "A"
	}
	if d.Network == "B" {
		d.Kind = "cloud"
		if d.Provider == "Local" {
			d.Provider, d.Product = "Unknown", "Program"
		}
		d.raise(model.ActionUnknown)
		d.Identity.Resolved = false
		d.Identity.Label = "unknown"
	}
	if d.Opaque && d.Network != "" {
		d.raise(model.ActionUnknown)
	}
	if len(d.RecognizedBy) == 0 {
		d.by("local")
	}
	if d.Kind == "cloud" && c.Description != "" && d.TitleSource != "adapter" {
		d.Title, d.TitleSource = oneLine(c.Description), "agent"
	}
	return d
}

func segmentDraft(s segment, c Call, env Env, adapters *Adapters) Draft {
	if s.Prog == "" {
		return Draft{Kind: "local", Action: model.ActionRead, ReviewedRead: true}
	}
	if adapters != nil {
		if ad, ok := adapters.matchShell(s); ok {
			generic := genericCloud(s, env)
			mergeCautious(&ad, generic)
			return ad
		}
	}
	if _, ok := cloudCLIs[s.Prog]; ok {
		return genericCloud(s, env)
	}
	if httpClients[s.Prog] {
		return httpDraft(s, env)
	}
	if s.Prog == "git" {
		return gitDraft(s, c)
	}
	if codeRunners[s.Prog] || strings.HasPrefix(s.ProgPath, "./") || strings.HasPrefix(s.ProgPath, "../") || isShell(s.Prog) {
		d := Draft{Kind: "local", Provider: "Local", Product: "Program", Action: model.ActionUnknown, Identity: Identity{Resolved: true}}
		d.by("local")
		if c.WantsNetwork {
			d.Network = "B"
		}
		return d
	}
	// Anything else runs inside the no-network sandbox unless it asks for network.
	d := Draft{Kind: "local", Provider: "Local", Product: "Shell", Action: model.ActionRead, ReviewedRead: true, Identity: Identity{Resolved: true}}
	d.by("local")
	if c.WantsNetwork {
		d.Action, d.ReviewedRead, d.Network = model.ActionUnknown, false, "B"
	}
	return d
}

// mergeCautious folds the generic layer's view into an adapter's (D-026).
func mergeCautious(ad *Draft, g Draft) {
	ad.raise(g.Action)
	if g.Action.Caution() > model.ActionRead.Caution() {
		ad.ReviewedRead = false
	}
	for _, t := range g.Tags {
		ad.addTag(t)
	}
	if !g.Identity.Resolved {
		ad.Identity = g.Identity
	}
	if g.Network > ad.Network {
		ad.Network = g.Network
	}
	ad.by("generic")
}

// ---------- verbs ----------

var readVerbs = map[string]bool{"list": true, "ls": true, "get": true, "describe": true, "show": true, "view": true,
	"status": true, "whoami": true, "info": true, "version": true, "inspect": true, "logs": true, "log": true,
	"retrieve": true, "history": true, "help": true, "head": true, "search": true, "lookup": true, "wait": true,
	"query": true, "find": true, "read": true, "fetch": true, "explain": true, "top": true, "diff": true, "tail": true,
	"cat": true, "print": true, "can-i": true, "check": true, "validate": true, "verify": true, "preview": true}

var deleteWords = map[string]bool{"delete": true, "remove": true, "rm": true, "rb": true, "destroy": true,
	"terminate": true, "purge": true, "deregister": true, "drop": true, "truncate": true, "wipe": true, "erase": true,
	"uninstall": true, "unpublish": true, "revoke": true, "cancel": true, "kill": true, "teardown": true, "nuke": true,
	"empty": true, "release": true, "disable": true, "detach": true, "deactivate": true, "decommission": true,
	"rmdir": true, "prune": true, "evict": true, "abort": true, "expire": true, "void": true, "archive": true}

var sendWords = map[string]bool{"send": true, "email": true, "broadcast": true, "notify": true, "sms": true,
	"publish": true, "invite": true, "message": true, "post-message": true, "announce": true, "blast": true, "dispatch": true}

var moneyWords = map[string]bool{"refund": true, "refunds": true, "charge": true, "charges": true, "payout": true,
	"payouts": true, "transfer": true, "transfers": true, "pay": true, "capture": true, "topup": true, "topups": true,
	"credit": true, "debit": true, "purchase": true, "buy": true, "subscribe": true, "subscriptions": true}

var createWords = map[string]bool{"create": true, "put": true, "add": true, "new": true, "register": true, "deploy": true,
	"apply": true, "up": true, "run": true, "start": true, "upload": true, "cp": true, "sync": true, "import": true,
	"copy": true, "launch": true, "provision": true, "init": true, "allocate": true, "issue": true, "generate": true,
	"invoke": true, "exec": true, "execute": true, "trigger": true, "push": true, "install": true, "submit": true,
	"post": true, "build": true, "clone": true, "fork": true, "request": true}

var changeWords = map[string]bool{"update": true, "set": true, "modify": true, "patch": true, "edit": true,
	"configure": true, "enable": true, "attach": true, "associate": true, "tag": true, "label": true, "move": true,
	"mv": true, "rename": true, "rotate": true, "reboot": true, "restart": true, "scale": true, "resize": true,
	"upgrade": true, "rollback": true, "replace": true, "change": true, "assign": true, "authorize": true,
	"grant": true, "promote": true, "merge": true, "close": true, "reopen": true, "approve": true, "rerun": true,
	"stop": true, "suspend": true, "pause": true, "resume": true, "lock": true, "unlock": true, "reset": true}

// words splits a token like "terminate-instances" or "sendBroadcast" into
// lower-case words.
func words(tok string) []string {
	var out []string
	var cur []rune
	rs := []rune(tok)
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	for i, r := range rs {
		switch {
		case r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '=' || r == ' ' || r == ',':
			flush()
		case r >= 'A' && r <= 'Z' && i > 0 && rs[i-1] >= 'a' && rs[i-1] <= 'z':
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

// singular folds simple plurals ("emails", "refunds") onto the word lists.
func singular(w string) string {
	if deleteWords[w] || sendWords[w] || moneyWords[w] || createWords[w] || changeWords[w] || readVerbs[w] {
		return w
	}
	for _, suf := range []string{"es", "s"} {
		if strings.HasSuffix(w, suf) {
			b := strings.TrimSuffix(w, suf)
			if deleteWords[b] || sendWords[b] || moneyWords[b] {
				return b
			}
		}
	}
	return w
}

// verbAction maps one word to an action ("" when it says nothing).
func verbAction(w string) model.Action {
	switch {
	case deleteWords[w]:
		return model.ActionDelete
	case sendWords[w]:
		return model.ActionSend
	case changeWords[w]:
		return model.ActionChange
	case createWords[w]:
		return model.ActionCreate
	case readVerbs[w]:
		return model.ActionRead
	}
	return ""
}

// scanWarnings applies warning words found anywhere in toks to d (D-026).
func scanWarnings(d *Draft, toks []string) {
	for _, t := range toks {
		if len(t) > 64 {
			continue // long free text (an email body) is not a command word
		}
		for _, w := range words(t) {
			w = singular(w)
			switch {
			case deleteWords[w]:
				d.raise(model.ActionDelete)
				if d.GuessWord == "" {
					d.GuessWord = w
				}
			case sendWords[w]:
				d.raise(model.ActionSend)
				d.addTag(model.TagPeople)
				d.addTag(model.TagIrreversible)
				if d.GuessWord == "" {
					d.GuessWord = w
				}
			case moneyWords[w]:
				d.addTag(model.TagMoney)
				if w == "refund" || w == "refunds" || w == "payout" || w == "payouts" || w == "transfer" || w == "transfers" || w == "charge" || w == "charges" || w == "capture" {
					d.addTag(model.TagIrreversible)
				}
				d.raise(model.ActionCreate)
				if d.GuessWord == "" {
					d.GuessWord = w
				}
			}
		}
	}
}

// ---------- cloud CLIs (generic layer) ----------

var awsProducts = map[string]string{"s3": "S3", "s3api": "S3", "ec2": "EC2", "iam": "IAM", "sts": "Account",
	"cloudfront": "CloudFront", "lambda": "Lambda", "rds": "RDS", "dynamodb": "DynamoDB", "sns": "SNS", "sqs": "SQS",
	"cloudwatch": "CloudWatch", "logs": "CloudWatch Logs", "ecs": "ECS", "eks": "EKS", "ecr": "ECR",
	"route53": "Route 53", "secretsmanager": "Secrets Manager", "ssm": "Systems Manager", "cloudformation": "CloudFormation",
	"apigateway": "API Gateway", "kms": "KMS", "acm": "ACM", "elbv2": "Load Balancers", "ses": "SES", "sesv2": "SES",
	"organizations": "Organizations", "budgets": "Budgets", "ce": "Cost Explorer"}

func genericCloud(s segment, env Env) Draft {
	info := cloudCLIs[s.Prog]
	pos := positional(s.Args)
	d := Draft{Kind: "cloud", Provider: info.Provider, Generic: true, Network: "A", Identity: identityFor(s, env)}
	d.by("generic")
	if info.RunsCode {
		d.Network = "B"
	}
	// product: the first subcommand
	if len(pos) > 0 {
		d.Product = pos[0]
		if s.Prog == "aws" {
			if p, ok := awsProducts[pos[0]]; ok {
				d.Product = p
			} else {
				d.Product = strings.ToUpper(pos[0][:1]) + pos[0][1:]
			}
		}
	} else {
		d.Product = s.Prog
	}
	// the operation word(s): everything positional after the product
	ops := pos
	if len(pos) > 1 {
		ops = pos[1:]
	}
	d.API = s.Prog + " " + strings.Join(firstN(pos, 3), " ")
	if s.Prog == "aws" && len(pos) >= 2 {
		d.API = pos[0] + ":" + pascal(pos[1])
	}
	d.Action = model.ActionUnknown
	opVerb := ""
	for i := len(ops) - 1; i >= 0 && opVerb == ""; i-- { // the last positional verb usually decides (gcloud x y delete)
		for _, w := range words(ops[i]) {
			if a := verbAction(w); a != "" {
				opVerb = w
				d.Action = a
				break
			}
		}
	}
	if s.Prog == "aws" && len(pos) >= 2 {
		// aws <service> <operation>: the operation's first word decides (D-012 S4 rule)
		ws := words(pos[1])
		if len(ws) > 0 {
			if a := verbAction(ws[0]); a != "" {
				opVerb, d.Action = ws[0], a
			} else {
				opVerb, d.Action = ws[0], model.ActionUnknown
			}
		}
		if pos[0] == "s3" { // high-level aws s3 commands
			switch pos[1] {
			case "ls":
				d.Action, opVerb = model.ActionRead, "ls"
			case "rm", "rb":
				d.Action, opVerb = model.ActionDelete, pos[1]
			case "cp", "sync":
				d.Action, opVerb = model.ActionCreate, pos[1]
			case "mv":
				d.Action, opVerb = model.ActionChange, "mv"
			}
		}
	}
	// stripe/gh raw API subcommands carry the HTTP method
	if (s.Prog == "stripe" && len(pos) > 0) || (s.Prog == "gh" && len(pos) > 0 && pos[0] == "api") {
		method := ""
		if s.Prog == "stripe" {
			switch pos[0] {
			case "get":
				method = "GET"
			case "post":
				method = "POST"
			case "delete":
				method = "DELETE"
			}
		} else {
			method = "GET"
			if m, ok := flagValue(s.Args, "-X", "--method"); ok {
				method = strings.ToUpper(m)
			} else if hasFlag(s.Args, "-f", "-F", "--field", "--raw-field", "--input") {
				method = "POST"
			}
		}
		if method != "" {
			d.Action, opVerb = methodAction(method), strings.ToLower(method)
		}
	}
	d.GuessWord = opVerb
	d.ReviewedRead = d.Action == model.ActionRead && readVerbs[opVerb]
	if hasFlag(s.Args, "--delete") || hasFlag(s.Args, "--force-delete") || hasFlag(s.Args, "--purge") {
		d.raise(model.ActionDelete)
	}
	scanWarnings(&d, scanTokens(s.Args))
	if isLive(s) {
		d.addTag(model.TagLive)
	}
	if d.Action == model.ActionDelete {
		d.addTag(model.TagIrreversible)
	}
	d.Resource = resourceOf(s.Args)
	if d.Resource == "" && len(pos) > 2 {
		d.Resource = pos[len(pos)-1]
	}
	if d.Action == model.ActionRead && !d.ReviewedRead {
		d.Generic = true
	}
	if info.RunsCode {
		d.raise(model.ActionUnknown)
		d.ReviewedRead = false
	}
	return d
}

// freeTextFlags carry prose, not operations, so their values aren't scanned
// for warning words.
var freeTextFlags = map[string]bool{"--description": true, "--comment": true, "--message": true, "-m": true,
	"--subject": true, "--body": true, "--text": true, "--title": true, "--notes": true, "--memo": true, "--reason": true}

// scanTokens returns every argument except free-text flag values (D-026:
// warning words anywhere in the call count).
func scanTokens(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		if j := strings.IndexByte(a, '='); j > 0 && strings.HasPrefix(a, "-") {
			name = a[:j]
		}
		if freeTextFlags[name] {
			if !strings.Contains(a, "=") {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func flagNames(args []string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if i := strings.IndexByte(a, '='); i > 0 {
				a = a[:i]
			}
			out = append(out, a)
		}
	}
	return out
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func pascal(kebab string) string {
	var b strings.Builder
	for _, p := range strings.Split(kebab, "-") {
		if p != "" {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

func methodAction(m string) model.Action {
	switch strings.ToUpper(m) {
	case "GET", "HEAD", "OPTIONS":
		return model.ActionRead
	case "POST":
		return model.ActionCreate
	case "PUT", "PATCH":
		return model.ActionChange
	case "DELETE":
		return model.ActionDelete
	}
	return model.ActionUnknown
}

// isLive reports explicit live/production markers (section 4.4).
func isLive(s segment) bool {
	if hasFlag(s.Args, "--live", "--prod", "--production") {
		return true
	}
	for _, f := range []string{"--env", "--environment", "-e", "--stage", "--target"} {
		if v, ok := flagValue(s.Args, f); ok {
			v = strings.ToLower(v)
			if v == "production" || v == "prod" || v == "live" {
				return true
			}
		}
	}
	for k, v := range s.Env {
		if strings.Contains(v, "sk_live_") || strings.Contains(v, "rk_live_") {
			return true
		}
		if (k == "STAGE" || k == "ENVIRONMENT" || k == "APP_ENV") && (v == "production" || v == "prod") {
			return true
		}
	}
	for _, a := range s.Args {
		if strings.Contains(a, "sk_live_") || strings.Contains(a, "rk_live_") {
			return true
		}
	}
	return false
}

func identityFor(s segment, env Env) Identity {
	look := func(flags []string, envs []string) (string, bool) {
		if v, ok := flagValue(s.Args, flags...); ok {
			return v, true
		}
		for _, e := range envs {
			if v, ok := s.Env[e]; ok {
				return v, true
			}
			if v, ok := env.AgentEnv[e]; ok {
				return v, true
			}
		}
		return "", false
	}
	var v string
	var ok bool
	switch s.Prog {
	case "aws", "cdk", "sam":
		v, ok = look([]string{"--profile"}, []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE"})
		if !ok {
			if _, k := s.Env["AWS_ACCESS_KEY_ID"]; k {
				return Identity{Resolved: false, Label: "keys given in the command"}
			}
			v, ok = "default", true
		}
		v = "profile " + v
	case "gcloud", "gsutil", "bq":
		v, ok = look([]string{"--account", "--project", "--configuration"}, []string{"CLOUDSDK_CORE_PROJECT", "CLOUDSDK_CORE_ACCOUNT", "CLOUDSDK_ACTIVE_CONFIG_NAME"})
		if !ok {
			v, ok = "default configuration", true
		}
	case "stripe":
		if k, has := flagValue(s.Args, "--api-key"); has {
			v, ok = k, true
			if strings.Contains(k, "$") {
				return Identity{Resolved: false, Label: "key from a variable"}
			}
			v = "given key"
		} else {
			v, ok = "logged-in account", true
		}
	case "kubectl", "helm":
		v, ok = look([]string{"--context", "--kube-context"}, []string{"KUBECONFIG"})
		if !ok {
			v, ok = "current context", true
		}
	default:
		v, ok = "logged-in account", true
	}
	if strings.ContainsAny(v, "$`") {
		return Identity{Resolved: false, Label: "set by a variable"}
	}
	return Identity{Resolved: ok, Label: v}
}

func resourceOf(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "s3://") || strings.HasPrefix(a, "arn:") || strings.HasPrefix(a, "gs://") {
			return a
		}
	}
	for _, f := range []string{"--bucket", "--function-name", "--instance-ids", "--id", "--name", "--stack-name",
		"--table-name", "--db-instance-identifier", "--cluster", "--service", "--topic-arn", "--zone", "--project-name", "--resource"} {
		if v, ok := flagValue(args, f); ok {
			return v
		}
	}
	return ""
}

// ---------- HTTP clients ----------

func httpDraft(s segment, env Env) Draft {
	method := ""
	if m, ok := flagValue(s.Args, "-X", "--request", "--method"); ok {
		method = strings.ToUpper(m)
	} else if hasFlag(s.Args, "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode", "--json", "-F", "--form", "--post-data", "--post-file") {
		method = "POST"
	} else if hasFlag(s.Args, "-T", "--upload-file") {
		method = "PUT"
	} else if hasFlag(s.Args, "-I", "--head") {
		method = "HEAD"
	}
	if s.Prog == "http" || s.Prog == "https" || s.Prog == "xh" {
		// httpie style: http [METHOD] URL
		pos := positional(s.Args)
		if len(pos) > 0 && isMethod(pos[0]) {
			method = strings.ToUpper(pos[0])
		}
	}
	if method == "" {
		method = "GET"
	}
	raw := ""
	for _, a := range s.Args {
		if strings.Contains(a, "://") || strings.HasPrefix(a, "localhost") || strings.HasPrefix(a, "127.0.0.1") {
			raw = a
			break
		}
	}
	u, _ := url.Parse(raw)
	if u != nil && u.Host == "" && raw != "" {
		u, _ = url.Parse("http://" + raw)
	}
	host := ""
	path := ""
	if u != nil {
		host, path = u.Hostname(), u.Path
	}
	d := Draft{Kind: "cloud", Provider: "Web", Product: host, Network: "A", Generic: true,
		Identity: Identity{Resolved: true, Label: "credentials in the request"}, Resource: raw,
		API: method + " " + host + path}
	d.by("generic")
	for _, hp := range hostProviders {
		if host == hp.suffix || strings.HasSuffix(host, "."+hp.suffix) {
			d.Provider = hp.provider
			d.Product = productFromPath(path)
			break
		}
	}
	d.Action = methodAction(method)
	d.GuessWord = strings.ToLower(method)
	// A GET to a known provider API is a reviewed read; to anywhere else it
	// still sends data out, so it asks.
	d.ReviewedRead = d.Action == model.ActionRead && d.Provider != "Web"
	scanWarnings(&d, strings.Split(path, "/"))
	if d.Action == model.ActionCreate && d.Provider != "Web" {
		// POST to an API: what the path names decides send / money
		scanWarnings(&d, strings.Split(path, "/"))
	}
	if strings.Contains(strings.Join(s.Args, " "), "sk_live_") || strings.Contains(strings.Join(s.Args, " "), "livemode") {
		d.addTag(model.TagLive)
	}
	for _, a := range s.Args {
		if strings.Contains(a, "$") && (strings.Contains(strings.ToLower(a), "authorization") || strings.Contains(a, "-u")) {
			d.Identity = Identity{Resolved: false, Label: "credentials from a variable"}
		}
	}
	if d.Action == model.ActionDelete {
		d.addTag(model.TagIrreversible)
	}
	return d
}

func isMethod(s string) bool {
	switch strings.ToUpper(s) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

func productFromPath(p string) string {
	for _, part := range strings.Split(strings.Trim(p, "/"), "/") {
		if part == "" || part == "v1" || part == "v2" || part == "v3" || part == "v4" || part == "client" {
			continue
		}
		return strings.ToUpper(part[:1]) + part[1:]
	}
	return "API"
}

// ---------- git ----------

var gitNetwork = map[string]bool{"push": true, "pull": true, "fetch": true, "clone": true, "ls-remote": true, "submodule": true, "remote": true}

func gitDraft(s segment, c Call) Draft {
	pos := positional(s.Args)
	d := Draft{Kind: "local", Provider: "Local", Product: "Git", Action: model.ActionRead, ReviewedRead: true, Identity: Identity{Resolved: true}}
	d.by("local")
	if len(pos) > 0 && gitNetwork[pos[0]] && c.WantsNetwork {
		// git runs repository hooks, so with network it counts as running project code
		d.Kind, d.Provider, d.Product, d.Network = "cloud", "Git remote", pos[0], "B"
		d.Action, d.ReviewedRead = model.ActionUnknown, false
		if pos[0] == "push" && (hasFlag(s.Args, "--force", "-f", "--force-with-lease", "--delete", "-d") || strings.HasPrefix(strings.Join(pos, " "), "push :")) {
			d.raise(model.ActionDelete)
		}
	}
	return d
}

// ---------- MCP ----------

var mcpProviders = []struct{ key, name string }{
	{"stripe", "Stripe"}, {"resend", "Resend"}, {"cloudflare", "Cloudflare"}, {"aws", "AWS"}, {"github", "GitHub"},
	{"supabase", "Supabase"}, {"vercel", "Vercel"}, {"linear", "Linear"}, {"notion", "Notion"}, {"slack", "Slack"},
	{"sentry", "Sentry"}, {"gcp", "Google Cloud"}, {"google", "Google"}, {"netlify", "Netlify"}, {"neon", "Neon"},
}

func mcpProvider(server string) string {
	l := strings.ToLower(server)
	for _, p := range mcpProviders {
		if strings.Contains(l, p.key) {
			return p.name
		}
	}
	if server == "" {
		return "MCP"
	}
	return strings.ToUpper(server[:1]) + server[1:]
}

func mcpDraft(c Call, adapters *Adapters) Draft {
	d := Draft{Kind: "cloud", Provider: mcpProvider(c.Server), Product: c.Server, Network: "A", Generic: true,
		API: c.Server + " · " + c.Tool, Identity: Identity{Resolved: false, Label: "the MCP server's account"}}
	d.by("generic")
	ws := words(c.Tool)
	d.Action = model.ActionUnknown
	if len(ws) > 0 {
		if a := verbAction(ws[0]); a != "" {
			d.Action, d.GuessWord = a, ws[0]
		}
	}
	readOnly := c.Hints != nil && c.Hints.ReadOnly != nil && *c.Hints.ReadOnly
	destructive := c.Hints != nil && c.Hints.Destructive != nil && *c.Hints.Destructive
	if c.Hints != nil {
		d.by("mcp-hints")
	}
	// Two signals must agree before an MCP call counts as a read: a read verb
	// in the name and the server's own readOnlyHint (a hint alone never does).
	d.ReviewedRead = d.Action == model.ActionRead && readOnly
	var toks []string
	toks = append(toks, c.Tool)
	for k, v := range c.Args {
		toks = append(toks, k)
		if sv, ok := v.(string); ok {
			toks = append(toks, sv)
		}
		if bv, ok := v.(bool); ok && k == "livemode" && bv {
			d.addTag(model.TagLive)
		}
	}
	scanWarnings(&d, toks)
	if destructive {
		d.raise(model.ActionDelete)
		d.addTag(model.TagIrreversible)
	}
	if c.Hints != nil && c.Hints.ReadOnly != nil && !*c.Hints.ReadOnly && d.Action == model.ActionRead {
		d.raise(model.ActionUnknown) // the server says it isn't read-only
	}
	if adapters != nil {
		if ad, ok := adapters.matchMCP(c); ok {
			mergeCautious(&ad, d)
			return ad
		}
	}
	return d
}

// ---------- file and other tools ----------

func fileDraft(c Call, env Env) Draft {
	d := Draft{Kind: "local", Provider: "Local", Product: "Files", Action: model.ActionRead, ReviewedRead: true,
		Identity: Identity{Resolved: true, Label: "this computer"}, API: c.Tool, Resource: strings.Join(c.Paths, ", ")}
	d.by("local")
	switch c.Tool {
	case "Read", "Glob", "Grep", "LS", "NotebookRead":
	default:
		d.Action, d.ReviewedRead = model.ActionChange, false
		for _, p := range c.Paths {
			if !insideAny(resolve(p, c.Cwd), []string{env.Workdir, "/tmp", "/private/tmp", "/var/folders", "/private/var/folders"}) {
				d.OutsideWorkdirWrite = true
			}
		}
	}
	return d
}

var localTools = map[string]bool{"TodoWrite": true, "ToolSearch": true, "Task": true, "Agent": true,
	"ExitPlanMode": true, "BashOutput": true, "KillShell": true, "KillBash": true, "TaskOutput": true}

func toolDraft(c Call, env Env) Draft {
	d := Draft{Kind: "local", Provider: "Local", Product: c.Tool, Action: model.ActionRead, ReviewedRead: true,
		Identity: Identity{Resolved: true}, API: c.Tool}
	d.by("local")
	switch {
	case localTools[c.Tool]:
	case c.Tool == "WebSearch":
		d.Network = "A"
	case c.Tool == "WebFetch":
		d.Network = "A"
		raw, _ := c.Args["url"].(string)
		d.Resource = raw
		if u, err := url.Parse(raw); err == nil {
			d.Product = u.Hostname()
		}
	default:
		// a tool CloudBuddy doesn't know: it isn't covered, so it asks
		d.Action, d.ReviewedRead, d.Opaque = model.ActionUnknown, false, true
		d.Network = "A"
	}
	return d
}

// ---------- protected targets (D-027) ----------

func protectedPaths(env Env) []string {
	var ps []string
	if env.DataDir != "" {
		ps = append(ps, env.DataDir)
	}
	if env.Home != "" {
		ps = append(ps, filepath.Join(env.Home, ".claude", "settings.json"), filepath.Join(env.Home, ".claude", "settings.local.json"),
			filepath.Join(env.Home, ".claude.json"), filepath.Join(env.Home, ".claude", ".credentials.json"),
			filepath.Join(env.Home, ".codex", "config.toml"), filepath.Join(env.Home, ".codex", "auth.json"),
			filepath.Join(env.Home, ".codex", "rules"), filepath.Join(env.Home, ".cloudbuddy"))
	}
	if env.CloudBuddyBin != "" {
		ps = append(ps, env.CloudBuddyBin)
	}
	return append(ps, env.Protected...)
}

func protectedTargets(c Call, env Env) []string {
	var hits []string
	prot := protectedPaths(env)
	check := func(p string) {
		if p == "" {
			return
		}
		rp := resolve(p, c.Cwd)
		for _, q := range prot {
			if rp == q || strings.HasPrefix(rp, q+string(filepath.Separator)) {
				hits = append(hits, q)
			}
		}
	}
	for _, p := range c.Paths {
		check(p)
	}
	text := c.Command
	if c.Via != "shell" {
		for _, v := range c.Args {
			if s, ok := v.(string); ok {
				text += " " + s
			}
		}
	}
	if text != "" {
		for _, q := range prot {
			if strings.Contains(text, q) || (env.Home != "" && strings.Contains(text, strings.Replace(q, env.Home, "~", 1))) || strings.Contains(text, strings.Replace(q, env.Home, "$HOME", 1)) {
				hits = append(hits, q)
			}
		}
		if env.Port != 0 {
			for _, h := range []string{"127.0.0.1", "localhost", "[::1]", "0.0.0.0"} {
				if strings.Contains(text, fmt.Sprintf("%s:%d", h, env.Port)) {
					hits = append(hits, fmt.Sprintf("CloudBuddy's port %d", env.Port))
				}
			}
		}
		if strings.Contains(text, "cloudbuddy exec") || strings.Contains(text, "cloudbuddy hook") ||
			strings.Contains(text, "cloudbuddy mcp-gateway") || strings.Contains(text, "CB_RELEASE") ||
			(env.CloudBuddyBin != "" && strings.Contains(text, filepath.Base(env.CloudBuddyBin)+" exec")) {
			hits = append(hits, "CloudBuddy's own programs")
		}
	}
	return hits
}

func resolve(p, cwd string) string {
	if strings.HasPrefix(p, "~/") {
		// the caller expands ~ via env.Home in protectedTargets text checks;
		// here we only need a stable absolute form
		return p
	}
	if !filepath.IsAbs(p) && cwd != "" {
		p = filepath.Join(cwd, p)
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return rp
	}
	// the file may not exist yet: resolve its folder
	if dir, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(dir, filepath.Base(p))
	}
	return filepath.Clean(p)
}

func insideAny(p string, roots []string) bool {
	for _, r := range roots {
		if r == "" {
			continue
		}
		rr := r
		if x, err := filepath.EvalSymlinks(r); err == nil {
			rr = x
		}
		if p == rr || strings.HasPrefix(p, rr+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
