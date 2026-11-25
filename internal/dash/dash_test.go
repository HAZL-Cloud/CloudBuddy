package dash

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCheckSpec(t *testing.T) {
	s := Spec{Title: " Users ", Refresh: "15m",
		Sources: []Source{{Key: "users", Command: "psql -tAc 'select count(*) from users'"}},
		Tiles:   []Tile{{Type: "number", Title: "Users", Source: "users"}, {Type: "line", Title: "Users over time", Source: "users"}}}
	if err := Check(&s); err != nil {
		t.Fatal(err)
	}
	if s.Title != "Users" || s.Refresh != "15m" || s.Sources[0].Format != "auto" || s.Sources[0].TimeoutSeconds != DefTimeout {
		t.Errorf("not normalized: %+v", s)
	}
	if s.Tiles[0].Key != "users" || s.Tiles[1].Key != "users-over-time" || s.Tiles[0].Width != 1 || s.Tiles[1].Width != 2 {
		t.Errorf("tile defaults: %+v", s.Tiles)
	}

	bad := Spec{Title: "", Refresh: "10s",
		Sources: []Source{{Key: "A B", Command: ""}, {Key: "x", Command: "ls", Format: "xml"}},
		Tiles:   []Tile{{Type: "pie", Title: "P", Source: "x"}, {Type: "number", Title: "N", Source: "nope"}, {Type: "line", Title: "L", Series: []Series{{Source: "zz"}}}}}
	err := Check(&bad)
	if err == nil {
		t.Fatal("expected problems")
	}
	for _, want := range []string{"title is missing", "too often", "keys use", "no command", "format \"xml\"", "type \"pie\"", "\"nope\"", "\"zz\""} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("problems should mention %q:\n%s", want, err)
		}
	}
}

func TestParseRefresh(t *testing.T) {
	for in, want := range map[string]time.Duration{"1m": time.Minute, "90": 90 * time.Second, "2h": 2 * time.Hour, "1d": 24 * time.Hour, "1h30m": 90 * time.Minute} {
		got, err := ParseRefresh(in)
		if err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	for _, in := range []string{"", "30s", "2d", "soon"} {
		if _, err := ParseRefresh(in); err == nil {
			t.Errorf("%q should fail", in)
		}
	}
}

func TestParseAndLookup(t *testing.T) {
	v, err := Parse(`{"data":[{"id":1},{"id":2}],"has_more":false,"total":"1,204"}`, "auto")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]float64{"data.length": 2, "data[1].id": 2, "data.last.id": 2, "$.total": 1204} {
		got, err := Lookup(v, path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if n, ok := Number(got); !ok || n != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}
	if _, err := Lookup(v, "nope"); err == nil || !strings.Contains(err.Error(), "data, has_more, total") {
		t.Errorf("missing key should list the keys: %v", err)
	}
	// a key asked of a list says what is there
	rows, _ := Parse(`[{"accounts": 11}]`, "json")
	if _, err := Lookup(rows, "rows.0.accounts"); err == nil || !strings.Contains(err.Error(), `the output is a list, not an object with "rows" in it`) {
		t.Errorf("a key on a list: %v", err)
	}
	if _, err := Lookup(v, "data.id"); err == nil || !strings.Contains(err.Error(), `"data" is a list`) {
		t.Errorf("a key on a nested list: %v", err)
	}
	if n, _ := Parse(" 1,234\n", "auto"); n != 1234.0 {
		t.Errorf("number: %v", n)
	}
	rows, err = Parse("day,count\n2026-11-01,4\n2026-11-02,7\n", "csv")
	if err != nil {
		t.Fatal(err)
	}
	bars, err := Bars(rows, "", "")
	if err != nil || len(bars) != 2 || bars[1].Label != "2026-11-02" || bars[1].Value != 7 {
		t.Errorf("bars from csv: %+v %v", bars, err)
	}
	jl, err := Parse("{\"a\":1}\n{\"a\":2}\n", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := Lookup(jl, "length"); l != 2.0 {
		t.Errorf("JSON lines: %v", jl)
	}
	if n, ok := Number([]any{map[string]any{"count": 12.0}}); !ok || n != 12 {
		t.Errorf("one SQL row: %v %v", n, ok)
	}
}

type fakeHistory map[string][]Point

func (h fakeHistory) Points(s string) []Point { return h[s] }
func (h fakeHistory) Before(s string, at time.Time, d time.Duration) *Point {
	var best *Point
	for i, p := range h[s] {
		if at.Sub(p.T) >= d {
			best = &h[s][i]
		}
	}
	return best
}
func (h fakeHistory) Oldest(s string) *Point {
	if len(h[s]) == 0 {
		return nil
	}
	return &h[s][0]
}
func (h fakeHistory) Previous(s string) *Point {
	if len(h[s]) < 2 {
		return nil
	}
	return &h[s][len(h[s])-2]
}

func TestValues(t *testing.T) {
	now := time.Now()
	s := Spec{Title: "T", Refresh: "5m", Sources: []Source{
		{Key: "users", Command: "x"}, {Key: "plans", Command: "y", Format: "json"}, {Key: "site", Command: "z"},
		{Key: "slow", Command: "w"}, {Key: "never", Command: "v"},
	}, Tiles: []Tile{
		{Type: "number", Title: "Users", Source: "users"},
		{Type: "bars", Title: "Plans", Source: "plans"},
		{Type: "table", Title: "Plans table", Source: "plans", Limit: 1},
		{Type: "status", Title: "Site", Source: "site"},
		{Type: "number", Title: "Slow", Source: "slow"},
		{Type: "number", Title: "Never", Source: "never"},
		{Type: "line", Title: "Users over time", Series: []Series{{Source: "users", Label: "Users"}}},
	}}
	if err := Check(&s); err != nil {
		t.Fatal(err)
	}
	one := 1
	runs := map[string]Runs{
		"users": {Latest: &RunView{State: RunOK, Output: "1204\n", At: now}, LastOK: &RunView{State: RunOK, Output: "1204\n", At: now}},
		"plans": {Latest: &RunView{State: RunOK, Output: `[{"plan":"free","n":900},{"plan":"pro","n":304}]`, At: now}},
		"site":  {Latest: &RunView{State: RunFailed, ExitCode: &one, Note: "curl: (7) Failed to connect", At: now}},
		"slow":  {Latest: &RunView{State: RunFailed, ExitCode: &one, At: now}, LastOK: &RunView{State: RunOK, Output: "5", At: now.Add(-time.Hour)}},
		"never": {Latest: &RunView{State: RunWaiting, At: now}},
	}
	runs["plans"] = Runs{Latest: runs["plans"].Latest, LastOK: runs["plans"].Latest}
	h := fakeHistory{SeriesKey("users", ""): {{T: now.Add(-25 * time.Hour), V: 1100}, {T: now, V: 1204}}}
	vals := Values(s, runs, h, now)
	byKey := map[string]TileValue{}
	for _, v := range vals {
		byKey[v.Key] = v
	}
	if u := byKey["users"]; u.State != "ok" || *u.Number != 1204 || u.Compare == nil || u.Compare.Value != 1100 || len(u.Spark) != 2 {
		t.Errorf("users: %+v", u)
	}
	if p := byKey["plans"]; p.State != "ok" || len(p.Bars) != 2 || p.Bars[1].Label != "pro" || p.Bars[1].Value != 304 {
		t.Errorf("plans: %+v", p)
	}
	if p := byKey["plans-table"]; len(p.Rows) != 1 || p.More != 1 || len(p.Columns) != 2 {
		t.Errorf("table: %+v", p)
	}
	if st := byKey["site"]; st.Status != "fail" || !strings.Contains(st.Message, "Failed to connect") {
		t.Errorf("site: %+v", st)
	}
	if sl := byKey["slow"]; sl.State != "ok" || !sl.Stale || *sl.Number != 5 {
		t.Errorf("slow: %+v", sl)
	}
	if nv := byKey["never"]; nv.State != "waiting" {
		t.Errorf("never: %+v", nv)
	}
	if l := byKey["users-over-time"]; len(l.Series) != 1 || len(l.Series[0].Points) != 2 {
		t.Errorf("line: %+v", l)
	}
}

// A source's revision changes with what its output means, and only that.
func TestSourceRev(t *testing.T) {
	base := Source{Key: "n", Label: "N", Command: "cat n.json", Cwd: "sub", Format: "json", TimeoutSeconds: 10}
	rev := SourceRev("/p", base)
	for name, c := range map[string]struct {
		workdir string
		src     Source
	}{
		"workdir": {"/q", base},
		"command": {"/p", Source{Key: "n", Command: "cat m.json", Cwd: "sub", Format: "json"}},
		"cwd":     {"/p", Source{Key: "n", Command: "cat n.json", Cwd: "other", Format: "json"}},
		"format":  {"/p", Source{Key: "n", Command: "cat n.json", Cwd: "sub", Format: "text"}},
		// the same bytes split differently between the fields
		"split": {"/p", Source{Key: "n", Command: "cat n.jsonsub", Cwd: "", Format: "json"}},
	} {
		if SourceRev(c.workdir, c.src) == rev {
			t.Errorf("changing the %s kept the revision", name)
		}
	}
	same := base
	same.Label, same.TimeoutSeconds = "Other label", 60
	if SourceRev("/p", same) != rev {
		t.Error("the label or timeout changed the revision")
	}
}

// A number that stopped refreshing is compared with the history before its
// own time, not before now: counting from now compared the old value with
// itself ("No change in 7 days" on a number 10 days old).
func TestOldValueComparesFromItsOwnTime(t *testing.T) {
	now := time.Now()
	s := Spec{Title: "T", Refresh: "1h", Sources: []Source{{Key: "traffic", Command: "x", Format: "json"}},
		Tiles: []Tile{{Type: "number", Title: "Visitors", Source: "traffic", Path: "visitors7d", Compare: "7d"},
			{Type: "number", Title: "Avg", Source: "traffic", Path: "avg"}}}
	if err := Check(&s); err != nil {
		t.Fatal(err)
	}
	one := 1
	lastOK := now.Add(-10 * 24 * time.Hour)
	runs := map[string]Runs{"traffic": {
		Latest: &RunView{State: RunFailed, ExitCode: &one, Note: "Not logged in", At: now},
		LastOK: &RunView{State: RunOK, Output: `{"visitors7d": 1397, "avg": 200}`, At: lastOK},
	}}
	h := fakeHistory{
		SeriesKey("traffic", "visitors7d"): {{T: lastOK.Add(-7*24*time.Hour - time.Hour), V: 1271}, {T: lastOK, V: 1397}},
		SeriesKey("traffic", "avg"):        {{T: lastOK.Add(-25 * time.Hour), V: 180}, {T: lastOK, V: 200}},
	}
	vals := Values(s, runs, h, now)
	if v := vals[0]; !v.Stale || v.Compare == nil || v.Compare.Value != 1271 {
		t.Errorf("visitors: compared with %+v, want the point 7 days before the value (1271)", v.Compare)
	}
	if v := vals[1]; v.Compare == nil || v.Compare.Value != 180 {
		t.Errorf("avg: compared with %+v, want the point a day before the value (180)", v.Compare)
	}
}

// A daily chart, oldest day first, that has more days than it shows keeps
// the newest days (2026-11-24: "Sign-ins per day (30 days)" showed Oct 27 to
// Nov 15 and hid the ten newest).
func TestBarsKeepTheNewestDays(t *testing.T) {
	day := func(i int) time.Time { return time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i) }
	var oldestFirst, newestFirst, isoDays []string
	for i := 0; i < 30; i++ {
		oldestFirst = append(oldestFirst, fmt.Sprintf(`{"day":%q,"n":%d}`, day(i).Format("Jan 02"), i))
		newestFirst = append([]string{fmt.Sprintf(`{"day":%q,"n":%d}`, day(i).Format("Jan 02"), i)}, newestFirst...)
		isoDays = append(isoDays, fmt.Sprintf(`{"day":%q,"n":%d}`, day(i).Format("2006-01-02"), i))
	}
	s := Spec{Title: "T", Refresh: "15m", Sources: []Source{
		{Key: "up", Command: "x", Format: "json"}, {Key: "down", Command: "y", Format: "json"},
		{Key: "iso", Command: "z", Format: "json"}, {Key: "plans", Command: "w", Format: "json"},
	}, Tiles: []Tile{
		{Type: "bars", Title: "Up", Source: "up", LabelField: "day", ValueField: "n", Limit: 20},
		{Type: "bars", Title: "Down", Source: "down", LabelField: "day", ValueField: "n", Limit: 20},
		{Type: "bars", Title: "Iso", Source: "iso", LabelField: "day", ValueField: "n", Limit: 7},
		{Type: "bars", Title: "Plans", Source: "plans", Limit: 2},
		{Type: "bars", Title: "Whole month", Source: "up", LabelField: "day", ValueField: "n"},
	}}
	if err := Check(&s); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ok := func(out string) Runs {
		r := &RunView{State: RunOK, Output: out, At: now}
		return Runs{Latest: r, LastOK: r}
	}
	runs := map[string]Runs{
		"up":    ok("[" + strings.Join(oldestFirst, ",") + "]"),
		"down":  ok("[" + strings.Join(newestFirst, ",") + "]"),
		"iso":   ok("[" + strings.Join(isoDays, ",") + "]"),
		"plans": ok(`[{"plan":"free","n":900},{"plan":"pro","n":304},{"plan":"team","n":12}]`),
	}
	byKey := map[string]TileValue{}
	for _, v := range Values(s, runs, fakeHistory{}, now) {
		byKey[v.Key] = v
	}
	check := func(key string, n, more int, oldestFirst bool, first, last string) {
		t.Helper()
		v := byKey[key]
		if len(v.Bars) != n || v.More != more || v.OldestFirst != oldestFirst {
			t.Fatalf("%s: %d bars, more %d, oldest first %v", key, len(v.Bars), v.More, v.OldestFirst)
		}
		if v.Bars[0].Label != first || v.Bars[n-1].Label != last {
			t.Errorf("%s: shows %s to %s, want %s to %s", key, v.Bars[0].Label, v.Bars[n-1].Label, first, last)
		}
	}
	check("up", 20, 10, true, "Nov 06", "Nov 25")
	check("down", 20, 10, false, "Nov 25", "Nov 06") // newest first already
	check("iso", 7, 23, true, "2026-11-19", "2026-11-25")
	check("plans", 2, 1, false, "free", "pro")            // not dates: the first ones
	check("whole-month", 30, 0, true, "Oct 27", "Nov 25") // all fit; still dates, oldest first
}

func TestDatesInOrder(t *testing.T) {
	for _, c := range []struct {
		labels []string
		want   bool
	}{
		{[]string{"Dec 30", "Dec 31", "Jan 01", "Jan 02"}, true}, // a new year, no year in the label
		{[]string{"2025-12-31", "2026-01-01"}, true},
		{[]string{"2026-01-01", "2025-12-31"}, false},
		{[]string{"Jan 02", "Jan 01"}, false},
		{[]string{"Jan 01", "Dec 31"}, true}, // one year, start to end
		{[]string{"Nov 3", "Nov 3"}, false},
		{[]string{"2026-11-24T10:00:00Z", "2026-11-24T11:00:00Z"}, true},
		{[]string{"2026-11", "2026-12"}, true},
		{[]string{"free", "pro"}, false},
		{[]string{"1", "2", "3"}, false},
		{[]string{"Nov 3"}, false},
	} {
		if got := datesInOrder(c.labels); got != c.want {
			t.Errorf("datesInOrder(%q) = %v, want %v", c.labels, got, c.want)
		}
	}
}
