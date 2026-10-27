// Package mask hides secrets in agent and tool text before it is stored,
// streamed or logged (plan section 4.6), and caps its size. Masking is best
// effort: formats it doesn't know can get through.
package mask

import (
	"regexp"
	"strings"
)

// MaxEventBytes caps every stored payload (section 4.6).
const MaxEventBytes = 256 * 1024

type pattern struct {
	re *regexp.Regexp
	// group is the submatch holding the secret; 0 masks the whole match.
	group int
}

var patterns = []pattern{
	{re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`)},
	{re: regexp.MustCompile(`\b(?:AKIA|ASIA|AIDA|AROA|AIPA|ANPA|ANVA|AGPA)[A-Z0-9]{16}\b`)},
	{re: regexp.MustCompile(`(?i)aws_secret_access_key\s*[=:]\s*["']?([A-Za-z0-9/+=]{40})`), group: 1},
	{re: regexp.MustCompile(`"SecretAccessKey"\s*:\s*"([A-Za-z0-9/+=]{40})"`), group: 1},
	{re: regexp.MustCompile(`"SessionToken"\s*:\s*"([A-Za-z0-9/+=]{40,})"`), group: 1},
	{re: regexp.MustCompile(`\b(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9]{10,}`)},
	{re: regexp.MustCompile(`\bwhsec_[A-Za-z0-9+/=]{10,}`)},
	{re: regexp.MustCompile(`\bre_[A-Za-z0-9]{8,}_[A-Za-z0-9]{8,}`)}, // Resend API keys
	{re: regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{30,}\b`)},
	{re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}\b`)},
	{re: regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`)},
	{re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)}, // JWT
	{re: regexp.MustCompile(`(?i)\b(authorization|proxy-authorization)\s*:\s*(?:bearer|basic|token)?\s*([^\s"'\\]{8,})`), group: 2},
	{re: regexp.MustCompile(`(?i)\b(?:bearer)\s+([A-Za-z0-9._~+/=-]{16,})`), group: 1},
	{re: regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^:/\s@]+:([^@\s/]{3,})@`), group: 1}, // passwords in URLs
	// KEY=value and "key": "value" where the name looks secret.
	{re: regexp.MustCompile(`(?i)\b[A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|ACCESS_?KEY|PRIVATE_?KEY|CLIENT_?SECRET)[A-Z0-9_]*\s*=\s*["']?([^\s"'&;|]{6,})`), group: 1},
	{re: regexp.MustCompile(`(?i)"[a-z0-9_]*(?:token|secret|password|api_?key|access_?key|private_?key)[a-z0-9_]*"\s*:\s*"([^"]{6,})"`), group: 1},
}

// keep is how many trailing characters stay visible so masked values can
// still be told apart.
const keep = 3

func hide(s string) string {
	if len(s) <= keep+4 {
		return strings.Repeat("•", 4)
	}
	// keep a short readable prefix such as "whsec_" or "sk_live_"
	prefix := ""
	if i := strings.LastIndex(s[:min(len(s), 9)], "_"); i > 0 {
		prefix = s[:i+1]
	}
	return prefix + "••••" + s[len(s)-keep:]
}

// String masks known secret formats in s. It reports whether anything was
// hidden.
func String(s string) (string, bool) {
	changed := false
	for _, p := range patterns {
		s = p.re.ReplaceAllStringFunc(s, func(m string) string {
			if p.group == 0 {
				changed = true
				return hide(m)
			}
			sub := p.re.FindStringSubmatchIndex(m)
			if sub == nil || sub[2*p.group] < 0 {
				return m
			}
			start, end := sub[2*p.group], sub[2*p.group+1]
			secret := m[start:end]
			if strings.Contains(secret, "••••") {
				return m
			}
			changed = true
			return m[:start] + hide(secret) + m[end:]
		})
	}
	return s, changed
}

// Cap shortens s to at most n bytes, cutting at a rune boundary, and reports
// whether it was cut.
func Cap(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "\n… (cut by CloudBuddy: too long to store)", true
}

// Clean masks and then caps s for storage.
func Clean(s string) (out string, masked, truncated bool) {
	out, masked = String(s)
	out, truncated = Cap(out, MaxEventBytes)
	return
}
