// Package model holds the types shared by the server, the gate and the
// recognizer. JSON names match docs/api.md.
package model

import "time"

type Action string

const (
	ActionRead    Action = "read"
	ActionCreate  Action = "create"
	ActionChange  Action = "change"
	ActionSend    Action = "send"
	ActionDelete  Action = "delete"
	ActionUnknown Action = "unknown"
)

// Caution orders actions from least to most cautious, so recognizer layers can
// be merged by taking the maximum (D-026).
func (a Action) Caution() int {
	switch a {
	case ActionRead:
		return 0
	case ActionCreate:
		return 1
	case ActionChange:
		return 2
	case ActionSend:
		return 3
	case ActionUnknown:
		return 4
	case ActionDelete:
		return 5
	}
	return 4
}

type Tag string

const (
	TagLive         Tag = "live"
	TagIrreversible Tag = "irreversible"
	TagMoney        Tag = "money"
	TagPeople       Tag = "people"
)

type StepStatus string

const (
	StatusRunning        StepStatus = "running"
	StatusDone           StepStatus = "done"
	StatusFailed         StepStatus = "failed"
	StatusWaiting        StepStatus = "waiting"
	StatusDenied         StepStatus = "denied"
	StatusOutcomeUnknown StepStatus = "outcome_unknown"
)

type ConvStatus string

const (
	ConvIdle        ConvStatus = "idle"
	ConvRunning     ConvStatus = "running"
	ConvWaiting     ConvStatus = "waiting"
	ConvDone        ConvStatus = "done"
	ConvFailed      ConvStatus = "failed"
	ConvInterrupted ConvStatus = "interrupted"
)

type Approval struct {
	ID          string     `json:"id"`
	State       string     `json:"state"` // pending | approved | denied | expired
	Reason      string     `json:"reason"`
	LossTitle   string     `json:"lossTitle,omitempty"`
	Loss        []string   `json:"loss,omitempty"`
	Warning     string     `json:"warning,omitempty"`
	RequestedAt time.Time  `json:"requestedAt"`
	DecidedAt   *time.Time `json:"decidedAt,omitempty"`
}

type Use struct {
	FromStepID string `json:"fromStepId"`
	What       string `json:"what"`
}

type Step struct {
	ID              string      `json:"id"`
	Seq             int         `json:"seq"`
	Kind            string      `json:"kind"` // cloud | local
	Provider        string      `json:"provider"`
	Product         string      `json:"product"`
	Action          Action      `json:"action"`
	Tags            []Tag       `json:"tags"`
	Title           string      `json:"title"`
	TitleSource     string      `json:"titleSource"`
	Resource        string      `json:"resource"`
	Facts           [][2]string `json:"facts"`
	Via             string      `json:"via"`
	RecognizedBy    []string    `json:"recognizedBy"`
	Generic         bool        `json:"generic"`
	GuessWord       string      `json:"guessWord,omitempty"`
	Command         string      `json:"command"`
	API             string      `json:"api"`
	Why             string      `json:"why"`
	Status          StepStatus  `json:"status"`
	StartedAt       time.Time   `json:"startedAt"`
	EndedAt         *time.Time  `json:"endedAt,omitempty"`
	DurationMs      *int64      `json:"durationMs,omitempty"`
	Output          string      `json:"output"`
	OutputTruncated bool        `json:"outputTruncated"`
	MaskedNote      string      `json:"maskedNote,omitempty"`
	Uses            []Use       `json:"uses"`
	Approval        *Approval   `json:"approval,omitempty"`
}

type ChatItem struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"` // user | agent | steps | approval | notice
	Text    string    `json:"text,omitempty"`
	StepIDs []string  `json:"stepIds,omitempty"`
	StepID  string    `json:"stepId,omitempty"`
	Level   string    `json:"level,omitempty"`
	At      time.Time `json:"at"`
}

type RunResult struct {
	Title string    `json:"title"`
	Line  string    `json:"line"`
	At    time.Time `json:"at"`
}

type ConversationSummary struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Agent        string     `json:"agent"`
	Model        string     `json:"model"`
	Workdir      string     `json:"workdir"`
	Status       ConvStatus `json:"status"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	StepCount    int        `json:"stepCount"`
	WaitingCount int        `json:"waitingCount"`
}

type ConversationDetail struct {
	ConversationSummary
	Subtitle string     `json:"subtitle"`
	Chat     []ChatItem `json:"chat"`
	Steps    []Step     `json:"steps"`
	Result   *RunResult `json:"result,omitempty"`
}

type Connection struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // cli | mcp
	Name        string `json:"name"`
	Mark        string `json:"mark"`
	How         string `json:"how"`
	Account     string `json:"account"`
	Detail      string `json:"detail"`
	Adapter     string `json:"adapter"`
	AdapterNote string `json:"adapterNote,omitempty"`
	Rule        string `json:"rule"`
	Supported   bool   `json:"supported"`
}

// Connection rules (section 4.5).
const (
	RuleIrreversible = "irreversible"
	RuleLive         = "live"
	RuleEvery        = "every"
	RuleReadOnly     = "readonly"
	RuleOff          = "off"
)
