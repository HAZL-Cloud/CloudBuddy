package mask

import (
	"strings"
	"testing"
)

// Fake secrets in the shapes real ones take. None of these are real.
var corpus = map[string]string{
	"aws access key":       "AKIAIOSFODNN7EXAMPLE",
	"aws secret in config": "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	"sts json":             `{"SecretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}`,
	"stripe live key":      "sk_live_51Hx9EXAMPLEaaaaaaaaaaaa",
	"stripe test key":      "sk_test_51Hx9EXAMPLEbbbbbbbbbbbb",
	"stripe webhook":       "whsec_EXAMPLEc3VwZXJzZWNyZXQ7Qa",
	"resend key":           "re_EXAMPLEab_cdEXAMPLEefgh",
	"github token":         "ghp_EXAMPLE0123456789abcdefghijklmnopqr",
	"jwt":                  "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
	"auth header":          "Authorization: Bearer abcdefghijklmnop1234567890",
	"url password":         "postgres://app:hunter2secret@db.example.com:5432/app",
	"env secret":           "STRIPE_WEBHOOK_SECRET=supersecretvalue123",
	"json token":           `{"access_token": "tok_1234567890abcdef"}`,
	"private key":          "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----",
}

func secretPart(name, s string) string {
	switch name {
	case "aws secret in config", "sts json":
		return "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	case "auth header":
		return "abcdefghijklmnop1234567890"
	case "url password":
		return "hunter2secret"
	case "env secret":
		return "supersecretvalue123"
	case "json token":
		return "tok_1234567890abcdef"
	case "private key":
		return "MIIEowIBAAKCAQEA"
	}
	return s
}

func TestMasksEveryCorpusSecret(t *testing.T) {
	for name, s := range corpus {
		in := "before " + s + " after"
		out, changed := String(in)
		if !changed {
			t.Errorf("%s: nothing masked in %q", name, in)
		}
		if strings.Contains(out, secretPart(name, s)) {
			t.Errorf("%s: secret still visible: %q", name, out)
		}
		if !strings.HasPrefix(out, "before ") || !strings.HasSuffix(out, " after") {
			t.Errorf("%s: surrounding text damaged: %q", name, out)
		}
	}
}

func TestKeepsOrdinaryText(t *testing.T) {
	for _, s := range []string{
		"aws s3 ls s3://shipyard-web-prod",
		"arn:aws:s3:::shipyard-web-prod",
		`{"Location": "/shipyard-web-prod"}`,
		"price_1QxR8sKj2LmN7pQ",
		"Deployed revision newsletter-api-00012-kq4",
	} {
		out, changed := String(s)
		if changed || out != s {
			t.Errorf("ordinary text changed: %q -> %q", s, out)
		}
	}
}

func TestMaskedValueKeepsTail(t *testing.T) {
	out, _ := String("whsec_EXAMPLEc3VwZXJzZWNyZXQ7Qa")
	if out != "whsec_••••7Qa" {
		t.Errorf("got %q", out)
	}
}

func TestMaskIsIdempotent(t *testing.T) {
	for name, s := range corpus {
		once, _ := String(s)
		twice, changed := String(once)
		if twice != once || changed {
			t.Errorf("%s: second pass changed %q -> %q", name, once, twice)
		}
	}
}

func TestCap(t *testing.T) {
	long := strings.Repeat("é", MaxEventBytes) // 2 bytes each
	out, cut := Cap(long, MaxEventBytes)
	if !cut || len(out) > MaxEventBytes+100 {
		t.Fatalf("not capped: %d bytes", len(out))
	}
	if !strings.HasSuffix(out, "too long to store)") {
		t.Errorf("no cut marker")
	}
	if _, cut := Cap("short", MaxEventBytes); cut {
		t.Errorf("short text marked as cut")
	}
}
