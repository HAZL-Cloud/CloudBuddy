import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { clock } from '../format'
import { deployConv, ec2Conv, newsletterConv } from '../mock/data'
import type { Step } from '../types'
import { StepCard, type StepCardProps } from './StepCard'

const news = newsletterConv()
const find = (id: string): Step => [...news.steps, ...ec2Conv().steps, ...deployConv().steps].find((s) => s.id === id)!
const numbers = Object.fromEntries(news.steps.filter((s) => s.kind === 'cloud').map((s, i) => [s.id, i + 1]))

function setup(step: Step, over: Partial<StepCardProps> = {}) {
  const props: StepCardProps = {
    step,
    number: numbers[step.id] ?? 1,
    numbers,
    selected: false,
    current: false,
    future: false,
    enter: false,
    deciding: null,
    onSelect: vi.fn(),
    onDecide: vi.fn(),
    ...over,
  }
  render(<StepCard {...props} />)
  return props
}

describe('StepCard', () => {
  it('shows the action, step number, title, resource, facts and status', () => {
    setup(find('n5'))
    expect(screen.getByText('Create')).toBeInTheDocument()
    expect(screen.getByText('Step 5')).toBeInTheDocument()
    expect(screen.getByText('Created the $8/month plan')).toBeInTheDocument()
    expect(screen.getByText('Prices · Shipyard Weekly')).toBeInTheDocument()
    expect(screen.getByText('$8.00 / month')).toBeInTheDocument()
    expect(screen.getByText('Done · 0.8s')).toBeInTheDocument()
  })

  it('shows the warning tags', () => {
    setup(find('n5'))
    expect(screen.getByText('Live')).toBeInTheDocument()
    expect(screen.getByText("Can't undo")).toBeInTheDocument()
  })

  it('stamps an approval with who and when', () => {
    const step = find('n5')
    setup(step)
    expect(screen.getByText(`Approved by you · ${clock(step.approval!.decidedAt)}`)).toBeInTheDocument()
  })

  it('stamps a denial and shows the step as not run', () => {
    const step = find('e5')
    setup(step)
    expect(screen.getByText(/You said no ·/)).toBeInTheDocument()
    expect(screen.getByText('Not run')).toBeInTheDocument()
  })

  it('says when no adapter matched and which word it guessed from', () => {
    setup(find('n7'))
    expect(screen.getByText(/No Google Cloud adapter\./)).toHaveTextContent('Guessed from the word “update”.')
  })

  it('tells screen readers what the lines bring in', () => {
    setup(find('n7'), {
      carried: [
        { fromStepId: 'n5', what: 'plan ID' },
        { fromStepId: 'n6', what: 'webhook secret' },
      ],
    })
    expect(screen.getByText(/Uses the plan ID from step 5\./).closest('.sr-only')).not.toBeNull()
    expect(screen.getByText(/Uses the webhook secret from step 6\./)).toBeInTheDocument()
  })

  it('shows values found earlier as chips that trace on hover and show the step on click', async () => {
    const onTrace = vi.fn()
    const onShow = vi.fn()
    const props = setup(find('n7'), { chips: [{ from: ['n1', 'n2'], local: false, whats: ['domain'] }], onTrace, onShow })
    const chip = screen.getByRole('button', { name: 'Uses the domain from 1–2. Show step 1.' })
    expect(chip).toHaveTextContent('from 1–2: domain')
    await userEvent.hover(chip)
    expect(onTrace).toHaveBeenLastCalledWith('n7', ['n1', 'n2'])
    await userEvent.unhover(chip)
    expect(onTrace).toHaveBeenLastCalledWith('n7', null)
    await userEvent.click(chip)
    expect(onShow).toHaveBeenCalledWith('n1')
    // A chip click is not a click on the card.
    expect(props.onSelect).not.toHaveBeenCalled()
  })

  it('shows what will happen, the agent’s reason, and the buttons while waiting', () => {
    setup(find('n9'))
    expect(screen.getByText('Waiting for your approval')).toBeInTheDocument()
    expect(screen.getByText('What will happen')).toBeInTheDocument()
    expect(screen.getByText('1,240 people on the Waitlist get this email')).toBeInTheDocument()
    expect(screen.getByText(/You asked me to send the launch email/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'No' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Approve' })).toBeEnabled()
  })

  it('asks the parent to decide, and does not open the details when a button is pressed', async () => {
    const step = find('n9')
    const props = setup(step)
    await userEvent.click(screen.getByRole('button', { name: 'Approve' }))
    expect(props.onDecide).toHaveBeenCalledWith(step, 'approve')
    await userEvent.click(screen.getByRole('button', { name: 'No' }))
    expect(props.onDecide).toHaveBeenCalledWith(step, 'deny')
    expect(props.onSelect).not.toHaveBeenCalled()
  })

  it('disables both buttons while a decision is on its way', () => {
    const step = find('n9')
    setup(step, { deciding: { approvalId: step.approval!.id, decision: 'approve' } })
    expect(screen.getByRole('button', { name: 'Approving…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'No' })).toBeDisabled()
  })

  it('opens the details on click, and on Enter from the keyboard', async () => {
    const step = find('n3')
    const props = setup(step)
    await userEvent.click(screen.getByText('Added 3 email DNS records'))
    expect(props.onSelect).toHaveBeenLastCalledWith('n3')
    screen.getByRole('article').focus()
    await userEvent.keyboard('{Enter}')
    expect(props.onSelect).toHaveBeenLastCalledWith('n3', true)
  })

  it('has no approve buttons on a card that is not waiting', () => {
    setup(find('n5'))
    expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull()
  })

  it('names the step and its provider for screen readers', () => {
    setup(find('n3'))
    expect(screen.getByRole('article', { name: 'Step 3: Added 3 email DNS records (Cloudflare)' })).toBeInTheDocument()
  })
})
