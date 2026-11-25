package dash

import (
	"fmt"
	"strings"
	"time"
)

// Run states, as stored for each run of a command.
const (
	RunOK       = "ok"
	RunFailed   = "failed"
	RunWaiting  = "waiting"  // needs the user's OK before it can run on the timer
	RunBlocked  = "blocked"  // can never run from a dashboard
	RunDeclined = "declined" // the user said no
)

// RunView is one stored run of a command.
type RunView struct {
	State      string    `json:"state"`
	ExitCode   *int      `json:"exitCode,omitempty"`
	Output     string    `json:"output"`
	Truncated  bool      `json:"truncated,omitempty"`
	Note       string    `json:"note,omitempty"`
	At         time.Time `json:"at"`
	DurationMs int       `json:"durationMs"`
}

// Point is one number at one time.
type Point struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// History gives a tile the numbers recorded before.
type History interface {
	// Points are the series' points in the page's time range, oldest first.
	Points(series string) []Point
	// Before is the latest point at least d older than at: the time of the
	// value the tile shows, which is older than now when the last refresh
	// failed. Counting from now compared an old value with itself ("No
	// change in 7 days" on a number that stopped refreshing 10 days ago).
	Before(series string, at time.Time, d time.Duration) *Point
	// Oldest is the series' first point.
	Oldest(series string) *Point
	// Previous is the point before the latest one.
	Previous(series string) *Point
}

// Compared is the earlier value a number tile shows its change against.
type Compared struct {
	Value float64   `json:"value"`
	At    time.Time `json:"at"`
	Label string    `json:"label"`
}

// SeriesValue is one line of a line tile: points over time, or one point
// per row of the output (Bars).
type SeriesValue struct {
	Label  string  `json:"label"`
	Points []Point `json:"points,omitempty"`
	Bars   []Bar   `json:"bars,omitempty"`
}

// TileValue is what one tile shows right now.
type TileValue struct {
	Key string `json:"key"`
	// ok | pending | waiting | blocked | declined | error
	State string `json:"state"`
	// Message explains a state other than ok, or that the value is old.
	Message string `json:"message,omitempty"`
	// Stale: the latest run failed; the value is from the last that worked.
	Stale bool `json:"stale,omitempty"`
	// Waiting: a line chart shows its history while a command for it waits
	// for the user's OK.
	Waiting   bool             `json:"waiting,omitempty"`
	UpdatedAt *time.Time       `json:"updatedAt,omitempty"`
	Number    *float64         `json:"number,omitempty"`
	Compare   *Compared        `json:"compare,omitempty"`
	Spark     []Point          `json:"spark,omitempty"`
	Series    []SeriesValue    `json:"series,omitempty"`
	Bars      []Bar            `json:"bars,omitempty"`
	Rows      []map[string]any `json:"rows,omitempty"`
	Columns   []Column         `json:"columns,omitempty"`
	More      int              `json:"more,omitempty"`
	// OldestFirst: the bars are dates, oldest first. The newest are at the
	// end, and any left out (More) are the earliest.
	OldestFirst bool   `json:"oldestFirst,omitempty"`
	Text        string `json:"text,omitempty"`
	Status      string `json:"status,omitempty"` // ok | warn | fail
}

// Runs are a source's latest run and its latest run that worked.
type Runs struct {
	Latest *RunView
	LastOK *RunView
}

// Values works out every tile's value from the commands' latest runs.
func Values(spec Spec, runs map[string]Runs, h History, now time.Time) []TileValue {
	out := make([]TileValue, 0, len(spec.Tiles))
	for _, t := range spec.Tiles {
		out = append(out, tileValue(spec, t, runs, h, now))
	}
	return out
}

// sourceValue is a source's parsed output at a path, from the latest run
// that worked. tv gets the state when there is nothing to show.
func sourceValue(spec Spec, key, path string, runs map[string]Runs, tv *TileValue) (any, *RunView, bool) {
	src, _ := spec.Source(key)
	r := runs[key]
	if r.Latest == nil {
		tv.State, tv.Message = "pending", "Not fetched yet."
		return nil, nil, false
	}
	// A command waiting for an OK shows nothing, even when an earlier version
	// of its files printed something: the changed script's numbers may mean
	// something else.
	switch r.Latest.State {
	case RunWaiting:
		tv.State, tv.Message = "waiting", "CloudBuddy runs this tile's command once you allow it."
	case RunBlocked:
		tv.State, tv.Message = "blocked", r.Latest.Note
	case RunDeclined:
		tv.State, tv.Message = "declined", "So it doesn't run, and this tile stays empty."
	}
	if tv.State != "" {
		return nil, nil, false
	}
	use := r.Latest
	if r.Latest.State != RunOK {
		if r.LastOK == nil {
			tv.State, tv.Message = "error", failMessage(r.Latest)
			return nil, nil, false
		}
		use = r.LastOK
		tv.Stale = true
		tv.Message = "The last refresh failed, so this is from " + use.At.Local().Format("Jan 2, 3:04 PM") + ". " + failMessage(r.Latest)
	}
	at := use.At
	tv.UpdatedAt = &at
	if use.Truncated {
		tv.State, tv.Message = "error", "The output was too big to keep in full. Ask the agent to fetch less (fewer rows or fields)."
		return nil, nil, false
	}
	v, err := Parse(use.Output, src.Format)
	if err != nil {
		tv.State, tv.Message = "error", "CloudBuddy couldn't read the output: "+err.Error()
		return nil, nil, false
	}
	if v, err = Lookup(v, path); err != nil {
		tv.State, tv.Message = "error", "CloudBuddy couldn't find this tile's value in the output: "+err.Error()
		return nil, nil, false
	}
	return v, use, true
}

func failMessage(r *RunView) string {
	msg := "The command failed"
	if r.ExitCode != nil {
		msg += fmt.Sprintf(" (exit code %d)", *r.ExitCode)
	}
	if n := strings.TrimSpace(r.Note); n != "" {
		msg += ": " + short(n, 300)
	} else {
		msg += "."
	}
	return msg
}

func tileValue(spec Spec, t Tile, runs map[string]Runs, h History, now time.Time) TileValue {
	tv := TileValue{Key: t.Key}
	if t.Type == "line" && len(t.Series) > 0 {
		return lineSeries(t, runs, h, tv)
	}
	if t.Type == "status" && strings.TrimSpace(t.Path) == "" {
		return exitStatus(t, runs, tv)
	}
	v, run, ok := sourceValue(spec, t.Source, t.Path, runs, &tv)
	if !ok {
		return tv
	}
	tv.State = "ok"
	series := SeriesKey(t.Source, t.Path)
	switch t.Type {
	case "number":
		n, ok := Number(v)
		if !ok {
			return errorTile(tv, "The value isn't a number: "+short(Text(v), 80))
		}
		tv.Number = &n
		tv.Spark = h.Points(series)
		tv.Compare = compareWith(t.Compare, series, h, run.At)
	case "line":
		if n, ok := Number(v); ok {
			tv.Number = &n
			tv.Series = []SeriesValue{{Label: t.Title, Points: h.Points(series)}}
			break
		}
		bars, err := Bars(v, t.LabelField, t.ValueField)
		if err != nil {
			return errorTile(tv, "CloudBuddy couldn't draw a line from the output: "+err.Error())
		}
		kept, _, _ := limitBars(bars, t.Limit, 200)
		tv.Series = []SeriesValue{{Label: t.Title, Bars: kept}}
	case "bars":
		bars, err := Bars(v, t.LabelField, t.ValueField)
		if err != nil {
			return errorTile(tv, "CloudBuddy couldn't draw bars from the output: "+err.Error())
		}
		tv.Bars, tv.More, tv.OldestFirst = limitBars(bars, t.Limit, defaultBars)
	case "table":
		rows, cols, err := Rows(v)
		if err != nil {
			return errorTile(tv, "CloudBuddy couldn't make a table from the output: "+err.Error())
		}
		limit := t.Limit
		if limit == 0 {
			limit = 50
		}
		if len(rows) > limit {
			tv.More = len(rows) - limit
			rows = rows[:limit]
		}
		tv.Rows = rows
		if len(t.Columns) > 0 {
			tv.Columns = t.Columns
		} else {
			for _, c := range cols {
				tv.Columns = append(tv.Columns, Column{Field: c, Label: c})
			}
		}
	case "text":
		tv.Text = short(Text(v), 20000)
	case "status":
		tv.Status, tv.Text = statusOf(t, v)
		if run.ExitCode != nil && *run.ExitCode != 0 {
			tv.Status = "fail"
		}
		if n, ok := Number(v); ok {
			tv.Number = &n
			tv.Spark = h.Points(series)
		}
	}
	return tv
}

func errorTile(tv TileValue, msg string) TileValue {
	tv.State, tv.Message = "error", msg
	return tv
}

// defaultBars is how many bars a bars tile shows unless it sets a limit: a
// month of days. Longer lists scroll inside the tile.
const defaultBars = 31

// limitBars keeps at most limit bars (def when limit is 0), says how many it
// left out, and whether the bars are dates, oldest first. Most lists keep
// their first bars: a top-ten sorted by value, or days newest first. Days
// oldest first keep their last bars instead, so the newest days show and the
// oldest are the ones left out (2026-11-24: a 30-day chart showed its 20
// oldest days).
func limitBars(b []Bar, limit, def int) (kept []Bar, more int, oldestFirst bool) {
	if limit == 0 {
		limit = def
	}
	labels := make([]string, len(b))
	for i, x := range b {
		labels[i] = x.Label
	}
	oldestFirst = datesInOrder(labels)
	switch {
	case len(b) <= limit:
		return b, 0, oldestFirst
	case oldestFirst:
		return b[len(b)-limit:], len(b) - limit, true
	}
	return b[:limit], len(b) - limit, false
}

// dateLayouts are the ways commands write a day or a time on a label.
var dateLayouts = []string{
	time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006-01-02", "2006/01/02", "2006-01",
	"Jan 2, 2006", "2 Jan 2006", "Jan 2006", "January 2006",
	"Jan 2", "2 Jan", "January 2", "2 January", "Mon Jan 2", "Mon, Jan 2",
}

// datesInOrder says whether every label is a date in one layout, oldest
// first. Dates without a year may pass from December to January once a
// year.
func datesInOrder(labels []string) bool {
	if len(labels) < 2 {
		return false
	}
	for _, layout := range dateLayouts {
		var prev time.Time
		years, ok := 0, true
		for i, l := range labels {
			t, err := time.Parse(layout, strings.TrimSpace(l))
			if err != nil {
				ok = false
				break
			}
			t = t.AddDate(years, 0, 0)
			if i > 0 && t.Before(prev) {
				if t.Year() == years && prev.Month() == time.December && t.Month() == time.January {
					years++ // no year in the label: the new year began
					t = t.AddDate(1, 0, 0)
				} else {
					ok = false
					break
				}
			}
			prev = t
		}
		if ok {
			// the same date over and over isn't a series in time
			first, _ := time.Parse(layout, strings.TrimSpace(labels[0]))
			return prev.After(first)
		}
	}
	return false
}

func lineSeries(t Tile, runs map[string]Runs, h History, in TileValue) (tv TileValue) {
	tv = in
	tv.State = "ok"
	points, firstState := 0, ""
	defer func() {
		// nothing to draw and a command that can't run: say why, not "fills in"
		if points == 0 && firstState != "" {
			tv.State = firstState
		}
	}()
	for _, se := range t.Series {
		label := se.Label
		if label == "" {
			label = se.Source
		}
		pts := h.Points(SeriesKey(se.Source, se.Path))
		points += len(pts)
		tv.Series = append(tv.Series, SeriesValue{Label: label, Points: pts})
		r := runs[se.Source]
		if r.Latest != nil && r.Latest.State != RunOK {
			switch {
			case firstState != "":
			case r.Latest.State == RunWaiting:
				tv.Message, tv.Stale, tv.Waiting, firstState = "A command for this chart is waiting for your OK.", false, true, "waiting"
			case r.Latest.State == RunBlocked:
				tv.Message, tv.Stale, firstState = r.Latest.Note, false, "blocked"
			case r.Latest.State == RunDeclined:
				tv.Message, tv.Stale, firstState = "You didn't allow a command for this chart.", false, "declined"
			case tv.Message == "":
				tv.Message, tv.Stale = failMessage(r.Latest), true
			}
		}
		if r.LastOK != nil && (tv.UpdatedAt == nil || r.LastOK.At.After(*tv.UpdatedAt)) {
			at := r.LastOK.At
			tv.UpdatedAt = &at
		}
	}
	return tv
}

// exitStatus: a status tile without a path is OK when the command succeeds.
func exitStatus(t Tile, runs map[string]Runs, tv TileValue) TileValue {
	r := runs[t.Source]
	if r.Latest == nil {
		tv.State, tv.Message = "pending", "Not checked yet."
		return tv
	}
	switch r.Latest.State {
	case RunWaiting:
		tv.State, tv.Message = "waiting", "CloudBuddy runs this tile's command once you allow it."
		return tv
	case RunBlocked:
		tv.State, tv.Message = "blocked", r.Latest.Note
		return tv
	case RunDeclined:
		tv.State, tv.Message = "declined", "So it doesn't run, and this tile stays empty."
		return tv
	}
	tv.State = "ok"
	at := r.Latest.At
	tv.UpdatedAt = &at
	if r.Latest.State == RunOK {
		tv.Status, tv.Text = "ok", "OK"
		return tv
	}
	tv.Status, tv.Text, tv.Message = "fail", "Failing", failMessage(r.Latest)
	return tv
}

func statusOf(t Tile, v any) (string, string) {
	if n, ok := v.(float64); ok {
		switch {
		case t.FailAbove != nil && n > *t.FailAbove, t.FailBelow != nil && n < *t.FailBelow:
			return "fail", Text(n)
		case t.WarnAbove != nil && n > *t.WarnAbove, t.WarnBelow != nil && n < *t.WarnBelow:
			return "warn", Text(n)
		}
		return "ok", Text(n)
	}
	if b, ok := v.(bool); ok {
		if b {
			return "ok", "OK"
		}
		return "fail", "Failing"
	}
	s := strings.TrimSpace(Text(v))
	if len(t.OkValues) == 0 {
		for _, good := range []string{"ok", "healthy", "up", "pass", "passing", "green", "true", "running", "active"} {
			if strings.EqualFold(s, good) {
				return "ok", s
			}
		}
		return "warn", s
	}
	for _, good := range t.OkValues {
		if strings.EqualFold(s, strings.TrimSpace(good)) {
			return "ok", s
		}
	}
	return "fail", s
}

func compareWith(mode, series string, h History, latestAt time.Time) *Compared {
	switch mode {
	case "previous":
		if p := h.Previous(series); p != nil {
			return &Compared{Value: p.V, At: p.T, Label: "since the last refresh"}
		}
		return nil
	case "1h":
		if p := h.Before(series, latestAt, time.Hour); p != nil {
			return &Compared{Value: p.V, At: p.T, Label: "in the last hour"}
		}
		return nil
	case "7d":
		if p := h.Before(series, latestAt, 7*24*time.Hour); p != nil {
			return &Compared{Value: p.V, At: p.T, Label: "in 7 days"}
		}
		return nil
	}
	if p := h.Before(series, latestAt, 24*time.Hour); p != nil {
		return &Compared{Value: p.V, At: p.T, Label: "in 24 hours"}
	}
	// a younger dashboard: since its first number, once there is some history
	if p := h.Oldest(series); p != nil && latestAt.Sub(p.T) >= 30*time.Minute {
		return &Compared{Value: p.V, At: p.T, Label: "since " + p.T.Local().Format("Jan 2, 3:04 PM")}
	}
	return nil
}
