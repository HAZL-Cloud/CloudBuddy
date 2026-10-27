import { useState, type FormEvent } from 'react'
import { ApiError } from '../api'
import { useDeleteProject, useSaveProject } from '../queries'
import type { Project } from '../types'
import { Dialog } from './Dialog'
import { FolderField, folderName, parentFolder, usePickFolder } from './FolderField'
import { Icon } from './Icon'
import { useToast } from './Toast'

export const MAX_FOLDERS = 12

export interface ProjectDialogProps {
  /** null makes a new project. */
  project: Project | null
  onClose: () => void
  onSaved: (project: Project) => void
  onDeleted: (id: string) => void
}

// The server's 400 messages are written for people ("folder not found: …"); anything else gets a general line.
const problem = (err: unknown) =>
  err instanceof ApiError && err.status === 400 ? `${err.code[0]?.toUpperCase() ?? ''}${err.code.slice(1)}.` : "Couldn't save. Try again."

export function ProjectDialog({ project, onClose, onSaved, onDeleted }: ProjectDialogProps) {
  const toast = useToast()
  const save = useSaveProject()
  const remove = useDeleteProject()
  const [name, setName] = useState(project?.name ?? '')
  const [folders, setFolders] = useState<string[]>(project?.folders.length ? project.folders : [''])
  const [confirmDelete, setConfirmDelete] = useState(false)
  const picker = usePickFolder()

  const filled = folders.map((f) => f.trim()).filter(Boolean)
  const ready = name.trim() !== '' && filled.length > 0 && !save.isPending
  const setFolder = (i: number, value: string) => setFolders((list) => list.map((f, j) => (j === i ? value : f)))
  // Other folders of a project usually sit next to the first one, so the chooser opens there.
  const nextTo = filled[0] ? parentFolder(filled[0]) : undefined
  // "Add a folder" goes straight to the chooser. Without one, it adds an empty box to type in.
  const addFolder = async () => {
    const res = await picker.choose(nextTo)
    if (res.path) {
      const path = res.path
      setFolders((list) => (list.some((f) => f.trim() === path) ? list : [...list.filter((f) => f.trim() !== ''), path]))
    } else if (res.unavailable) {
      setFolders((list) => [...list, ''])
    }
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!ready) return
    save.mutate(
      { id: project?.id ?? null, input: { name: name.trim(), folders: filled } },
      {
        onSuccess: (saved) => {
          toast(project ? 'Project saved. New chats in it use these folders.' : `Project “${saved.name}” created.`)
          onSaved(saved)
        },
      },
    )
  }
  const doDelete = () => {
    if (!project) return
    remove.mutate(project.id, {
      onSuccess: () => {
        toast(`Deleted “${project.name}”. Its chats are still in All chats.`)
        onDeleted(project.id)
      },
      onError: () => toast("Couldn't delete the project. Try again."),
    })
  }

  return (
    <Dialog title={project ? 'Edit project' : 'New project'} onClose={onClose}>
      <form onSubmit={submit}>
        <div className="field">
          <label htmlFor="pj-name">Name</label>
          <input id="pj-name" value={name} maxLength={80} placeholder="Shop website" autoComplete="off" onChange={(e) => setName(e.target.value)} />
        </div>
        <fieldset className="field folders">
          <legend>Folders</legend>
          <p className="field-help">The agent starts in the first folder and can read and change files in all of them.</p>
          {folders.map((f, i) => (
            <div className="folder-row" key={i}>
              <FolderField
                label={i === 0 ? 'First folder (the agent starts here)' : `Folder ${i + 1}`}
                value={f}
                start={i === 0 ? undefined : nextTo}
                onChange={(value) => setFolder(i, value)}
                // A new project is named after its first folder until you name it.
                onPicked={(path) => i === 0 && !name.trim() && setName(folderName(path))}
              />
              {folders.length > 1 && (
                <button
                  type="button"
                  className="icon-btn"
                  aria-label={`Remove folder ${i + 1}`}
                  onClick={() => setFolders((list) => list.filter((_, j) => j !== i))}
                >
                  <Icon name="x" small />
                </button>
              )}
            </div>
          ))}
          {folders.length < MAX_FOLDERS && (
            <button type="button" className="btn link add-folder" disabled={picker.busy} onClick={() => void addFolder()}>
              <Icon name="plus" small />
              {picker.busy ? 'Choosing…' : 'Add a folder'}
            </button>
          )}
        </fieldset>
        {project && project.chatCount > 0 && (
          <p className="field-help">Chats already in this project keep the folders they started with.</p>
        )}
        {save.isError && (
          <div className="field-error" role="alert">
            {problem(save.error)}
          </div>
        )}
        <div className="dialog-actions">
          {project &&
            (confirmDelete ? (
              <button type="button" className="btn danger push-left" disabled={remove.isPending} onClick={doDelete}>
                {remove.isPending ? 'Deleting…' : 'Delete it. Chats stay.'}
              </button>
            ) : (
              <button type="button" className="btn link push-left" onClick={() => setConfirmDelete(true)}>
                <Icon name="trash" small />
                Delete project
              </button>
            ))}
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn primary" disabled={!ready}>
            {save.isPending ? 'Saving…' : project ? 'Save' : 'Create project'}
          </button>
        </div>
      </form>
    </Dialog>
  )
}
