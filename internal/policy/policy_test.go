package policy

import (
	"testing"

	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
)

var env = recognize.Env{Home: "/Users/test", DataDir: "/Users/test/.cloudbuddy", Port: 7420, Workdir: "/work"}

func shell(cmd string, net bool) recognize.Draft {
	return recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: cmd, WantsNetwork: net, Cwd: "/work"}, env, nil)
}

func TestPolicyRows(t *testing.T) {
	ro := true
	cases := []struct {
		name    string
		draft   recognize.Draft
		ctx     Context
		verdict Verdict
		row     int
	}{
		{"1 protected data dir", shell("cat /Users/test/.cloudbuddy/state.db", false), Context{CanWrapCode: true}, Deny, 1},
		{"1 protected port", shell("curl -X POST http://127.0.0.1:7420/api/approvals/1", true), Context{CanWrapCode: true}, Deny, 1},
		{"1 own exec wrapper", shell("cloudbuddy exec --release abc", true), Context{CanWrapCode: true}, Deny, 1},
		{"2 background cloud", shell("nohup aws s3 sync . s3://b &", true), Context{CanWrapCode: true}, Deny, 2},
		{"3 local in sandbox", shell("npm run build && ls dist", false), Context{CanWrapCode: true}, Allow, 3},
		{"3 rm inside sandbox is local", shell("rm -rf build", false), Context{CanWrapCode: true}, Allow, 3},
		{"4 rule off", shell("aws s3 ls", true), Context{Rule: model.RuleOff, CanWrapCode: true}, Deny, 4},
		{"5 read-only blocks create", shell("aws s3api create-bucket --bucket x", true), Context{Rule: model.RuleReadOnly, CanWrapCode: true}, Deny, 5},
		{"5 read-only allows read", shell("aws s3 ls", true), Context{Rule: model.RuleReadOnly, CanWrapCode: true}, Allow, 13},
		{"6 delete", shell("aws s3 rb s3://old --force", true), Context{CanWrapCode: true}, Ask, 6},
		{"6 terminate", shell("aws ec2 terminate-instances --instance-ids i-1", true), Context{CanWrapCode: true}, Ask, 6},
		{"6 curl DELETE", shell("curl -X DELETE https://api.stripe.com/v1/customers/cus_1", true), Context{CanWrapCode: true}, Ask, 6},
		{"7 refund", shell("stripe refunds create --charge ch_1", true), Context{CanWrapCode: true}, Ask, 7},
		{"7 send", shell("curl -X POST https://api.resend.com/emails -d @mail.json", true), Context{CanWrapCode: true}, Ask, 7},
		{"8 live create", shell("stripe prices create --live --unit-amount 800 --currency usd", true), Context{CanWrapCode: true}, Ask, 8},
		{"9 project code with network", shell("npm install", true), Context{CanWrapCode: true}, Ask, 9},
		{"9 cdk deploy", shell("cdk deploy", true), Context{CanWrapCode: true}, Ask, 9},
		{"9 codex can't wrap code", shell("npm install", true), Context{CanWrapCode: false}, Deny, 9},
		{"10 identity from variable", shell("aws s3api create-bucket --bucket x --profile $P", true), Context{CanWrapCode: true}, Ask, 10},
		{"11 every change", shell("aws s3api put-bucket-tagging --bucket x --tagging x", true), Context{Rule: model.RuleEvery, CanWrapCode: true}, Ask, 11},
		{"12 generic create asks", shell("aws s3api create-bucket --bucket x", true), Context{CanWrapCode: true}, Ask, 12},
		{"12 unknown aws verb", shell("aws ec2 batch-something --x y", true), Context{CanWrapCode: true}, Ask, 12},
		{"12 GET to unknown host", shell("curl https://example.com", true), Context{CanWrapCode: true}, Ask, 12},
		{"13 reviewed read", shell("aws sts get-caller-identity", true), Context{CanWrapCode: true}, Allow, 13},
		{"13 GET known API", shell("curl https://api.stripe.com/v1/customers", true), Context{CanWrapCode: true}, Allow, 13},
		{"mcp read with both signals", recognize.Recognize(recognize.Call{Via: "mcp", Server: "fake", Tool: "list_items", Hints: &recognize.Hints{ReadOnly: &ro}}, env, nil), Context{CanWrapCode: true}, Allow, 13},
		{"mcp read verb without hint asks", recognize.Recognize(recognize.Call{Via: "mcp", Server: "fake", Tool: "list_items"}, env, nil), Context{CanWrapCode: true}, Ask, 10},
		{"mcp destructive hint", recognize.Recognize(recognize.Call{Via: "mcp", Server: "fake", Tool: "tidy_up", Hints: &recognize.Hints{Destructive: &ro}}, env, nil), Context{CanWrapCode: true}, Ask, 6},
		{"mcp send", recognize.Recognize(recognize.Call{Via: "mcp", Server: "resend", Tool: "send_broadcast", Args: map[string]any{"id": "bc_1"}}, env, nil), Context{CanWrapCode: true}, Ask, 7},
		{"file write outside project", recognize.Recognize(recognize.Call{Via: "file", Tool: "Write", Paths: []string{"/etc/hosts"}, Cwd: "/work"}, env, nil), Context{CanWrapCode: true}, Ask, 3},
		{"file write protected", recognize.Recognize(recognize.Call{Via: "file", Tool: "Edit", Paths: []string{"/Users/test/.claude/settings.json"}, Cwd: "/work"}, env, nil), Context{CanWrapCode: true}, Deny, 1},
		{"unknown tool asks", recognize.Recognize(recognize.Call{Via: "tool", Tool: "RemoteTrigger"}, env, nil), Context{CanWrapCode: true}, Ask, 12},
	}
	for _, c := range cases {
		got := Decide(c.draft, c.ctx)
		if got.Verdict != c.verdict || got.Row != c.row {
			t.Errorf("%s: got %s row %d (%s), want %s row %d\n  draft: action=%s reviewed=%v net=%q tags=%v generic=%v id=%+v prot=%v",
				c.name, got.Verdict, got.Row, got.Reason, c.verdict, c.row,
				c.draft.Action, c.draft.ReviewedRead, c.draft.Network, c.draft.Tags, c.draft.Generic, c.draft.Identity, c.draft.Protected)
		}
	}
}

func TestDefaultRuleWhenEmpty(t *testing.T) {
	d := shell("aws s3 rb s3://old", true)
	if got := Decide(d, Context{CanWrapCode: true}); got.Verdict != Ask {
		t.Fatalf("delete with default rule: %+v", got)
	}
}

func TestDeleteLossText(t *testing.T) {
	got := Decide(shell("aws s3 rb s3://shipyard-old --force", true), Context{CanWrapCode: true})
	if got.LossTitle != "What will be lost" || len(got.Loss) == 0 {
		t.Fatalf("no loss text: %+v", got)
	}
}
