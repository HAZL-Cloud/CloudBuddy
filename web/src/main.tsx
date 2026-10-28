import '@fontsource/bricolage-grotesque/latin-600.css'
import '@fontsource/bricolage-grotesque/latin-700.css'
import '@fontsource/ibm-plex-sans/latin-400.css'
import '@fontsource/ibm-plex-sans/latin-500.css'
import '@fontsource/ibm-plex-sans/latin-600.css'
import '@fontsource/ibm-plex-mono/latin-400.css'
import '@fontsource/ibm-plex-mono/latin-500.css'
import '@fontsource/ibm-plex-mono/latin-600.css'

import { StrictMode, Suspense, lazy } from 'react'
import { createRoot } from 'react-dom/client'
import { isAppPath } from './route'

// Each page loads only its own code and styles: the landing page at /, the app at /app.
const Page = lazy(() =>
  isAppPath(window.location.pathname) ? import('./AppRoot') : import('./landing/Landing').then((m) => ({ default: m.Landing })),
)

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <Suspense fallback={null}>
      <Page />
    </Suspense>
  </StrictMode>,
)
