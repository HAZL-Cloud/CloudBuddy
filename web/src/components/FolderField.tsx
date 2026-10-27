import { useCallback, useState } from 'react'
import { api, ApiError } from '../api'
import { Icon } from './Icon'
import { useToast } from './Toast'

export interface Picked {
  path?: string
  /** This computer has no folder chooser, so the path has to be typed. */
  unavailable?: boolean
}

/** Opens the computer's own folder chooser through the server (D-058). Cancel gives an empty result. */
export function usePickFolder() {
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const choose = useCallback(
    async (start?: string): Promise<Picked> => {
      setBusy(true)
      try {
        const res = await api.pickFolder(start)
        return res.path ? { path: res.path } : {}
      } catch (err) {
        if (err instanceof ApiError && err.status === 501) {
          toast('This computer has no folder chooser. Type the full path instead.')
          return { unavailable: true }
        }
        toast(
          err instanceof ApiError && err.status === 409
            ? 'A folder chooser is already open. It may be behind this window.'
            : "Couldn't open the folder chooser. Type the full path instead.",
        )
        return {}
      } finally {
        setBusy(false)
      }
    },
    [toast],
  )
  return { choose, busy }
}

/** The last part of a path: "~/code/shop-web" gives "shop-web". */
export const folderName = (path: string) => path.replace(/\/+$/, '').split('/').pop() ?? ''

/** The folder a path is in: "~/code/shop-web" gives "~/code". */
export const parentFolder = (path: string) => path.replace(/\/+$/, '').replace(/\/[^/]*$/, '')

export interface FolderFieldProps {
  id?: string
  label?: string
  value: string
  onChange: (path: string) => void
  /** Where the chooser opens when the box is empty. */
  start?: string
  placeholder?: string
  onPicked?: (path: string) => void
}

/** A folder path you can type, or pick with the computer's folder chooser. */
export function FolderField({ id, label, value, onChange, start, placeholder, onPicked }: FolderFieldProps) {
  const { choose, busy } = usePickFolder()
  const pick = async () => {
    const res = await choose(value.trim() || start)
    if (res.path) {
      onChange(res.path)
      onPicked?.(res.path)
    }
  }
  return (
    <div className="folder-field">
      <input
        id={id}
        className="mono"
        aria-label={label}
        value={value}
        placeholder={placeholder ?? 'Choose a folder, or type its full path'}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => onChange(e.target.value)}
      />
      <button type="button" className="btn" disabled={busy} aria-label={label ? `Choose ${label.toLowerCase()}` : 'Choose a folder'} onClick={() => void pick()}>
        <Icon name="folder" small />
        {busy ? 'Choosing…' : 'Choose…'}
      </button>
      {busy && (
        <span className="folder-wait" role="status">
          Pick a folder in the window that opened. It may be behind this one.
        </span>
      )}
    </div>
  )
}
