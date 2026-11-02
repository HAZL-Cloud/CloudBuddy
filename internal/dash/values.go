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
	// Before is the latest point at least d old.
	Before(series string, d time.Duration) *Point
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
	Message   string           `json:"message,omitempty"`
	Stale     bool             `json:"stale,omitempty"`
	UpdatedAt *time.Time       `json:"updatedAt,omitempty"`
	Number    *float64         `json:"number,omitempty"`
	Compare   *Compared        `json:"compare,omitempty"`
	Spark     []Point          `json:"spark,omitempty"`
	Series    []SeriesValue    `json:"series,omitempty"`
	Bars      []Bar            `json:"bars,omitempty"`
	Rows      []map[string]any `json:"rows,omitempty"`
	Columns   []Column         `json:"columns,omitempty"`
	More      int              `json:"more,omitempty"`
	Text      string           `json:"text,omitempty"`
	Status    string           `json:"status,omitempty"` // ok | warn | fail
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
	if err == nil {
		v, err = Lookup(v, path)
	}
	if err != nil {
		tv.State, tv.Message = "error", "CloudBuddy couldn't read the output: "+err.Error()
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
		tv.Series = []SeriesValue{{Label: t.Title, Bars: limitBars(bars, t.Limit, 200)}}
	case "bars":
		bars, err := Bars(v, t.LabelField, t.ValueField)
		if err != nil {
			return errorTile(tv, "CloudBuddy couldn't draw bars from the output: "+err.Error())
		}
		limit := t.Limit
		if limit == 0 {
			limit = 20
		}
		if len(bars) > limit {
			tv.More = len(bars) - limit
		}
		tv.Bars = limitBars(bars, limit, limit)
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

func limitBars(b []Bar, limit, def int) []Bar {
	if limit == 0 {
		limit = def
	}
	if len(b) > limit {
		return b[:limit]
	}
	return b
}

func lineSeries(t Tile, runs map[string]Runs, h History, tv TileValue) TileValue {
	tv.State = "ok"
	for _, se := range t.Series {
		label := se.Label
		if label == "" {
			label = se.Source
		}
		tv.Series = append(tv.Series, SeriesValue{Label: label, Points: h.Points(SeriesKey(se.Source, se.Path))})
		r := runs[se.Source]
		if r.Latest != nil && r.Latest.State != RunOK && tv.Message == "" {
			switch r.Latest.State {
			case RunWaiting:
				tv.Message = "A command for this chart is waiting for your OK."
			case RunBlocked:
				tv.Message = r.Latest.Note
			case RunDeclined:
				tv.Message = "You didn't allow a command for this chart."
			default:
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
		if p := h.Before(series, time.Hour); p != nil {
			return &Compared{Value: p.V, At: p.T, Label: "in the last hour"}
		}
		return nil
	case "7d":
		if p := h.Before(series, 7*24*time.Hour); p != nil {
			return &Compared{Value: p.V, At: p.T, Label: "in 7 days"}
		}
		return nil
	}
	if p := h.Before(series, 24*time.Hour); p != nil {
		return &Compared{Value: p.V, At: p.T, Label: "in 24 hours"}
	}
	// a younger dashboard: since its first number, once there is some history
	if p := h.Oldest(series); p != nil && latestAt.Sub(p.T) >= 30*time.Minute {
		return &Compared{Value: p.V, At: p.T, Label: "since " + p.T.Local().Format("Jan 2, 3:04 PM")}
	}
	return nil
}
