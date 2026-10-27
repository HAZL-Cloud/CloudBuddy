// Package policy decides allow, ask or deny for a recognized call. The rows
// mirror plan_and_track.md section 4.5 and are evaluated top to bottom; the
// first match wins.
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

// KindBWarning is shown on every approval for project code with network (D-029).
const KindBWarning = "CloudBuddy can't see what this does inside. Anything it changes won't show on the canvas."

func Decide(d recognize.Draft, ctx Context) Decision {
	rule := ctx.Rule
	if rule == "" {
		rule = model.RuleIrreversible
	}
	isRead := d.Action == model.ActionRead && d.ReviewedRead
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
	// 2. Background work with network (D-029).
	if d.Background && d.Network != "" {
		return Decision{Verdict: Deny, Row: 2, Reason: "It would keep running in the background, outside CloudBuddy's view."}
	}
	// 3. Local work inside the no-network sandbox.
	if d.Network == "" && !d.OutsideWorkdirWrite {
		return Decision{Verdict: Allow, Row: 3}
	}
	// A file change outside the project folder is not network work, but it
	// isn't covered by the sandbox either.
	if d.Network == "" && d.OutsideWorkdirWrite {
		return Decision{Verdict: Ask, Row: 3, Reason: "It changes a file outside the project folder.",
			LossTitle: "What will happen", Loss: []string{"Changes " + d.Resource}}
	}
	// 4–5. Connection rules that only deny.
	if rule == model.RuleOff {
		return Decision{Verdict: Deny, Row: 4, Reason: d.Provider + " is turned off in Connections."}
	}
	if rule == model.RuleReadOnly && !isRead {
		return Decision{Verdict: Deny, Row: 5, Reason: d.Provider + " is read-only in Connections."}
	}
	// Codex can't keep file rules on approved project code (D-038).
	if d.Network == "B" && !ctx.CanWrapCode {
		return Decision{Verdict: Deny, Row: 9, Reason: "This runs project code with network access, which this agent can't do safely inside CloudBuddy. Run it yourself in a terminal if you want it."}
	}
	// 6. Deletes.
	if d.Action == model.ActionDelete {
		return withLoss(Decision{Verdict: Ask, Row: 6, Reason: "It's a delete."}, d)
	}
	// 7. Warning tags.
	var tagNames []string
	for _, t := range []model.Tag{model.TagIrreversible, model.TagMoney, model.TagPeople} {
		if has(t) {
			tagNames = append(tagNames, tagLabel(t))
		}
	}
	if len(tagNames) > 0 {
		return withLoss(Decision{Verdict: Ask, Row: 7, Reason: "It's marked " + strings.Join(tagNames, ", ") + "."}, d)
	}
	// 8. Live mode always asks (D-025).
	if has(model.TagLive) {
		return withLoss(Decision{Verdict: Ask, Row: 8, Reason: "It runs in live mode."}, d)
	}
	// 9. Project or third-party code with network.
	if d.Network == "B" {
		return Decision{Verdict: Ask, Row: 9, Reason: "It runs code with network access.", Warning: KindBWarning}
	}
	// 10. Unresolved identity on a non-read.
	if !isRead && !d.Identity.Resolved {
		return withLoss(Decision{Verdict: Ask, Row: 10, Reason: "CloudBuddy can't tell which account this uses (" + d.Identity.Label + ")."}, d)
	}
	// 11. "Ask before every change".
	if rule == model.RuleEvery && !isRead {
		return withLoss(Decision{Verdict: Ask, Row: 11, Reason: "Your rule for " + d.Provider + " asks before every change."}, d)
	}
	// Files from outside the project folder in a command with network: they
	// could be sent anywhere (Phase 2 review).
	if len(d.ExternalFiles) > 0 {
		return withLoss(Decision{Verdict: Ask, Row: 12, Reason: "It uses files outside the project folder: " + strings.Join(d.ExternalFiles, ", ") + "."}, d)
	}
	// 12. Unknown, or only the generic layer recognized it and it isn't a reviewed read.
	// Only reviewed rules run without asking (D-049): reviewed reads, and
	// creates/changes a person checked are reversible with no money or people.
	if d.Action == model.ActionUnknown || d.Opaque || (!isRead && !d.ReviewedChange) {
		why := "CloudBuddy can't tell what this does."
		switch {
		case d.Opaque:
			why = "Parts of this command can't be checked (shell tricks, uploads or variables)."
		case onlyGeneric(d) && d.Action != model.ActionUnknown:
			why = fmt.Sprintf("No %s adapter, and it isn't clearly read-only.", d.Provider)
		case d.Action != model.ActionUnknown:
			why = "No reviewed rule says this change is safe to run on its own."
		}
		return withLoss(Decision{Verdict: Ask, Row: 12, Reason: why}, d)
	}
	// 13. Everything else.
	return Decision{Verdict: Allow, Row: 13}
}

// onlyGeneric reports whether no adapter recognized the call.
func onlyGeneric(d recognize.Draft) bool {
	for _, by := range d.RecognizedBy {
		if by == "builtin" || by == "generated" {
			return false
		}
	}
	return true
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
