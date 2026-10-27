import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { deployConv, ec2Conv, newsletterConv } from '../mock/data'
import type { Step } from '../types'
import { ApprovalBlock } from './ApprovalBlock'

const find = (id: string): Step => [...newsletterConv().steps, ...ec2Conv().steps, ...deployConv().steps].find((s) => s.id === id)!

describe('ApprovalBlock', () => {
  it('asks for approval with the command, the reason, and a way to review on the canvas', async () => {
    const step = find('n9')
    const onDecide = vi.fn()
    const onShowOnCanvas = vi.fn()
    render(<ApprovalBlock step={step} deciding={null} onDecide={onDecide} onShowOnCanvas={onShowOnCanvas} />)
    expect(screen.getByText('Needs your approval')).toBeInTheDocument()
    expect(screen.getByText("It reaches real people, and an email can't be unsent.")).toBeInTheDocument()
    expect(screen.getByText('send_broadcast({ "id": "bc_7e2a91f4" })')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Review on canvas' }))
    expect(onShowOnCanvas).toHaveBeenCalledWith('n9')
    await userEvent.click(screen.getByRole('button', { name: 'Approve' }))
    expect(onDecide).toHaveBeenCalledWith(step, 'approve')
    await userEvent.click(screen.getByRole('button', { name: 'No' }))
    expect(onDecide).toHaveBeenLastCalledWith(step, 'deny')
  })

  it('shows an approved step quietly, with the time', () => {
    render(<ApprovalBlock step={find('d8')} deciding={null} onDecide={vi.fn()} />)
    expect(screen.getByText('Approval')).toBeInTheDocument()
    expect(screen.getByText(/You approved this at/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull()
  })

  it('says nothing was deleted when you said no to a delete', () => {
    render(<ApprovalBlock step={find('e5')} deciding={null} onDecide={vi.fn()} />)
    expect(screen.getByText(/You said no at .* Nothing was deleted\./)).toBeInTheDocument()
  })

  it('says an expired approval did not run', () => {
    const step = find('n9')
    render(<ApprovalBlock step={{ ...step, status: 'failed', approval: { ...step.approval!, state: 'expired' } }} deciding={null} onDecide={vi.fn()} />)
    expect(screen.getByText('This approval expired. Nothing ran.')).toBeInTheDocument()
  })

  it('shows the warning for code CloudBuddy cannot see into', () => {
    const step = find('n9')
    const warning = "CloudBuddy can't see what this does inside. Anything it changes won't show on the canvas."
    render(<ApprovalBlock step={{ ...step, approval: { ...step.approval!, warning } }} deciding={null} onDecide={vi.fn()} />)
    expect(screen.getByText(warning)).toBeInTheDocument()
  })
})
