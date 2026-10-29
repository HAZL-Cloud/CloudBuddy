import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { isAppPath } from '../route'
import { Landing } from './Landing'

describe('the landing page', () => {
  it('opens the app at /app from every Open CloudBuddy button, with no sign-in of its own', () => {
    render(<Landing github="" />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('See what your agent does to your cloud.')
    const open = screen.getAllByRole('link', { name: 'Open CloudBuddy' })
    expect(open.length).toBeGreaterThanOrEqual(2)
    for (const link of open) expect(link).toHaveAttribute('href', '/app')
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByLabelText(/password/i)).toBeNull()
  })

  it('shows the GitHub button marked Soon, going nowhere, until the repository has a URL', () => {
    render(<Landing github="" />)
    const buttons = screen.getAllByRole('link', { name: 'GitHub, coming soon' })
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    for (const b of buttons) {
      expect(b).toHaveAttribute('aria-disabled', 'true')
      expect(b).not.toHaveAttribute('href')
    }
    expect(screen.getByText(/git clone <repo-url> cloudbuddy/)).toBeInTheDocument()
  })

  it('links GitHub in a new tab and puts the URL in the clone command once it is set', () => {
    const url = 'https://github.com/example/cloudbuddy'
    render(<Landing github={url} />)
    const buttons = screen.getAllByRole('link', { name: 'GitHub' })
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    for (const b of buttons) {
      expect(b).toHaveAttribute('href', url)
      expect(b).toHaveAttribute('target', '_blank')
      expect(b).toHaveAttribute('rel', 'noopener noreferrer')
    }
    expect(screen.queryByText('Soon')).toBeNull()
    expect(screen.getByText(`git clone ${url} cloudbuddy && cd cloudbuddy`, { exact: false })).toBeInTheDocument()
  })

  it('copies the quick start commands without their comments', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    render(<Landing github="" />)
    const start = screen.getByRole('region', { name: 'Clone, build, run.' })
    await userEvent.click(within(start).getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith('git clone <repo-url> cloudbuddy && cd cloudbuddy\nmake db-up\nmake release\nbin/cloudbuddy')
    expect(await within(start).findByRole('button', { name: 'Copied' })).toBeInTheDocument()
    vi.unstubAllGlobals()
  })

  it('says to copy by hand when the clipboard is not available', async () => {
    vi.stubGlobal('navigator', { ...navigator, clipboard: undefined })
    render(<Landing github="" />)
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }))
    expect(await screen.findByRole('button', { name: 'Select and copy' })).toBeInTheDocument()
    vi.unstubAllGlobals()
  })
})

describe('which page a path opens', () => {
  it('opens the app at /app and below, and the landing page everywhere else', () => {
    expect(isAppPath('/app')).toBe(true)
    expect(isAppPath('/app/')).toBe(true)
    expect(isAppPath('/app/settings')).toBe(true)
    expect(isAppPath('/')).toBe(false)
    expect(isAppPath('/apple')).toBe(false)
    expect(isAppPath('/settings')).toBe(false)
  })
})
