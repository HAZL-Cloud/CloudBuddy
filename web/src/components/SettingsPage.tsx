import { useEffect, useState, type FormEvent } from 'react'
import { ApiError } from '../api'
import { plural } from '../format'
import { useSaveSettings, useSettings } from '../queries'
import type { Agent, AgentName, Project } from '../types'
import { Icon } from './Icon'
import { useToast } from './Toast'

export interface SettingsPageProps {
  agents: Agent[]
  projects: Project[]
  onNewProject: () => void
  onEditProject: (project: Project) => void
}

export function SettingsPage({ agents, projects, onNewProject, onEditProject }: SettingsPageProps) {
  const toast = useToast()
  const settings = useSettings()
  const save = useSaveSettings()
  const [agent, setAgent] = useState<AgentName | null>(null)
  const [folder, setFolder] = useState<string | null>(null)

  // The form starts from the saved settings, and again after every save.
  useEffect(() => {
    if (!settings.data) return
    setAgent(settings.data.agent)
    setFolder(settings.data.folder)
  }, [settings.data])

  const changed = settings.data !== undefined && (agent !== settings.data.agent || folder?.trim() !== settings.data.folder)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!agent || folder === null || !changed || save.isPending) return
    save.mutate({ agent, folder: folder.trim() }, { onSuccess: () => toast('Settings saved. New chats use them.') })
  }

  return (
    <section className="settings-page" aria-labelledby="settings-title">
      <div className="conn-inner">
        <div className="conn-head">
          <div className="conn-head-text">
            <h2 id="settings-title">Settings</h2>
            <p>These apply to new chats. A chat keeps the agent and folders it started with.</p>
          </div>
        </div>

        {settings.isError ? (
          <p className="field-error">Couldn&apos;t load the settings. Check that CloudBuddy is still running.</p>
        ) : (
          <form className="set-form" onSubmit={submit}>
            <fieldset className="conn-sec">
              <legend>
                <h3>Agent</h3>
              </legend>
              <div className="agent-choices">
                {agents.map((a) => (
                  <label key={a.name} className={`agent-choice ${agent === a.name ? 'on' : ''} ${a.available ? '' : 'off'}`}>
                    <input
                      type="radio"
                      name="agent"
                      value={a.name}
                      checked={agent === a.name}
                      disabled={!a.available}
                      onChange={() => setAgent(a.name)}
                    />
                    <span className="agent-choice-text">
                      <b>{a.label}</b>
                      <span className="mono">{a.available ? a.version : 'not found on this computer'}</span>
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>

            <div className="conn-sec">
              <h3>
                <label htmlFor="set-folder">Folder for chats outside a project</label>
              </h3>
              <input
                id="set-folder"
                className="set-input mono"
                value={folder ?? ''}
                placeholder="~/code/my-project"
                autoComplete="off"
                spellCheck={false}
                onChange={(e) => setFolder(e.target.value)}
              />
              <p className="field-help">Use a full path. Chats in a project use the project&apos;s folders instead.</p>
            </div>

            {save.isError && (
              <div className="field-error" role="alert">
                {save.error instanceof ApiError && save.error.status === 400
                  ? `${save.error.code[0]?.toUpperCase() ?? ''}${save.error.code.slice(1)}.`
                  : "Couldn't save the settings. Try again."}
              </div>
            )}
            <div>
              <button type="submit" className="btn primary" disabled={!changed || save.isPending}>
                {save.isPending ? 'Saving…' : 'Save settings'}
              </button>
            </div>
          </form>
        )}

        <div className="conn-sec">
          <h3>
            Projects <span>{plural(projects.length, 'project')}</span>
          </h3>
          <p className="field-help">A project is a name and its folders. Start a chat in a project and the agent has them all.</p>
          {projects.length > 0 && (
            <div className="conn-list">
              {projects.map((p) => (
                <div key={p.id} className="set-project">
                  <div className="set-project-main">
                    <b>{p.name}</b>
                    <span className="set-folders">
                      {p.folders.map((f) => (
                        <span key={f} className="mono">
                          {f}
                        </span>
                      ))}
                    </span>
                  </div>
                  <span className="set-count">{plural(p.chatCount, 'chat')}</span>
                  <button type="button" className="btn" onClick={() => onEditProject(p)}>
                    <Icon name="pencil" small />
                    Edit
                  </button>
                </div>
              ))}
            </div>
          )}
          <div>
            <button type="button" className="btn" onClick={onNewProject}>
              <Icon name="plus" small />
              New project
            </button>
          </div>
        </div>
      </div>
    </section>
  )
}
