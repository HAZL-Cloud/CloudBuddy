package policy

import (
	"os"
	"path/filepath"
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
		{"aws create bucket runs", sh("aws s3api create-bucket --bucket shipyard-web-prod --region us-east-1"), Allow, 9},
		{"aws s3 mb runs", sh("aws s3 mb s3://cb-live-test"), Allow, 9},
		{"aws generated change runs (not reviewed)", sh("aws ecs update-service --cluster c --service s --force-new-deployment"), Allow, 9},
		{"aws run instances runs (spend, not reviewed)", sh("aws ec2 run-instances --image-id ami-0abc --count 1"), Allow, 9},
		{"aws iam change runs (not reviewed)", sh("aws iam attach-role-policy --role-name r --policy-arn arn:aws:iam::aws:policy/ReadOnlyAccess"), Allow, 9},
		{"aws identity read", sh("aws sts get-caller-identity"), Allow, 9},
		{"aws describe read", sh("aws ec2 describe-instances --filters Name=tag:env,Values=test"), Allow, 9},
		{"aws reviewed create runs", sh("aws cloudfront create-distribution --distribution-config file://cf.json"), Allow, 9},
		{"aws sync runs (overwrites)", sh("aws s3 sync ./dist s3://b"), Allow, 9},
		{"aws s3 mv asks (deletes source)", sh("aws s3 mv s3://a/r.csv s3://b/r.csv"), Ask, 5},
		{"aws put-item runs (replaces)", sh(`aws dynamodb put-item --table-name Customers --item '{"id":{"S":"1"}}'`), Allow, 9},
		{"aws execute-statement asks", sh("aws redshift-data execute-statement --workgroup-name wg --database db --sql 'DROP TABLE customers'"), Ask, 5},
		{"aws athena query runs", sh("aws athena start-query-execution --query-string 'SELECT 1'"), Allow, 9},
		{"aws lambda invoke runs", sh("aws lambda invoke --function-name nightly-cleanup out.json"), Allow, 9},
		{"aws session token runs", sh("aws sts get-session-token"), Allow, 9},
		{"compound reviewed read + reviewed create runs", sh("aws sts get-caller-identity && aws s3api create-bucket --bucket b"), Allow, 9},
		{"aws terminate asks", sh("aws ec2 terminate-instances --instance-ids i-0a7f3c91e2b4d5f68"), Ask, 5},
		{"aws rb asks", sh("aws s3 rb s3://shipyard-web-staging-old --force"), Ask, 5},
		{"aws sync --delete asks", sh("aws s3 sync ./dist s3://b --delete"), Ask, 5},
		{"aws kms deletion asks", sh("aws kms schedule-key-deletion --key-id k"), Ask, 5},
		{"aws reserved purchase asks", sh("aws ec2 purchase-reserved-instances-offering --reserved-instances-offering-id x --instance-count 1"), Ask, 6},
		{"aws sns publish asks", sh("aws sns publish --topic-arn arn:aws:sns:us-east-1:1:t --message hi"), Ask, 6},
		{"aws ses send asks", sh("aws sesv2 send-email --from-email-address a@b.c"), Ask, 6},
		{"aws unknown verb runs", sh("aws ec2 batch-thing --x y"), Allow, 9},
		{"aws bucket named delete-me asks", sh("aws s3api create-bucket --bucket delete-me"), Ask, 5},
		// Stripe
		{"stripe list runs", sh("stripe customers list --limit 3"), Allow, 9},
		{"stripe test price asks (amount can't change)", sh("stripe prices create --unit-amount 800 --currency usd"), Ask, 6},
		{"stripe test customer runs", sh("stripe customers create --description Acme"), Allow, 9},
		{"stripe live price asks", sh("stripe prices create --live --unit-amount 800 --currency usd"), Ask, 6},
		{"stripe refund asks", sh("stripe refunds create --charge ch_3Nx9EXAMPLE"), Ask, 6},
		{"stripe delete asks", sh("stripe customers delete cus_EXAMPLE123"), Ask, 5},
		{"stripe raw refund asks", sh("stripe post /v1/refunds -d charge=ch_1"), Ask, 6},
		{"stripe http refund asks", sh("curl -X POST https://api.stripe.com/v1/refunds -d charge=ch_1"), Ask, 6},
		{"stripe finalize asks", sh("curl -X POST https://api.stripe.com/v1/invoices/in_1/finalize -u $STRIPE_KEY:"), Ask, 6},
		{"stripe unknown POST runs", sh("curl -X POST https://api.stripe.com/v1/billing_portal/configurations -u $STRIPE_KEY:"), Allow, 9},
		{"stripe mcp list runs", mcp("stripe", "list_customers", nil), Allow, 9},
		{"stripe mcp refund asks", mcp("stripe", "create_refund", map[string]any{"payment_intent": "pi_1"}), Ask, 6},
		{"look-alike server name gets no adapter", mcp("evil-stripe-proxy", "list_customers", nil), Allow, 9},
		{"stripe-evil gets no adapter", mcp("stripe-evil", "list_customers", nil), Allow, 9},
		{"exact stripe server name matches", mcp("stripe", "list_customers", nil), Allow, 9},
		// Resend
		{"resend send asks", sh("curl -X POST https://api.resend.com/emails -H 'Content-Type: application/json' -d @mail.json"), Ask, 6},
		{"resend read runs", sh("curl https://api.resend.com/domains"), Allow, 9},
		{"resend mcp send asks", mcp("resend", "send-email", map[string]any{"to": "a@b.c"}), Ask, 6},
		{"resend mcp broadcast send asks", mcp("resend", "send-broadcast", map[string]any{"id": "bc_1"}), Ask, 6},
		// Cloudflare
		{"wrangler whoami runs", sh("wrangler whoami"), Allow, 9},
		{"wrangler kv list runs", sh("wrangler kv namespace list"), Allow, 9},
		{"wrangler deploy is project code", sh("wrangler deploy"), Allow, 9},
		{"wrangler pages deploy is project code", sh("wrangler pages deploy dist --project-name site"), Allow, 9},
		{"wrangler kv delete asks", sh("wrangler kv key delete mykey --namespace-id abc"), Ask, 5},
		{"wrangler d1 drop asks", sh(`wrangler d1 execute prod-db --command "DROP TABLE users"`), Ask, 5},
		{"cloudflare dns delete asks", sh("curl -X DELETE https://api.cloudflare.com/client/v4/zones/z1/dns_records/r1"), Ask, 5},
		{"cloudflare read runs", sh("curl https://api.cloudflare.com/client/v4/zones"), Allow, 9},
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

// D-065: commands run as they would in a terminal, but whatever can't be
// undone still asks, however it's written. These include the tricks the
// Phase 2 and release reviews found (wrappers, nested shells, curl option
// tricks, signed raw requests, sed's e command), and deletes CloudBuddy can
// only see by name.
func TestIrreversibleAsksHoweverWritten(t *testing.T) {
	ads, err := recognize.LoadAdapters("", adapters.FS())
	if err != nil {
		t.Fatal(err)
	}
	genv := env
	genv.Pinned = map[string]string{"curl": "/usr/bin/curl", "aws": "/opt/homebrew/bin/aws"}
	for _, c := range []string{
		// deletes
		"aws ec2 describe-instances; aws ec2 terminate-instances --instance-ids i-1",
		"aws s3 ls s3://bucket --delete",
		"nohup aws s3 rb s3://old --force &",
		"bash -c 'aws s3 rb s3://x'",
		"./env aws s3 rb s3://x",
		"function aws() { command aws s3 rb s3://prod --force; }; aws ec2 describe-instances",
		"source /dev/stdin <<'EOF'\naws s3 rb s3://x\nEOF",
		"printf 'x\\n' | sed 'e aws s3 rm s3://source-bucket/report.csv'; aws sts get-caller-identity",
		"curl https://api.stripe.com/v1/a --next -X DELETE https://api.stripe.com/v1/customers/c",
		`curl -XDELETE -H "Authorization: Bearer $STRIPE_API_KEY" https://api.stripe.com/v1/customers/cus_123`,
		`curl -sXDELETE https://api.stripe.com/v1/customers/cus_123`,
		`curl -X GET -X DELETE https://api.stripe.com/v1/customers/cus_123`,
		`curl --aws-sigv4 aws:amz:us-east-1:ec2 -u "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" 'https://ec2.us-east-1.amazonaws.com/?Action=TerminateInstances&InstanceId.1=i-0123456789abcdef0&Version=2016-11-15'`,
		"curl -G -X GET --aws-sigv4 aws:amz:us-east-1:ec2 -u \"$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY\" --data-urlencode Action=TerminateInstances --data-urlencode InstanceId.1=i-0123456789abcdef0 https://ec2.us-east-1.amazonaws.com/",
		"aws s3 sync ./site s3://archive-bucket/ --delete",
		"terraform destroy -auto-approve",
		"wrangler d1 execute prod-db --command \"DROP TABLE users\"",
		// deletes only the name shows
		"make db-drop ENV=dev",
		"./scripts/delete-prod.sh",
		"pnpm run cache:purge",
		"npm run teardown",
		"python3 scripts/wipe_bucket.py --env dev",
		// money, people, live
		"stripe refunds create --charge ch_1",
		"curl -X POST https://api.stripe.com/v1/refunds -d charge=ch_1",
		`curl "https://slack.com/api/chat.postMessage?channel=C1&text=hi"`,
		"aws sesv2 send-email --from-email-address a@b.c",
		"stripe prices create --live --unit-amount 800 --currency usd",
	} {
		for _, wants := range []bool{false, true} {
			d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: c, WantsNetwork: wants, Cwd: "/work"}, genv, ads)
			if got := Decide(d, Context{CanWrapCode: true}); got.Verdict != Ask {
				t.Errorf("didn't ask: %q wants=%v -> %s row %d (action=%s tags=%v net=%q opaque=%v)", c, wants, got.Verdict, got.Row, d.Action, d.Tags, d.Network, d.Opaque)
			}
		}
	}
}

// Self-protection (D-027) holds whatever else changed: the agent can't reach
// CloudBuddy's data, its port, or the agents' settings and logins.
func TestSelfProtectionHolds(t *testing.T) {
	ads, err := recognize.LoadAdapters("", adapters.FS())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{
		"cat /Users/test/.cloudbuddy/gate.json",
		"curl -X POST http://127.0.0.1:7420/api/approvals/ap_1 -d '{\"decision\":\"approve\"}'",
		"curl http://localhost:7420/api/session",
		"echo '{}' > /Users/test/.claude/settings.json",
		"cat ~/.claude/.credentials.json",
		"cloudbuddy exec --release abc",
	} {
		for _, wants := range []bool{false, true} {
			d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: c, WantsNetwork: wants, Cwd: "/work"}, env, ads)
			if got := Decide(d, Context{CanWrapCode: true}); got.Verdict != Deny {
				t.Errorf("not refused: %q wants=%v -> %s row %d", c, wants, got.Verdict, got.Row)
			}
		}
	}
}

// Ordinary work runs without asking, whether or not the agent asked for
// network, as in a terminal (D-064, D-065). So do the things the old rules
// asked about only because CloudBuddy couldn't see inside: project code with
// network, files from outside the project, another account's config. The
// card shows them; this is the trade-off the user accepted (D-065).
func TestTerminalWorkRuns(t *testing.T) {
	ads, err := recognize.LoadAdapters("", adapters.FS())
	if err != nil {
		t.Fatal(err)
	}
	genv := env
	genv.Pinned = map[string]string{"curl": "/usr/bin/curl", "aws": "/opt/homebrew/bin/aws"}
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "infra"), 0o755); err != nil {
		t.Fatal(err)
	}
	genv.Workdir = work
	for _, ok := range []string{"aws s3 ls", "aws ec2 describe-instances | jq .", "cd infra && aws sts get-caller-identity",
		"curl -s https://api.stripe.com/v1/customers -u $STRIPE_SECRET_KEY:",
		"aws ec2 describe-instances | head -5", "aws s3 ls; echo ---; aws sts get-caller-identity",
		"supabase migration list --linked 2>&1 | tail -12", "supabase projects list 2>&1 | tail -5",
		"wrangler deployments list --name web | grep Created | wc -l",
		"make credit-purchases ENV=dev DAYS=7", "HAZLSKY_PROVIDER_CLI_APPROVED=1 make credit-purchases ENV=dev DAYS=7",
		"cd frontend && pnpm exec wrangler deployments list --name hazlsky-d",
		"curl -s -o /dev/null -w '%{http_code}' https://www.hazl.ca/",
		"for u in https://hazl.ca/ https://dev-sky.hazl.ca/; do curl -s -o /dev/null -w \"$u %{http_code}\\n\" $u; done",
		"wrangler tail hazlsky-api --format pretty",
		"aws logs tail /aws/lambda/api --follow --since 10m",
		"npm install", "pnpm run deploy:prod", "wrangler deploy", "git push origin main",
		"python3 evil.py; curl https://api.github.com/user",
		"aws s3 cp /etc/hosts s3://b/",
		"AWS_CONFIG_FILE=/tmp/other aws ec2 start-instances --instance-ids i-1",
		"aws s3api create-bucket --bucket b", "stripe customers create --description Acme",
		// delete words that aren't a hidden delete
		"npm run build && aws s3 sync dist s3://b && rm -rf dist",
		"aws s3 cp $(ls dist/app.js) s3://b/ && rm -rf tmp",
		`git commit -m "remove aws keys from the docs"`,
		"grep -rn 'aws s3 rm' scripts/",
		"make deploy-rollback ENV=dev REVISION=tree-1",
	} {
		for _, wants := range []bool{false, true} {
			d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: ok, WantsNetwork: wants, Cwd: work}, genv, ads)
			if got := Decide(d, Context{CanWrapCode: true}); got.Verdict != Allow {
				t.Errorf("asks: %q wants=%v -> row %d %s", ok, wants, got.Row, got.Reason)
			}
		}
	}
}

// A project's other folders count as project folders (D-054): files there
// aren't "outside the project". Since D-065 writes run either way; the
// protected folders stay protected.
func TestProjectFoldersCountAsProject(t *testing.T) {
	ads, err := recognize.LoadAdapters("", adapters.FS())
	if err != nil {
		t.Fatal(err)
	}
	project := env
	project.Folders = []string{"/code/infra"}
	project.Pinned = map[string]string{"aws": "/opt/homebrew/bin/aws"}
	single := project
	single.Folders = nil

	write := func(e recognize.Env, p string) Decision {
		d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "file", Tool: "Write", Paths: []string{p}, Cwd: "/work"}, e, ads)
		return Decide(d, Context{CanWrapCode: true})
	}
	outside := func(e recognize.Env, p string) bool {
		return recognize.Recognize(recognize.Call{Agent: "claude", Via: "file", Tool: "Write", Paths: []string{p}, Cwd: "/work"}, e, ads).OutsideWorkdirWrite
	}
	if outside(project, "/code/infra/main.tf") {
		t.Errorf("write in a project folder counted as outside")
	}
	if !outside(project, "/code/infra-old/main.tf") || !outside(project, "/elsewhere/main.tf") || !outside(single, "/code/infra/main.tf") {
		t.Errorf("write outside the project not recognized as outside")
	}
	// D-065: writes run either way, as in a terminal
	for _, p := range []string{"/code/infra/main.tf", "/elsewhere/main.tf"} {
		if got := write(project, p); got.Verdict != Allow {
			t.Errorf("write %s: row %d %s", p, got.Row, got.Reason)
		}
	}

	upload := "aws s3 cp /code/infra/dist/site.zip s3://bucket/site.zip"
	call := recognize.Call{Agent: "claude", Via: "shell", Command: upload, WantsNetwork: true, Cwd: "/work"}
	if d := recognize.Recognize(call, project, ads); len(d.ExternalFiles) != 0 {
		t.Errorf("file in a project folder counted as outside: %v", d.ExternalFiles)
	}
	if d := recognize.Recognize(call, single, ads); len(d.ExternalFiles) == 0 {
		t.Errorf("file in a folder of no project not counted as outside")
	}
	// the protected folders stay protected inside a project folder
	project.Folders = []string{"/Users/test"}
	d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "file", Tool: "Write", Paths: []string{"/Users/test/.cloudbuddy/x"}, Cwd: "/work"}, project, ads)
	if got := Decide(d, Context{CanWrapCode: true}); got.Verdict != Deny {
		t.Errorf("CloudBuddy's own folder writable through a project folder: row %d", got.Row)
	}
}
