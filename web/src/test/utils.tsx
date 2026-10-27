import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { ToastProvider } from '../components/Toast'

export function renderWithProviders(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return { client, ...render(<QueryClientProvider client={client}>{<ToastProvider>{ui}</ToastProvider>}</QueryClientProvider>) }
}
