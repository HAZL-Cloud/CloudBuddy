import '@fontsource/bricolage-grotesque/latin-600.css'
import '@fontsource/bricolage-grotesque/latin-700.css'
import '@fontsource/ibm-plex-sans/latin-400.css'
import '@fontsource/ibm-plex-sans/latin-500.css'
import '@fontsource/ibm-plex-sans/latin-600.css'
import '@fontsource/ibm-plex-mono/latin-400.css'
import '@fontsource/ibm-plex-mono/latin-500.css'
import '@fontsource/ibm-plex-mono/latin-600.css'
import '@xyflow/react/dist/base.css'
import './styles/tokens.css'
import './styles/base.css'
import './styles/sidebar.css'
import './styles/chat.css'
import './styles/markdown.css'
import './styles/canvas.css'
import './styles/connections.css'
import './styles/extras.css'
import './styles/workspace.css'
import './styles/responsive.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { ToastProvider } from './components/Toast'
import { makeQueryClient } from './queryClient'

const client = makeQueryClient()

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <ToastProvider>
        <App />
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
)
