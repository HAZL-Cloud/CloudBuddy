import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'

export interface DialogProps {
  title: string
  onClose: () => void
  children: ReactNode
}

const FOCUSABLE = 'select, input, textarea, button:not(:disabled)'

/** A modal: focus moves in and back out, Escape closes, and Tab stays inside. */
export function Dialog({ title, onClose, children }: DialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const opener = useRef<Element | null>(document.activeElement)

  useEffect(() => {
    dialogRef.current?.querySelector<HTMLElement>('input, select, textarea')?.focus()
    const from = opener.current
    return () => {
      if (from instanceof HTMLElement) from.focus()
    }
  }, [])

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
      return
    }
    if (e.key !== 'Tab') return
    const items = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? [])
    const first = items[0]
    const last = items[items.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last?.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first?.focus()
    }
  }

  return (
    <div className="dialog-scrim" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title" ref={dialogRef} onKeyDown={onKeyDown}>
        <h2 id="dialog-title">{title}</h2>
        {children}
      </div>
    </div>
  )
}
