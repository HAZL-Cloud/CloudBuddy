package dash

import "time"

// Summary is one dashboard in the list (docs/api.md).
type Summary struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	ConversationID string     `json:"conversationId,omitempty"`
	Refresh        string     `json:"refresh"`
	Paused         bool       `json:"paused"`
	Running        bool       `json:"running"`
	LastRunAt      *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt      *time.Time `json:"nextRunAt,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	// NeedsOK counts commands waiting for the user's OK; Failing counts
	// commands whose latest run failed or can't run.
	NeedsOK   int        `json:"needsOk"`
	Failing   int        `json:"failing"`
	TileCount int        `json:"tileCount"`
	Headlines []Headline `json:"headlines"`
}

// Headline is a number tile shown on the dashboard's card in the list.
type Headline struct {
	Tile  Tile      `json:"tile"`
	Value TileValue `json:"value"`
}

// Approval is the user's answer for a command that isn't a read.
type Approval struct {
	Allowed   bool      `json:"allowed"`
	DecidedAt time.Time `json:"decidedAt"`
	// Changed is true when the command, its folder or the files it runs
	// changed since the user answered: it needs a new answer.
	Changed bool `json:"changed,omitempty"`
}

// SourceView is one command on the dashboard page.
type SourceView struct {
	Key     string `json:"key"`
	Label   string `json:"label,omitempty"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	Format  string `json:"format"`
	// Verdict is read, needs_ok or never (D-067), with the reason for the
	// last two.
	Verdict  string    `json:"verdict"`
	Reason   string    `json:"reason,omitempty"`
	Approval *Approval `json:"approval,omitempty"`
	LastRun  *RunView  `json:"lastRun,omitempty"`
	// Revision names exactly what the page shows for a command that needs
	// the user's OK: the command, its folder and the files it runs, as of
	// its last run. An answer must send it back.
	Revision string `json:"revision,omitempty"`
}

// Detail is the dashboard page.
type Detail struct {
	Summary
	Workdir string        `json:"workdir"`
	Range   string        `json:"range"`
	Tiles   []Tile        `json:"tiles"`
	Values  []TileValue   `json:"values"`
	Sources []SourceView  `json:"sources"`
	Choices []string      `json:"refreshChoices"`
	Ranges  []RangeChoice `json:"rangeChoices"`
}

// RangeChoice is a time range for charts.
type RangeChoice struct {
	Key   string        `json:"key"`
	Label string        `json:"label"`
	Span  time.Duration `json:"-"`
}

// Ranges are the chart time ranges the page offers.
var Ranges = []RangeChoice{{"1h", "1 hour", time.Hour}, {"24h", "24 hours", 24 * time.Hour},
	{"7d", "7 days", 7 * 24 * time.Hour}, {"30d", "30 days", 30 * 24 * time.Hour}}

// RangeFor finds a range by key; the default is 24 hours.
func RangeFor(key string) RangeChoice {
	for _, r := range Ranges {
		if r.Key == key {
			return r
		}
	}
	return Ranges[1]
}
