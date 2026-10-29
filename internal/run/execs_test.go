package run

import (
	"strings"
	"testing"

	"cloudbuddy/internal/model"
	"cloudbuddy/internal/netlog"
	"cloudbuddy/internal/recognize"
)

// A script's CLI call is quoted so the recognizer reads back the same words.
func TestShellJoinRoundTrips(t *testing.T) {
	env := recognize.Env{Home: "/Users/test", Workdir: "/work"}
	for _, c := range []struct {
		args []string
		want model.Action
	}{
		{[]string{"s3", "rb", "s3://old-bucket", "--force"}, model.ActionDelete},
		{[]string{"sts", "get-caller-identity"}, model.ActionRead},
		{[]string{"dynamodb", "get-item", "--table-name", "users", "--key", `{"id":{"S":"a b"}}`}, model.ActionRead},
	} {
		cmd := shellJoin("aws", c.args)
		d := recognize.Recognize(recognize.Call{Via: "shell", Command: cmd, WantsNetwork: true, Cwd: "/work"}, env, nil)
		if d.Action != c.want {
			t.Errorf("%s: action %s, want %s", cmd, d.Action, c.want)
		}
	}
	if got := shellJoin("aws", []string{"it's", "a b", "plain"}); got != `aws 'it'\''s' 'a b' plain` {
		t.Errorf("quoting: %s", got)
	}
}

// A command that reached the internet becomes a cloud card named after the
// provider it talked to; other sites make it a Web card.
func TestApplyHits(t *testing.T) {
	s := model.Step{Kind: "local", Provider: "Local", Facts: [][2]string{{"Talked to", "old"}}}
	applyHits(&s, []netlog.Hit{{Host: "api.supabase.com", Count: 7}, {Host: "eu.i.posthog.com", Count: 2}})
	if s.Kind != "cloud" || s.Provider != "Supabase" || len(s.Facts) != 1 || s.Facts[0][1] != "api.supabase.com ×7, eu.i.posthog.com ×2" {
		t.Errorf("supabase script: %+v", s)
	}
	s = model.Step{Kind: "local", Provider: "Local"}
	applyHits(&s, []netlog.Hit{{Host: "hazl.ca", Count: 1}})
	if s.Kind != "cloud" || s.Provider != "Web" {
		t.Errorf("web script: %+v", s)
	}
	s = model.Step{Kind: "cloud", Provider: "AWS"}
	applyHits(&s, []netlog.Hit{{Host: "registry.npmjs.org", Count: 3}})
	if s.Provider != "AWS" {
		t.Errorf("a known provider was replaced: %+v", s)
	}
}

func TestStopNote(t *testing.T) {
	if got := withStopNote("PING 1.1.1.1", "you"); !strings.HasSuffix(got, "[You stopped this command.]") {
		t.Errorf("you: %q", got)
	}
	if got := withStopNote(withStopNote("x", "run"), "run"); strings.Count(got, "stopped") != 1 {
		t.Errorf("note added twice: %q", got)
	}
	if got := withStopNote("x", ""); got != "x" {
		t.Errorf("no stop: %q", got)
	}
}
