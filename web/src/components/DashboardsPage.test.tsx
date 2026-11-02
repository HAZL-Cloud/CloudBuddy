import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from '../App'
import { renderWithProviders } from '../test/utils'

const sidebar = () => within(screen.getByRole('complementary', { name: 'Navigation' }))

async function openDashboards() {
  renderWithProviders(<App />)
  const nav = within(await screen.findByRole('complementary', { name: 'Navigation' }))
  await userEvent.click(nav.getByRole('button', { name: /Dashboards/ }))
  return screen.getByRole('region', { name: 'Dashboards' })
}

describe('the Dashboards page', () => {
  it('shows in the sidebar with the number of commands waiting for an OK', async () => {
    renderWithProviders(<App />)
    await screen.findByRole('complementary', { name: 'Navigation' })
    expect(await sidebar().findByLabelText('1 waiting for your OK')).toBeInTheDocument()
  })

  it('lists dashboards with their headline numbers', async () => {
    const page = within(await openDashboards())
    const card = await page.findByRole('button', { name: /Users/ })
    expect(within(card).getByText('1,204')).toBeInTheDocument()
    expect(within(card).getByText('every 15 min')).toBeInTheDocument()
    expect(within(card).getByText('1 command waiting for your OK')).toBeInTheDocument()
  })

  it('opens a dashboard: tiles, their states and the commands behind them', async () => {
    const page = within(await openDashboards())
    await userEvent.click(await page.findByRole('button', { name: /Users/ }))
    expect(await page.findByRole('heading', { name: 'Users', level: 2 })).toBeInTheDocument()

    const users = page.getByRole('article', { name: 'Users' })
    expect(within(users).getByText('1,204')).toBeInTheDocument()
    expect(within(users).getByText('+42')).toBeInTheDocument()
    expect(within(users).getByText('in 24 hours')).toBeInTheDocument()

    const api = page.getByRole('article', { name: 'API' })
    expect(within(api).getByText('OK')).toBeInTheDocument()

    const mrr = page.getByRole('article', { name: 'Monthly revenue' })
    expect(within(mrr).getByText('Waiting for your OK')).toBeInTheDocument()

    const plans = page.getByRole('article', { name: 'Users by plan' })
    expect(within(plans).getByRole('columnheader', { name: 'Share' })).toBeInTheDocument()
    expect(within(plans).getByText('92.8%')).toBeInTheDocument()

    const signups = page.getByRole('article', { name: 'Signups per day' })
    expect(within(signups).getByText('Nov 1')).toBeInTheDocument()
    await userEvent.click(within(signups).getByRole('button', { name: 'Table' }))
    expect(within(signups).getByRole('table')).toBeInTheDocument()

    expect(page.getAllByText('Runs by itself').length).toBe(5)
  })

  it('asks before a script runs on the timer, and runs it once allowed', async () => {
    const page = within(await openDashboards())
    await userEvent.click(await page.findByRole('button', { name: /Users/ }))
    const ask = within(await page.findByRole('region', { name: 'Commands waiting for your OK' }))
    expect(ask.getByText('node scripts/mrr.js')).toBeInTheDocument()
    expect(ask.getByText(/can’t see what it does inside/)).toBeInTheDocument()

    await userEvent.click(ask.getByRole('button', { name: 'Allow on the timer' }))
    await waitFor(() => expect(page.queryByRole('region', { name: 'Commands waiting for your OK' })).not.toBeInTheDocument())
    const mrr = page.getByRole('article', { name: 'Monthly revenue' })
    expect(await within(mrr).findByText('$3,480')).toBeInTheDocument()
    expect(page.getByText('Allowed by you')).toBeInTheDocument()
  })

  it('pauses, changes the interval, and deletes only after asking', async () => {
    const page = within(await openDashboards())
    await userEvent.click(await page.findByRole('button', { name: /Users/ }))
    await page.findByRole('heading', { name: 'Users', level: 2 })

    await userEvent.click(page.getByRole('button', { name: 'Pause' }))
    expect(await page.findByRole('button', { name: 'Resume' })).toBeInTheDocument()
    expect(page.getByText(/^Paused/)).toBeInTheDocument()

    await userEvent.selectOptions(page.getByLabelText('How often'), '1h')
    await userEvent.click(page.getByRole('button', { name: 'Resume' }))
    expect(await page.findByText(/Refreshes every hour/)).toBeInTheDocument()

    await userEvent.click(page.getByRole('button', { name: 'Delete this dashboard' }))
    const dialog = within(screen.getByRole('dialog', { name: /Delete “Users”/ }))
    await userEvent.click(dialog.getByRole('button', { name: 'Delete dashboard' }))
    expect(await page.findByText('No dashboards yet')).toBeInTheDocument()
  })

  it('goes back to the chat that built it', async () => {
    const page = within(await openDashboards())
    await userEvent.click(await page.findByRole('button', { name: /Users/ }))
    await userEvent.click(await page.findByRole('button', { name: 'Change in chat' }))
    expect(await screen.findByRole('heading', { name: /Deploy shop site/ })).toBeInTheDocument()
  })
})
