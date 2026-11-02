package dash

import (
	"strings"

	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
)

// Verdicts for a dashboard command (D-067).
const (
	Read    = "read"     // runs on the timer by itself
	NeedsOK = "needs_ok" // runs once the user allows it
	Never   = "never"    // can't be a dashboard command
)

// Class is what CloudBuddy decided about one dashboard command.
type Class struct {
	Verdict string `json:"verdict"`
	// Reason says, in plain words, why a command needs the user's OK or can
	// never run from a dashboard.
	Reason   string `json:"reason,omitempty"`
	SQL      string `json:"sql,omitempty"` // psql | sqlite3 for a database read
	Provider string `json:"provider,omitempty"`
}

// Input is a command and what the classifier needs to know about it.
type Input struct {
	Command  string
	Cwd      string
	Env      recognize.Env
	Adapters *recognize.Adapters
	// Rule returns the user's Connections rule for a provider ("" for the
	// default).
	Rule func(provider string) string
	// LocalProgram reports a program the command runs from inside the
	// project (a project file could be changed at any time), or "".
	LocalProgram func(command string) string
}

// Classify decides whether a dashboard command may run on a timer. It is
// stricter than the chat gate: there is nobody to ask, so only what
// CloudBuddy recognizes as a read runs alone, and anything the chat gate
// would ask about can't run from a dashboard at all.
func Classify(in Input) Class {
	d := recognize.Recognize(recognize.Call{Agent: "claude", Via: "shell", Command: in.Command, WantsNetwork: true, Cwd: in.Cwd},
		in.Env, in.Adapters)
	c := Class{Provider: d.Provider}
	rule := ""
	if in.Rule != nil {
		rule = in.Rule(d.Provider)
	}
	has := func(t model.Tag) bool {
		for _, x := range d.Tags {
			if x == t {
				return true
			}
		}
		return false
	}
	never := func(reason string) Class { c.Verdict, c.Reason = Never, reason; return c }
	needs := func(reason string) Class { c.Verdict, c.Reason = NeedsOK, reason; return c }

	if len(d.Protected) > 0 {
		return never("It touches CloudBuddy's own setup (" + strings.Join(d.Protected, ", ") + ").")
	}
	if d.Background {
		return never("Dashboards can't start commands that keep running in the background.")
	}
	if rule == model.RuleOff && d.Kind == "cloud" {
		return never(d.Provider + " is turned off in Connections.")
	}

	local := ""
	if in.LocalProgram != nil {
		local = in.LocalProgram(in.Command)
	}
	// A database query: one read runs alone; a query that changes data never.
	sql := recognize.SQLRead(in.Command)
	if sql.Write != "" {
		return never("Dashboards only fetch data, and this query changes data (" + strings.ToUpper(sql.Write) + ").")
	}
	if sql.Engine != "" && local == "" {
		c.Verdict, c.SQL, c.Provider = Read, sql.Engine, "Database"
		return c
	}

	if d.Action == model.ActionDelete {
		if d.HiddenDelete != "" {
			return never("Dashboards only fetch data, and this looks like a delete (“" + d.HiddenDelete + "”).")
		}
		return never("Dashboards only fetch data, and this is a delete.")
	}
	var tags []string
	for _, t := range []struct {
		tag   model.Tag
		label string
	}{{model.TagIrreversible, "Can't undo"}, {model.TagMoney, "Moves money"}, {model.TagPeople, "Reaches people"}} {
		if has(t.tag) {
			tags = append(tags, t.label)
		}
	}
	if len(tags) > 0 {
		return never("Dashboards only fetch data, and this is marked " + strings.Join(tags, ", ") + ".")
	}
	if d.Action == model.ActionSend {
		return never("Dashboards only fetch data, and this sends something.")
	}
	isRead := d.Action == model.ActionRead && d.ReviewedRead
	if has(model.TagLive) && d.Action != model.ActionRead {
		return never("Dashboards only fetch data, and this changes something in live mode.")
	}
	if rule == model.RuleReadOnly && d.Kind == "cloud" && !isRead {
		return never(d.Provider + " is read-only in Connections, and CloudBuddy can't tell this only reads.")
	}

	switch {
	case local != "":
		return needs("It runs " + local + " from the project, which can change at any time.")
	case recognize.DashboardWrites(in.Command):
		return needs("It writes to a file.")
	case len(d.ExternalFiles) > 0:
		return needs("It reads files outside the project: " + strings.Join(d.ExternalFiles, ", ") + ".")
	case d.Opaque:
		return needs("It uses shell features CloudBuddy can't read, so it can't be sure this only reads.")
	case recognize.TextOnly(in.Command):
		c.Verdict, c.Provider = Read, "Local"
		return c
	case sql.Engine == "" && sql.Why != "" && sql.Why != "it isn't psql or sqlite3":
		return needs("CloudBuddy can't be sure this database query only reads: " + sql.Why + ".")
	case d.Network == "B":
		return needs("It runs a program or script, and CloudBuddy can't see what it does inside.")
	case d.Action == model.ActionCreate || d.Action == model.ActionChange:
		return needs("It looks like it changes something (" + string(d.Action) + "), not only reads.")
	case d.Action == model.ActionRead && !d.ReviewedRead && d.API != "" && strings.Contains(d.API, " "):
		return needs("CloudBuddy can't be sure this web request only reads: some services change things on a GET.")
	case !isRead:
		return needs("CloudBuddy can't tell that this only reads.")
	}
	c.Verdict = Read
	return c
}
