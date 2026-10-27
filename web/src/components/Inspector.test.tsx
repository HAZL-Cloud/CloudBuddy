import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { newsletterConv } from '../mock/data'
import { renderWithProviders } from '../test/utils'
import type { Step } from '../types'
import { Inspector } from './Inspector'

const news = newsletterConv()
const find = (id: string): Step => news.steps.find((s) => s.id === id)!
const numbers = Object.fromEntries(news.steps.filter((s) => s.kind === 'cloud').map((s, i) => [s.id, i + 1]))

function open(step: Step, over: Partial<Parameters<typeof Inspector>[0]> = {}) {
  const props = { step, number: numbers[step.id]!, numbers, deciding: null, onDecide: vi.fn(), onClose: vi.fn(), ...over }
  const view = renderWithProviders(<Inspector {...props} />)
  return { ...props, ...view }
}

describe('Inspector', () => {
  it('shows where the step ran and how it was reached', () => {
    open(find('n7'))
    expect(screen.getByText('Google Cloud · Cloud Run · via CLI')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Gave the signup API the plan and secret' })).toBeInTheDocument()
  })

  it('explains why CloudBuddy asked, and how it read an unadapted command', () => {
    open(find('n7'))
    expect(screen.getByText('Why CloudBuddy asked')).toBeInTheDocument()
    expect(screen.getByText(/There's no Google Cloud adapter yet, and this command isn't clearly read-only\./)).toBeInTheDocument()
    expect(screen.getByText(/the word “update” means this is a change/)).toBeInTheDocument()
  })

  it('lists what a step uses, with step numbers', () => {
    open(find('n7'))
    expect(screen.getByText('The plan ID from step 5')).toBeInTheDocument()
  })

  it('labels an MCP call as a tool call', () => {
    open(find('n9'))
    expect(screen.getByText('Tool call')).toBeInTheDocument()
    expect(screen.getByText('MCP tool')).toBeInTheDocument()
  })

  it('has the approve buttons while waiting, and not after', async () => {
    const { onDecide } = open(find('n9'))
    await userEvent.click(screen.getByRole('button', { name: 'Approve' }))
    expect(onDecide).toHaveBeenCalledWith(find('n9'), 'approve')
    expect(screen.getByText('Nothing yet. This step has not run.')).toBeInTheDocument()
  })

  it('shows the decision once made, and the masked-output note', () => {
    open(find('n6'))
    expect(screen.getByText(/Approved by you at/)).toBeInTheDocument()
    expect(screen.getByText('What was at stake')).toBeInTheDocument()
    expect(screen.getByText('CloudBuddy hid the webhook secret before saving this output.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull()
  })

  it('closes from the close button', async () => {
    const { onClose } = open(find('n1'))
    await userEvent.click(screen.getByRole('button', { name: 'Close details' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('shows command and output from tools as plain text, never as markup', () => {
    const hostile = '<img src=x onerror="alert(1)"><script>alert(2)</script>'
    const step: Step = { ...find('n1'), command: hostile, output: hostile, title: '<b>bold?</b>', why: hostile }
    const { container } = open(step)
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('b')).toBeNull()
    expect(screen.getByText('<b>bold?</b>')).toBeInTheDocument()
    expect(screen.getAllByText(hostile).length).toBeGreaterThanOrEqual(2)
  })
})
