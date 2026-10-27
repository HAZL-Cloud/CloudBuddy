import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'

const ToastContext = createContext<(message: string) => void>(() => {})

export const useToast = () => useContext(ToastContext)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [message, setMessage] = useState('')
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const show = useCallback((text: string) => {
    setMessage(text)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setMessage(''), 2600)
  }, [])
  useEffect(() => () => clearTimeout(timer.current), [])
  return (
    <ToastContext.Provider value={show}>
      {children}
      {/* The live region stays in the page so screen readers announce the text when it changes. */}
      <div className="toast" role="status" hidden={!message}>
        {message}
      </div>
    </ToastContext.Provider>
  )
}
