package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/store"
)

// MaxFolders is how many folders one project can have.
const MaxFolders = 12

// ErrNoFolder means a chat outside a project has no folder to work in.
var ErrNoFolder = errors.New("choose a folder in Settings, or start the chat in a project")

// StartInput is a new chat. Agent and Workdir are optional: the settings
// fill them in (D-055). A project brings its own folders (D-054).
type StartInput struct {
	Agent, Workdir, ProjectID, Text string
}

func (e *Engine) knownAgent(name string) bool {
	return name == "claude" || name == "codex" || (name == "fake" && e.fake != nil)
}

func (e *Engine) StartConversation(ctx context.Context, in StartInput) (string, error) {
	if strings.TrimSpace(in.Text) == "" {
		return "", fmt.Errorf("the message is empty")
	}
	set, err := e.Settings(ctx)
	if err != nil {
		return "", err
	}
	agentName := in.Agent
	if agentName == "" {
		agentName = set.Agent
	}
	if !e.knownAgent(agentName) {
		return "", fmt.Errorf("unknown agent %q", agentName)
	}
	var folders []string
	switch {
	case in.ProjectID != "" && in.Workdir != "":
		return "", fmt.Errorf("pick a project or a folder, not both")
	case in.ProjectID != "":
		p, err := e.st.GetProject(ctx, in.ProjectID)
		if errors.Is(err, store.ErrNotFound) {
			return "", fmt.Errorf("that project no longer exists")
		}
		if err != nil {
			return "", err
		}
		folders = p.Folders
	case in.Workdir != "":
		folders = []string{in.Workdir}
	case set.Folder != "":
		folders = []string{set.Folder}
	default:
		return "", ErrNoFolder
	}
	// Checked again here: a saved folder may have moved since.
	folders, err = e.validFolders(folders)
	if err != nil {
		return "", err
	}
	id := store.NewID("cv")
	if err := e.st.CreateConversation(ctx, store.Conversation{ID: id, Title: titleFrom(in.Text), Agent: agentName,
		Workdir: folders[0], ExtraFolders: folders[1:], ProjectID: in.ProjectID, Status: model.ConvIdle}); err != nil {
		return "", err
	}
	e.bus.Publish(bus.Event{Type: "conversations"})
	e.bus.Publish(bus.Event{Type: "projects"}) // chat counts
	return id, e.SendMessage(ctx, id, in.Text)
}

// validFolders resolves each folder, drops repeats, and keeps the order: the
// first one is where the agent starts.
func (e *Engine) validFolders(dirs []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		real, err := e.validWorkdir(d)
		if err != nil {
			return nil, err
		}
		if !seen[real] {
			seen[real] = true
			out = append(out, real)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("add at least one folder")
	}
	if len(out) > MaxFolders {
		return nil, fmt.Errorf("a project can have up to %d folders", MaxFolders)
	}
	return out, nil
}

// existingDirs keeps the folders that are still there. A project folder that
// was deleted after the chat started is left out of later runs.
func existingDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// ---------- settings ----------

// Settings are the saved settings, with Claude Code as the agent until one is
// chosen.
func (e *Engine) Settings(ctx context.Context) (model.Settings, error) {
	set, err := e.st.Settings(ctx)
	if set.Agent == "" {
		set.Agent = "claude"
	}
	return set, err
}

func (e *Engine) SaveSettings(ctx context.Context, set model.Settings) (model.Settings, error) {
	if !e.knownAgent(set.Agent) {
		return set, fmt.Errorf("unknown agent %q", set.Agent)
	}
	if strings.TrimSpace(set.Folder) == "" {
		set.Folder = ""
	} else {
		real, err := e.validWorkdir(strings.TrimSpace(set.Folder))
		if err != nil {
			return set, err
		}
		set.Folder = real
	}
	if err := e.st.SaveSettings(ctx, set); err != nil {
		return set, err
	}
	e.bus.Publish(bus.Event{Type: "settings"})
	return set, nil
}

// ---------- projects ----------

func (e *Engine) Projects(ctx context.Context) ([]model.Project, error) { return e.st.Projects(ctx) }

// SaveProject creates a project when p.ID is empty, otherwise updates it.
func (e *Engine) SaveProject(ctx context.Context, p model.Project) (model.Project, error) {
	p.Name = strings.Join(strings.Fields(p.Name), " ")
	if p.Name == "" {
		return p, fmt.Errorf("give the project a name")
	}
	if utf8.RuneCountInString(p.Name) > 80 {
		return p, fmt.Errorf("use a name of 80 characters or fewer")
	}
	folders, err := e.validFolders(p.Folders)
	if err != nil {
		return p, err
	}
	p.Folders = folders
	if p.ID == "" {
		p.ID = store.NewID("pj")
	} else if _, err := e.st.GetProject(ctx, p.ID); err != nil {
		return p, err
	}
	if err := e.st.SaveProject(ctx, p); err != nil {
		return p, err
	}
	e.bus.Publish(bus.Event{Type: "projects"})
	return e.st.GetProject(ctx, p.ID)
}

// DeleteProject removes the project. Its chats stay, outside any project, and
// keep the folders they started with.
func (e *Engine) DeleteProject(ctx context.Context, id string) error {
	if err := e.st.DeleteProject(ctx, id); err != nil {
		return err
	}
	e.bus.Publish(bus.Event{Type: "projects"})
	e.bus.Publish(bus.Event{Type: "conversations"})
	return nil
}
