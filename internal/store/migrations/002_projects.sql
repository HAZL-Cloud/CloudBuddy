-- Projects and settings (D-054, D-055).

-- A project is a name and its folders. The first folder is where the agent
-- starts; the others are project folders too.
CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    folders    TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A chat keeps the folders it started with. Editing the project later
-- changes new chats only.
ALTER TABLE conversations ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE conversations ADD COLUMN extra_folders TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX conversations_project ON conversations(project_id);

-- The agent and the folder for chats outside a project.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Start from what the latest chat used, so New chat keeps working after the upgrade.
INSERT INTO settings(key, value)
    SELECT 'agent', agent FROM conversations WHERE agent IN ('claude', 'codex') ORDER BY updated_at DESC LIMIT 1;
INSERT INTO settings(key, value)
    SELECT 'folder', workdir FROM conversations ORDER BY updated_at DESC LIMIT 1;
