package recognize

import (
	"testing"

	"cloudbuddy/internal/model"
)

var testEnv = Env{Home: "/Users/test", DataDir: "/Users/test/.cloudbuddy", Port: 7420, Workdir: "/work"}

func sh(cmd string, net bool) Draft {
	return Recognize(Call{Agent: "claude", Via: "shell", Command: cmd, WantsNetwork: net, Cwd: "/work"}, testEnv, nil)
}

func TestSplitShell(t *testing.T) {
	cases := []struct {
		cmd   string
		progs []string
	}{
		{"ls -la && aws s3 ls", []string{"ls", "aws"}},
		{"cd x; FOO=1 aws s3 rb s3://b | tee log", []string{"cd", "aws", "tee"}},
		{`echo "a && b" && rm -rf x`, []string{"echo", "rm"}},
		{"sudo -E aws ec2 describe-instances", []string{"aws"}},
		{"timeout 30 aws s3 ls", []string{"aws"}},
		{"env AWS_PROFILE=prod aws s3 ls", []string{"aws"}},
		{"echo $(aws sts get-caller-identity)", []string{"aws", "echo"}},
		{"echo `whoami`", []string{"whoami", "echo"}},
		{"(cd infra && terraform apply)", []string{"cd", "terraform"}},
		{"cmd1 || cmd2", []string{"cmd1", "cmd2"}},
		{"curl -s https://x 2>&1 | jq .", []string{"curl", "jq"}},
	}
	for _, c := range cases {
		segs := expand(splitShell(c.cmd))
		var got []string
		for _, s := range segs {
			got = append(got, s.Prog)
		}
		if len(got) != len(c.progs) {
			t.Errorf("%q: got %v want %v", c.cmd, got, c.progs)
			continue
		}
		for i := range got {
			if got[i] != c.progs[i] {
				t.Errorf("%q: got %v want %v", c.cmd, got, c.progs)
				break
			}
		}
	}
}

func TestShellWrapperIsExpanded(t *testing.T) {
	d := sh(`/bin/zsh -lc 'cd x && aws s3 rb s3://old --force'`, true)
	if d.Action != model.ActionDelete || d.Provider != "AWS" {
		t.Fatalf("zsh -lc wrapper not looked into: %+v", d)
	}
}

func TestCompoundTakesMostCautious(t *testing.T) {
	d := sh("aws s3 ls && aws s3 rb s3://old", true)
	if d.Action != model.ActionDelete || d.ReviewedRead {
		t.Fatalf("read then delete should be delete: action=%s reviewed=%v", d.Action, d.ReviewedRead)
	}
}

func TestBackgroundDetected(t *testing.T) {
	for _, c := range []string{"aws s3 sync . s3://b &", "nohup aws s3 sync . s3://b", "setsid curl https://x", "(curl https://x &)"} {
		if d := sh(c, true); !d.Background {
			t.Errorf("%q: background not detected", c)
		}
	}
	if d := sh("cmd 2>&1", true); d.Background {
		t.Errorf("2>&1 taken as background")
	}
}

func TestOpaqueConstructs(t *testing.T) {
	for _, c := range []string{`eval "$CMD"`, "$(echo aws) s3 rb s3://x", "cat <<EOF | sh\naws s3 rb s3://x\nEOF", "xargs -n1 aws s3 rb"} {
		d := sh(c, true)
		if !d.Opaque && d.Action != model.ActionDelete {
			t.Errorf("%q: neither opaque nor delete: %+v", c, d)
		}
	}
}

func TestAWSOperations(t *testing.T) {
	cases := map[string]model.Action{
		"aws ec2 describe-instances":                        model.ActionRead,
		"aws s3api list-buckets":                            model.ActionRead,
		"aws s3api create-bucket --bucket b":                model.ActionCreate,
		"aws s3api put-bucket-policy --bucket b":            model.ActionCreate,
		"aws ec2 modify-instance-attribute --instance-id i": model.ActionChange,
		"aws ec2 terminate-instances --instance-ids i-1":    model.ActionDelete,
		"aws s3 rm s3://b/k":                                model.ActionDelete,
		"aws s3 sync ./dist s3://b --delete":                model.ActionDelete,
		"aws ec2 batch-thing":                               model.ActionUnknown,
	}
	for cmd, want := range cases {
		if d := sh(cmd, true); d.Action != want {
			t.Errorf("%q: got %s want %s", cmd, d.Action, want)
		}
	}
	if d := sh("aws ec2 terminate-instances --instance-ids i-1", true); d.API != "ec2:TerminateInstances" || d.Product != "EC2" {
		t.Errorf("api/product: %q %q", d.API, d.Product)
	}
}

func TestWarningWordInsideNameRaisesCaution(t *testing.T) {
	// "describe" is a read verb, but the bucket name carries a delete word:
	// the most cautious reading wins (D-026).
	d := sh("aws s3api describe-thing --bucket delete-me", true)
	if d.ReviewedRead {
		t.Fatalf("warning word ignored: %+v", d)
	}
}

func TestLiveMarkers(t *testing.T) {
	for _, c := range []string{"stripe prices create --live", "vercel deploy --prod", "wrangler deploy --env production",
		"STRIPE_API_KEY=sk_live_abc123 stripe customers list", "serverless deploy --stage prod"} {
		d := sh(c, true)
		live := false
		for _, t := range d.Tags {
			if t == model.TagLive {
				live = true
			}
		}
		if !live {
			t.Errorf("%q: live not detected", c)
		}
	}
}

func TestIdentity(t *testing.T) {
	if d := sh("aws s3 ls --profile prod", true); !d.Identity.Resolved || d.Identity.Label != "profile prod" {
		t.Errorf("profile flag: %+v", d.Identity)
	}
	if d := sh("aws s3 ls --profile $P", true); d.Identity.Resolved {
		t.Errorf("variable profile should be unresolved")
	}
	if d := sh("AWS_ACCESS_KEY_ID=x AWS_SECRET_ACCESS_KEY=y aws s3 ls", true); d.Identity.Resolved {
		t.Errorf("inline keys should be unresolved")
	}
}

func TestNetworkKinds(t *testing.T) {
	if d := sh("ls -la", false); d.Network != "" || d.Kind != "local" {
		t.Errorf("ls: %+v", d)
	}
	if d := sh("aws s3 ls", false); d.Network != "A" {
		t.Errorf("aws without network flag still needs network: %q", d.Network)
	}
	if d := sh("npm install", true); d.Network != "B" {
		t.Errorf("npm install with network: %q", d.Network)
	}
	if d := sh("terraform apply", true); d.Network != "B" {
		t.Errorf("terraform runs code: %q", d.Network)
	}
	if d := sh("./deploy.sh", true); d.Network != "B" {
		t.Errorf("project script with network: %q", d.Network)
	}
	if d := sh("git push origin main", true); d.Network != "B" {
		t.Errorf("git push with network: %q", d.Network)
	}
	if d := sh("npm run build", false); d.Network != "" {
		t.Errorf("build inside sandbox: %q", d.Network)
	}
}

func TestProtectedTargets(t *testing.T) {
	for _, c := range []string{"cat ~/.cloudbuddy/gate.key", "cat $HOME/.cloudbuddy/x", "curl localhost:7420/api",
		"echo x > /Users/test/.claude/settings.json", "cloudbuddy exec --release x"} {
		if d := sh(c, false); len(d.Protected) == 0 {
			t.Errorf("%q: not protected", c)
		}
	}
	if d := sh("cat README.md", false); len(d.Protected) != 0 {
		t.Errorf("ordinary file protected: %v", d.Protected)
	}
}

func TestHTTPClient(t *testing.T) {
	d := sh(`curl -s -X POST https://api.stripe.com/v1/refunds -d charge=ch_1`, true)
	if d.Provider != "Stripe" || d.Product != "Refunds" {
		t.Errorf("provider/product: %s %s", d.Provider, d.Product)
	}
	money := false
	for _, t := range d.Tags {
		if t == model.TagMoney {
			money = true
		}
	}
	if !money {
		t.Errorf("refund not marked Moves money: %v", d.Tags)
	}
	if d := sh("curl https://api.stripe.com/v1/customers", true); d.ReviewedRead {
		t.Errorf("the generic layer must not review HTTP reads (adapters do)")
	}
}

func TestMCPHintsOnlyAddCaution(t *testing.T) {
	yes, no := true, false
	// readOnlyHint alone never makes a call a read
	d := Recognize(Call{Via: "mcp", Server: "x", Tool: "purge_everything", Hints: &Hints{ReadOnly: &yes}}, testEnv, nil)
	if d.ReviewedRead || d.Action == model.ActionRead {
		t.Errorf("readOnlyHint lowered caution: %+v", d)
	}
	// a server saying it isn't read-only cancels a read verb
	d = Recognize(Call{Via: "mcp", Server: "x", Tool: "get_thing", Hints: &Hints{ReadOnly: &no}}, testEnv, nil)
	if d.ReviewedRead {
		t.Errorf("readOnlyHint=false ignored")
	}
}

func TestFileTools(t *testing.T) {
	d := Recognize(Call{Via: "file", Tool: "Write", Paths: []string{"/work/a.txt"}, Cwd: "/work"}, testEnv, nil)
	if d.OutsideWorkdirWrite || d.Network != "" {
		t.Errorf("write inside workdir: %+v", d)
	}
	d = Recognize(Call{Via: "file", Tool: "Read", Paths: []string{"/Users/test/.cloudbuddy/db"}, Cwd: "/work"}, testEnv, nil)
	if len(d.Protected) == 0 {
		t.Errorf("reading CloudBuddy data not protected")
	}
}

func TestPinnedCLI(t *testing.T) {
	env := testEnv
	env.Pinned = map[string]string{"aws": "/opt/homebrew/bin/aws"}
	for _, c := range []string{"./aws s3 ls", "/tmp/aws s3 ls", "PATH=./bin:$PATH aws s3 ls", "node_modules/.bin/aws s3 ls"} {
		d := Recognize(Call{Via: "shell", Command: c, WantsNetwork: true, Cwd: "/work"}, env, nil)
		if d.Network != "B" || d.ReviewedRead {
			t.Errorf("%q: a copy of aws treated as the real one: net=%q reviewed=%v", c, d.Network, d.ReviewedRead)
		}
	}
	d := Recognize(Call{Via: "shell", Command: "aws s3 ls", WantsNetwork: true, Cwd: "/work"}, env, nil)
	if d.Network != "A" || !d.ReviewedRead {
		t.Errorf("installed aws: net=%q reviewed=%v", d.Network, d.ReviewedRead)
	}
}

func TestScriptStepsAreNamed(t *testing.T) {
	env := Env{Home: "/Users/test", Workdir: "/work"}
	for cmd, want := range map[string]string{
		"make credit-purchases ENV=dev DAYS=7":                   "make credit-purchases",
		"HAZLSKY_PROVIDER_CLI_APPROVED=1 python3 scripts/x.py a": "python3 x.py",
		"cd frontend && pnpm exec wrangler tail web":             "pnpm wrangler",
	} {
		d := Recognize(Call{Via: "shell", Command: cmd, WantsNetwork: true, Cwd: "/work"}, env, nil)
		if d.Provider != "Script" || d.Product != want {
			t.Errorf("%s: %s / %s, want Script / %s", cmd, d.Provider, d.Product, want)
		}
	}
}

func TestProviderForHost(t *testing.T) {
	for host, want := range map[string]string{"api.supabase.com": "Supabase", "abc.supabase.co": "Supabase",
		"sts.us-east-1.amazonaws.com": "AWS", "api.stripe.com:443": "Stripe", "hazl.ca": "", "tail.developers.workers.dev": "Cloudflare"} {
		if got := ProviderForHost(host); got != want {
			t.Errorf("%s: %q, want %q", host, got, want)
		}
	}
}
