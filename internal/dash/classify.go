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

// Classify decides whether a dashboard command may run on a timer. There is
// nobody to ask, so:
//
//  1. A command must be inside the dashboard grammar CloudBuddy parses itself
//     (recognize.ParseDashCommand); anything else can never run, because
//     CloudBuddy can't tell what it runs or hold it to the user's OK. Steps
//     that need more go in a script file.
//  2. Anything the chat gate would ask about (deletes, Can't undo, Moves
//     money, Reaches people, sends, changes in live mode), SQL that changes
//     data, CloudBuddy's own files, background commands and connections
//     turned Off can never run.
//  3. A command made only of recognized reads runs by itself
//     (recognize.StrictRead).
//  4. Everything else runs once the user allows it.
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
	segs, why := recognize.ParseDashCommand(in.Command)
	if why != "" {
		return never("CloudBuddy can't read this command fully (" + why + "), so it can't tell what it runs or hold it to your OK. Put the steps in a script file in the project and run that.")
	}
	if d.Background {
		return never("Dashboards can't start commands that keep running in the background.")
	}
	if rule == model.RuleOff && d.Kind == "cloud" {
		return never(d.Provider + " is turned off in Connections.")
	}
	if prog := recognize.LocalChange(segs); prog != "" {
		return never("Dashboards only fetch data, and this changes or deletes files or processes (" + prog + ").")
	}
	if w := recognize.SQLWrite(in.Command); w != "" {
		return never("Dashboards only fetch data, and this query changes data (" + strings.ToUpper(w) + ").")
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
	if has(model.TagLive) && d.Action != model.ActionRead {
		return never("Dashboards only fetch data, and this changes something in live mode.")
	}

	engine, notRead := recognize.StrictRead(segs, in.Env, in.Adapters)
	if rule == model.RuleReadOnly && d.Kind == "cloud" && notRead != "" {
		return never(d.Provider + " is read-only in Connections, and CloudBuddy can't tell this only reads.")
	}
	local := ""
	if in.LocalProgram != nil {
		local = in.LocalProgram(in.Command)
	}
	switch {
	case local != "":
		return needs("It runs " + local + " from the project, which can change at any time.")
	case len(d.ExternalFiles) > 0:
		return needs("It reads files outside the project: " + strings.Join(d.ExternalFiles, ", ") + ".")
	case notRead != "":
		return needs("CloudBuddy can't tell that this only reads: " + notRead + ".")
	}
	if engine == "psql" {
		// a Postgres service file can turn the read-only session back off
		for _, k := range []string{"PGSERVICE", "PGSERVICEFILE", "PGSYSCONFDIR"} {
			if in.Env.AgentEnv[k] != "" {
				return needs("It connects through a Postgres service file (" + k + "), which can turn CloudBuddy's read-only session off.")
			}
		}
	}
	c.Verdict, c.SQL = Read, engine
	if engine != "" {
		c.Provider = "Database"
	}
	return c
}
