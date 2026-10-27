package policy

import (
	"testing"

	"cloudbuddy/adapters"
	"cloudbuddy/internal/recognize"
)

// Golden tests: a real command or tool call in, the decision out, with the
// built-in adapters loaded (Phase 4).
func TestAdapterGolden(t *testing.T) {
	ads, err := recognize.LoadAdapters("", adapters.FS())
	if err != nil {
		t.Fatal(err)
	}
	genv := env
	genv.Pinned = map[string]string{}
	sh := func(cmd string) recognize.Draft {
		return recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: cmd, Cwd: "/work"}, genv, ads)
	}
	mcp := func(server, tool string, args map[string]any) recognize.Draft {
		return recognize.Recognize(recognize.Call{Agent: "claude", Via: "mcp", Server: server, Tool: tool, Args: args}, genv, ads)
	}
	cases := []struct {
		name    string
		d       recognize.Draft
		verdict Verdict
		row     int
	}{
		// AWS
		{"aws create bucket runs", sh("aws s3api create-bucket --bucket shipyard-web-prod --region us-east-1"), Allow, 13},
		{"aws run instances runs", sh("aws ec2 run-instances --image-id ami-0abc --count 1"), Allow, 13},
		{"aws iam change runs", sh("aws iam attach-role-policy --role-name r --policy-arn arn:aws:iam::aws:policy/ReadOnlyAccess"), Allow, 13},
		{"aws identity read", sh("aws sts get-caller-identity"), Allow, 13},
		{"aws describe read", sh("aws ec2 describe-instances --filters Name=tag:env,Values=test"), Allow, 13},
		{"aws terminate asks", sh("aws ec2 terminate-instances --instance-ids i-0a7f3c91e2b4d5f68"), Ask, 6},
		{"aws rb asks", sh("aws s3 rb s3://shipyard-web-staging-old --force"), Ask, 6},
		{"aws sync --delete asks", sh("aws s3 sync ./dist s3://b --delete"), Ask, 6},
		{"aws kms deletion asks", sh("aws kms schedule-key-deletion --key-id k"), Ask, 6},
		{"aws reserved purchase asks", sh("aws ec2 purchase-reserved-instances-offering --reserved-instances-offering-id x --instance-count 1"), Ask, 7},
		{"aws sns publish asks", sh("aws sns publish --topic-arn arn:aws:sns:us-east-1:1:t --message hi"), Ask, 7},
		{"aws ses send asks", sh("aws sesv2 send-email --from-email-address a@b.c"), Ask, 7},
		{"aws unknown verb asks", sh("aws ec2 batch-thing --x y"), Ask, 12},
		{"aws bucket named delete-me asks", sh("aws s3api create-bucket --bucket delete-me"), Ask, 6},
		// Stripe
		{"stripe list runs", sh("stripe customers list --limit 3"), Allow, 13},
		{"stripe test price runs", sh("stripe prices create --unit-amount 800 --currency usd"), Allow, 13},
		{"stripe live price asks", sh("stripe prices create --live --unit-amount 800 --currency usd"), Ask, 7},
		{"stripe refund asks", sh("stripe refunds create --charge ch_3Nx9EXAMPLE"), Ask, 7},
		{"stripe delete asks", sh("stripe customers delete cus_EXAMPLE123"), Ask, 6},
		{"stripe raw refund asks", sh("stripe post /v1/refunds -d charge=ch_1"), Ask, 7},
		{"stripe http refund asks", sh("curl -X POST https://api.stripe.com/v1/refunds -d charge=ch_1"), Ask, 7},
		{"stripe mcp list runs", mcp("stripe", "list_customers", nil), Allow, 13},
		{"stripe mcp refund asks", mcp("stripe", "create_refund", map[string]any{"payment_intent": "pi_1"}), Ask, 7},
		// Resend
		{"resend send asks", sh("curl -X POST https://api.resend.com/emails -H 'Content-Type: application/json' -d @mail.json"), Ask, 7},
		{"resend read runs", sh("curl https://api.resend.com/domains"), Allow, 13},
		{"resend mcp send asks", mcp("resend", "send-email", map[string]any{"to": "a@b.c"}), Ask, 7},
		{"resend mcp broadcast send asks", mcp("resend", "send-broadcast", map[string]any{"id": "bc_1"}), Ask, 7},
		// Cloudflare
		{"wrangler whoami runs", sh("wrangler whoami"), Allow, 13},
		{"wrangler kv list runs", sh("wrangler kv namespace list"), Allow, 13},
		{"wrangler deploy is project code", sh("wrangler deploy"), Ask, 9},
		{"wrangler kv delete asks", sh("wrangler kv key delete mykey --namespace-id abc"), Ask, 6},
		{"wrangler d1 drop asks", sh(`wrangler d1 execute prod-db --command "DROP TABLE users"`), Ask, 6},
		{"cloudflare dns delete asks", sh("curl -X DELETE https://api.cloudflare.com/client/v4/zones/z1/dns_records/r1"), Ask, 6},
		{"cloudflare read runs", sh("curl https://api.cloudflare.com/client/v4/zones"), Allow, 13},
	}
	for _, c := range cases {
		got := Decide(c.d, Context{CanWrapCode: true})
		if got.Verdict != c.verdict || got.Row != c.row {
			t.Errorf("%s: got %s row %d (%s); action=%s tags=%v by=%v reviewed=%v net=%q",
				c.name, got.Verdict, got.Row, got.Reason, c.d.Action, c.d.Tags, c.d.RecognizedBy, c.d.ReviewedRead, c.d.Network)
		}
	}
}

func TestAdaptersNeverLowerCaution(t *testing.T) {
	ads, err := recognize.LoadAdapters("", adapters.FS())
	if err != nil {
		t.Fatal(err)
	}
	// a reviewed-read adapter rule plus a destructive word elsewhere still asks
	d := recognize.Recognize(recognize.Call{Via: "shell", Command: "aws s3 ls s3://bucket --delete", Cwd: "/work"}, env, ads)
	if Decide(d, Context{CanWrapCode: true}).Verdict == Allow {
		t.Fatalf("adapter read hid a delete flag: %+v", d)
	}
	d = recognize.Recognize(recognize.Call{Via: "shell", Command: "aws ec2 describe-instances; aws ec2 terminate-instances --instance-ids i-1", Cwd: "/work"}, env, ads)
	if Decide(d, Context{CanWrapCode: true}).Verdict != Ask {
		t.Fatalf("compound read+terminate allowed")
	}
}
