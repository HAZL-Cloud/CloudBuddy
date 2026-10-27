package run

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
)

// Values one step produces and a later step uses become blue arrows on the
// canvas (section 4.4). Only keyed hashes are stored, never raw values; the
// key is per conversation and lives in CloudBuddy's protected folder.

var idPatterns = []struct {
	re   *regexp.Regexp
	what string
}{
	{regexp.MustCompile(`arn:aws[a-z-]*:[a-z0-9-]+:[a-z0-9-]*:\d{0,12}:[A-Za-z0-9/_+=.@:-]+`), "ARN"},
	{regexp.MustCompile(`\b(price|prod|cus|sub|we|acct|ch|re|pi|in|evt|pm|seti|txn|po|tr|plink|cs)_[A-Za-z0-9]{8,}\b`), "Stripe ID"},
	{regexp.MustCompile(`\bwhsec_[A-Za-z0-9+/=]{10,}`), "webhook secret"},
	{regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{10,}`), "API key"},
	{regexp.MustCompile(`\b(i|vol|snap|sg|subnet|vpc|ami|eni|igw|nat|rtb|lt|eipalloc)-[0-9a-f]{8,17}\b`), "resource ID"},
	{regexp.MustCompile(`\bE[A-Z0-9]{12,13}\b`), "distribution ID"},
	{regexp.MustCompile(`\b[a-z0-9][a-z0-9-]{1,62}\.(?:cloudfront\.net|run\.app|workers\.dev|vercel\.app|pages\.dev|netlify\.app|fly\.dev|herokuapp\.com)\b`), "address"},
	{regexp.MustCompile(`s3://[a-z0-9][a-z0-9.-]{1,62}`), "bucket"},
	{regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), "ID"},
	{regexp.MustCompile(`\b(?:bc|aud|dom|key|tmpl)_[A-Za-z0-9]{6,}\b`), "ID"},
}

func extractIDs(text string) map[string]string {
	out := map[string]string{}
	if len(text) > 512*1024 {
		text = text[:512*1024]
	}
	for _, p := range idPatterns {
		for _, m := range p.re.FindAllString(text, 200) {
			if _, ok := out[m]; !ok {
				out[m] = p.what
			}
		}
	}
	return out
}

func (e *Engine) linkKey(convID string) []byte {
	dir := filepath.Join(e.cfg.DataDir, "keys")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, convID+".key")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return b
	}
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	_ = os.WriteFile(path, k, 0o600)
	return k
}

func linkHash(key []byte, v string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(v))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

func (e *Engine) recordLinks(ctx context.Context, r *Run, stepID, output string) {
	ids := extractIDs(output)
	if len(ids) == 0 {
		return
	}
	key := e.linkKey(r.ConvID)
	toks := map[string]string{}
	for v, what := range ids {
		toks[linkHash(key, v)] = what
	}
	_ = e.st.AddLinkTokens(ctx, r.ConvID, stepID, toks)
}

func (e *Engine) linksFor(ctx context.Context, r *Run, toolKey string, c recognize.Call) []model.Use {
	text := c.Command
	if len(c.Args) > 0 {
		b, _ := json.Marshal(c.Args)
		text += " " + string(b)
	}
	ids := extractIDs(text)
	if len(ids) == 0 {
		return nil
	}
	key := e.linkKey(r.ConvID)
	var hashes []string
	for v := range ids {
		hashes = append(hashes, linkHash(key, v))
	}
	existing, _, _ := e.st.StepByKey(ctx, r.ConvID, toolKey)
	uses, err := e.st.FindLinks(ctx, r.ConvID, existing.ID, hashes)
	if err != nil {
		return nil
	}
	return uses
}
