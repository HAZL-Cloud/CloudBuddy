import '@xyflow/react/dist/base.css'
import './styles/tokens.css'
import './styles/base.css'
import './styles/sidebar.css'
import './styles/chat.css'
import './styles/markdown.css'
import './styles/canvas.css'
import './styles/connections.css'
import './styles/dashboards.css'
import './styles/extras.css'
import './styles/workspace.css'
import './styles/responsive.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { App } from './App'
import { ToastProvider } from './components/Toast'
import { makeQueryClient } from './queryClient'
import { applyTheme, storedTheme } from './theme'

const client = makeQueryClient()
// Before the first render, so a dark choice never shows a light frame.
applyTheme(storedTheme())

// The app at /app, with its own styles, so the landing page never loads them.
export default function AppRoot() {
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>
        <App />
      </ToastProvider>
    </QueryClientProvider>
  )
}
