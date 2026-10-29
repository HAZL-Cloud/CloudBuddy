// Package policy decides allow, ask or deny for a recognized call. The rows
// mirror "The rules (in order)" in docs/security.md and are evaluated top to
// bottom; the first match wins. Since D-065 everything runs as it would in a
// terminal unless it can't be undone or the user's connection rules say
// otherwise.
package policy

import (
	"fmt"
	"strings"

	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
)

type Verdict string

const (
	Allow Verdict = "allow"
	Ask   Verdict = "ask"
	Deny  Verdict = "deny"
)

type Decision struct {
	Verdict   Verdict
	Row       int
	Reason    string
	LossTitle string
	Loss      []string
	Warning   string
}

// Context is what the policy knows beyond the call itself.
type Context struct {
	// Rule is the connection rule for the call's provider; empty means the
	// default ("Ask before anything I can't undo").
	Rule string
	// CanWrapCode is false for agents that can't keep CloudBuddy's file rules
	// on approved project code with network (Codex, D-038).
	CanWrapCode bool
}

// KindBWarning is shown on an approval for project code with network (D-029).
const KindBWarning = "CloudBuddy can't see what this does inside. Anything it changes won't show on the canvas."

func Decide(d recognize.Draft, ctx Context) Decision {
	rule := ctx.Rule
	if rule == "" {
		rule = model.RuleIrreversible
	}
	// A reviewed read is what a Read-only connection lets through; any read
	// label is enough to run in live mode or under "ask before every change".
	reviewedRead := d.Action == model.ActionRead && d.ReviewedRead
	anyRead := d.Action == model.ActionRead
	has := func(t model.Tag) bool {
		for _, x := range d.Tags {
			if x == t {
				return true
			}
		}
		return false
	}

	// 1. Self-protection (D-027).
	if len(d.Protected) > 0 {
		return Decision{Verdict: Deny, Row: 1, Reason: "Protected: this is CloudBuddy's own setup (" + strings.Join(d.Protected, ", ") + ")."}
	}
	// 2–3. Connection rules that only deny.
	if rule == model.RuleOff && d.Kind == "cloud" {
		return Decision{Verdict: Deny, Row: 2, Reason: d.Provider + " is turned off in Connections."}
	}
	if rule == model.RuleReadOnly && d.Kind == "cloud" && !reviewedRead {
		return Decision{Verdict: Deny, Row: 3, Reason: d.Provider + " is read-only in Connections."}
	}
	// 4. Codex can't keep CloudBuddy's file rules on project code with
	// network (D-038).
	if d.Network == "B" && !ctx.CanWrapCode {
		return Decision{Verdict: Deny, Row: 4, Reason: "This runs project code with network access, which this agent can't do safely inside CloudBuddy. Run it yourself in a terminal if you want it."}
	}
	// 5. Deletes.
	if d.Action == model.ActionDelete {
		reason := "It's a delete."
		if d.HiddenDelete != "" {
			reason = "It looks like a delete (\u201c" + d.HiddenDelete + "\u201d), and CloudBuddy can't see exactly what it does."
		}
		return withLoss(Decision{Verdict: Ask, Row: 5, Reason: reason}, d)
	}
	// 6. Warning tags.
	var tagNames []string
	for _, t := range []model.Tag{model.TagIrreversible, model.TagMoney, model.TagPeople} {
		if has(t) {
			tagNames = append(tagNames, tagLabel(t))
		}
	}
	if len(tagNames) > 0 {
		return withLoss(Decision{Verdict: Ask, Row: 6, Reason: "It's marked " + strings.Join(tagNames, ", ") + "."}, d)
	}
	// 7. Live mode asks before anything but a read (D-025, D-065).
	if has(model.TagLive) && !anyRead {
		return withLoss(Decision{Verdict: Ask, Row: 7, Reason: "It changes something in live mode."}, d)
	}
	// 8. "Ask before every change".
	if rule == model.RuleEvery && d.Kind == "cloud" && !anyRead {
		return withLoss(Decision{Verdict: Ask, Row: 8, Reason: "Your rule for " + d.Provider + " asks before every change."}, d)
	}
	// 9. Everything else runs, as it would in a terminal (D-065).
	return Decision{Verdict: Allow, Row: 9}
}

func tagLabel(t model.Tag) string {
	switch t {
	case model.TagIrreversible:
		return "Can't undo"
	case model.TagMoney:
		return "Moves money"
	case model.TagPeople:
		return "Reaches people"
	case model.TagLive:
		return "Live"
	}
	return string(t)
}

func withLoss(dec Decision, d recognize.Draft) Decision {
	if len(d.Loss) > 0 {
		dec.Loss, dec.LossTitle = d.Loss, d.LossTitle
		if dec.LossTitle == "" {
			dec.LossTitle = "What will happen"
		}
		return dec
	}
	target := d.Resource
	if target == "" {
		target = d.Product
	}
	switch d.Action {
	case model.ActionDelete:
		dec.LossTitle = "What will be lost"
		dec.Loss = []string{"Deletes " + target + " (" + d.Provider + ")", "CloudBuddy can't undo this"}
	case model.ActionSend:
		dec.LossTitle = "What will happen"
		dec.Loss = []string{"Sends to real people through " + d.Provider, "It can't be unsent"}
	default:
		dec.LossTitle = "What will happen"
		dec.Loss = []string{strings.TrimSpace(fmt.Sprintf("%s %s in %s", actionVerb(d.Action), target, d.Provider))}
	}
	if d.Network == "B" {
		dec.Warning = KindBWarning
	}
	return dec
}

func actionVerb(a model.Action) string {
	switch a {
	case model.ActionCreate:
		return "Creates"
	case model.ActionChange:
		return "Changes"
	case model.ActionSend:
		return "Sends"
	case model.ActionRead:
		return "Reads"
	}
	return "Does something to"
}
