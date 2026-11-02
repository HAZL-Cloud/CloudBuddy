// Package dash holds what CloudBuddy needs to know about dashboards (D-067,
// D-068) without touching the database or running anything: the spec the
// agent saves and its checks, how a command's output becomes tile values,
// and which commands may run on a timer.
package dash

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Spec is a dashboard as the agent saves it.
type Spec struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Refresh     string   `json:"refresh"`
	Sources     []Source `json:"sources"`
	Tiles       []Tile   `json:"tiles"`
}

// Source is one command whose output feeds tiles.
type Source struct {
	Key     string `json:"key"`
	Label   string `json:"label,omitempty"`
	Command string `json:"command"`
	// Cwd is relative to the dashboard's folder, or a full path inside the
	// chat's project folders. Empty means the dashboard's folder.
	Cwd            string `json:"cwd,omitempty"`
	Format         string `json:"format,omitempty"` // auto | json | number | csv | tsv | lines | text
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
}

// Tile is one box on the dashboard, drawn by CloudBuddy's own widget kit.
type Tile struct {
	Key    string `json:"key"`
	Type   string `json:"type"` // number | line | bars | table | text | status
	Title  string `json:"title"`
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
	Width  int    `json:"width,omitempty"` // 1 to 4 columns
	Note   string `json:"note,omitempty"`

	// number (and line, bars, table columns)
	Format   string `json:"format,omitempty"` // number | currency | percent | bytes | duration
	Currency string `json:"currency,omitempty"`
	Decimals *int   `json:"decimals,omitempty"`
	Unit     string `json:"unit,omitempty"`
	Good     string `json:"good,omitempty"`    // up | down: which way is good news
	Compare  string `json:"compare,omitempty"` // previous | 1h | 24h | 7d

	// line: history of numbers, or points from the output
	Series []Series `json:"series,omitempty"`

	// bars, table and line from rows
	LabelField string   `json:"labelField,omitempty"`
	ValueField string   `json:"valueField,omitempty"`
	Columns    []Column `json:"columns,omitempty"`
	Limit      int      `json:"limit,omitempty"`

	// status
	WarnAbove *float64 `json:"warnAbove,omitempty"`
	FailAbove *float64 `json:"failAbove,omitempty"`
	WarnBelow *float64 `json:"warnBelow,omitempty"`
	FailBelow *float64 `json:"failBelow,omitempty"`
	OkValues  []string `json:"okValues,omitempty"`
}

// Series is one line on a line tile: a number tracked over time.
type Series struct {
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
	Label  string `json:"label,omitempty"`
}

// Column is one column of a table tile.
type Column struct {
	Field  string `json:"field"`
	Label  string `json:"label,omitempty"`
	Format string `json:"format,omitempty"`
}

const (
	MaxSources  = 12
	MaxTiles    = 24
	MaxCommand  = 4000
	MinRefresh  = time.Minute
	MaxRefresh  = 24 * time.Hour
	DefTimeout  = 60
	MaxTimeout  = 300
	DefaultCols = 4
)

var keyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

var tileTypes = map[string]bool{"number": true, "line": true, "bars": true, "table": true, "text": true, "status": true}
var formats = map[string]bool{"": true, "auto": true, "json": true, "number": true, "csv": true, "tsv": true, "lines": true, "text": true}
var numberFormats = map[string]bool{"": true, "number": true, "currency": true, "percent": true, "bytes": true, "duration": true}
var compares = map[string]bool{"": true, "previous": true, "1h": true, "24h": true, "7d": true}

// RefreshChoices are the intervals offered on the dashboard page.
var RefreshChoices = []string{"1m", "5m", "15m", "30m", "1h", "6h", "12h", "1d"}

// ParseRefresh reads "15m", "1h", "1d" or a number of seconds.
func ParseRefresh(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("refresh is missing: say how often to refresh, like \"15m\" or \"1h\"")
	}
	var d time.Duration
	switch {
	case strings.HasSuffix(s, "d"):
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("refresh %q isn't a time like \"15m\", \"1h\" or \"1d\"", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	default:
		if n, err := strconv.Atoi(s); err == nil {
			d = time.Duration(n) * time.Second
		} else if p, err := time.ParseDuration(s); err == nil {
			d = p
		} else {
			return 0, fmt.Errorf("refresh %q isn't a time like \"15m\", \"1h\" or \"1d\"", s)
		}
	}
	if d < MinRefresh {
		return 0, fmt.Errorf("refresh %q is too often: once a minute is the most", s)
	}
	if d > MaxRefresh {
		return 0, fmt.Errorf("refresh %q is too long: once a day is the least", s)
	}
	return d, nil
}

// RefreshLabel writes an interval the way the page shows it.
func RefreshLabel(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}

// Problems is every mistake found in a spec, so the agent can fix them all
// in one go.
type Problems []string

func (p Problems) Error() string {
	return "The dashboard wasn't saved. Fix these and save again:\n- " + strings.Join(p, "\n- ")
}

// Check normalizes a spec in place and returns its problems.
func Check(s *Spec) error {
	var p Problems
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }

	s.Title = strings.TrimSpace(s.Title)
	s.Description = strings.TrimSpace(s.Description)
	if s.Title == "" {
		add("title is missing")
	}
	if len([]rune(s.Title)) > 80 {
		add("title is longer than 80 characters")
	}
	if len([]rune(s.Description)) > 400 {
		add("description is longer than 400 characters")
	}
	if d, err := ParseRefresh(s.Refresh); err != nil {
		add("%s", err.Error())
	} else {
		s.Refresh = RefreshLabel(d)
	}

	if len(s.Sources) == 0 {
		add("sources is empty: add at least one command that fetches the data")
	}
	if len(s.Sources) > MaxSources {
		add("there are %d sources; %d is the most", len(s.Sources), MaxSources)
	}
	sources := map[string]*Source{}
	for i := range s.Sources {
		src := &s.Sources[i]
		src.Key = strings.TrimSpace(src.Key)
		src.Command = strings.TrimSpace(src.Command)
		src.Format = strings.ToLower(strings.TrimSpace(src.Format))
		name := fmt.Sprintf("sources[%d]", i)
		if src.Key != "" {
			name = fmt.Sprintf("source %q", src.Key)
		}
		switch {
		case src.Key == "":
			add("%s has no key", name)
		case !keyRE.MatchString(src.Key):
			add("%s: keys use lower-case letters, digits, - and _ (up to 32)", name)
		case sources[src.Key] != nil:
			add("%s is there twice", name)
		default:
			sources[src.Key] = src
		}
		if src.Command == "" {
			add("%s has no command", name)
		}
		if len(src.Command) > MaxCommand {
			add("%s: the command is longer than %d characters; put it in a script", name, MaxCommand)
		}
		if !formats[src.Format] {
			add("%s: format %q isn't one of auto, json, number, csv, tsv, lines, text", name, src.Format)
		}
		if src.Format == "" {
			src.Format = "auto"
		}
		if src.TimeoutSeconds < 0 || src.TimeoutSeconds > MaxTimeout {
			add("%s: timeoutSeconds must be between 1 and %d", name, MaxTimeout)
		}
		if src.TimeoutSeconds == 0 {
			src.TimeoutSeconds = DefTimeout
		}
	}

	if len(s.Tiles) == 0 {
		add("tiles is empty: add at least one tile")
	}
	if len(s.Tiles) > MaxTiles {
		add("there are %d tiles; %d is the most", len(s.Tiles), MaxTiles)
	}
	tileKeys := map[string]bool{}
	for i := range s.Tiles {
		t := &s.Tiles[i]
		t.Type = strings.ToLower(strings.TrimSpace(t.Type))
		t.Title = strings.TrimSpace(t.Title)
		t.Key = strings.TrimSpace(t.Key)
		name := fmt.Sprintf("tiles[%d]", i)
		if t.Title != "" {
			name = fmt.Sprintf("tile %q", t.Title)
		}
		if t.Key == "" {
			t.Key = slug(t.Title, i)
		}
		if !keyRE.MatchString(t.Key) {
			add("%s: keys use lower-case letters, digits, - and _ (up to 32)", name)
		} else if tileKeys[t.Key] {
			add("%s: key %q is used by another tile", name, t.Key)
		}
		tileKeys[t.Key] = true
		if !tileTypes[t.Type] {
			add("%s: type %q isn't one of number, line, bars, table, text, status", name, t.Type)
		}
		if t.Title == "" {
			add("%s has no title", name)
		}
		if t.Width == 0 {
			t.Width = defaultWidth(t.Type)
		}
		if t.Width < 1 || t.Width > DefaultCols {
			add("%s: width must be 1 to %d", name, DefaultCols)
		}
		if !numberFormats[t.Format] {
			add("%s: format %q isn't one of number, currency, percent, bytes, duration", name, t.Format)
		}
		if t.Good != "" && t.Good != "up" && t.Good != "down" {
			add("%s: good must be \"up\" or \"down\"", name)
		}
		if !compares[t.Compare] {
			add("%s: compare must be previous, 1h, 24h or 7d", name)
		}
		if t.Limit < 0 || t.Limit > 500 {
			add("%s: limit must be between 1 and 500", name)
		}
		if err := checkPath(t.Path); err != nil {
			add("%s: path %q: %s", name, t.Path, err)
		}
		needSource := func(key, where string) {
			if key == "" {
				add("%s: %s has no source", name, where)
			} else if sources[key] == nil {
				add("%s: %s names source %q, which isn't in sources", name, where, key)
			}
		}
		switch t.Type {
		case "line":
			if len(t.Series) == 0 {
				needSource(t.Source, "the tile")
			}
			if len(t.Series) > 6 {
				add("%s: 6 series is the most", name)
			}
			for j, se := range t.Series {
				needSource(se.Source, fmt.Sprintf("series[%d]", j))
				if err := checkPath(se.Path); err != nil {
					add("%s: series[%d] path %q: %s", name, j, se.Path, err)
				}
			}
		default:
			needSource(t.Source, "the tile")
			if len(t.Series) > 0 {
				add("%s: only line tiles have series", name)
			}
		}
	}
	if len(p) > 0 {
		return p
	}
	return nil
}

func defaultWidth(typ string) int {
	switch typ {
	case "number", "status":
		return 1
	case "table":
		return 4
	}
	return 2
}

func slug(title string, i int) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 28 {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = fmt.Sprintf("tile-%d", i+1)
	}
	return s
}

// SeriesKey names a number tracked over time: one per source and path, so
// a number tile and a line tile of the same number share their history.
func SeriesKey(source, path string) string { return source + "|" + strings.TrimSpace(path) }

// Tracked lists the numbers a refresh records for history.
func (s Spec) Tracked() map[string][2]string {
	out := map[string][2]string{}
	for _, t := range s.Tiles {
		switch t.Type {
		case "number", "status":
			out[SeriesKey(t.Source, t.Path)] = [2]string{t.Source, t.Path}
		case "line":
			if len(t.Series) == 0 {
				out[SeriesKey(t.Source, t.Path)] = [2]string{t.Source, t.Path}
			}
			for _, se := range t.Series {
				out[SeriesKey(se.Source, se.Path)] = [2]string{se.Source, se.Path}
			}
		}
	}
	return out
}

// Source finds a source by key.
func (s Spec) Source(key string) (Source, bool) {
	for _, src := range s.Sources {
		if src.Key == key {
			return src, true
		}
	}
	return Source{}, false
}
