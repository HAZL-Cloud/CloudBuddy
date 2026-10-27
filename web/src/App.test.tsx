import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { configureMock } from './mock/mockApi'
import { renderWithProviders } from './test/utils'

const sidebar = () => screen.getByRole('complementary', { name: 'Navigation' })
const cards = () => screen.queryAllByRole('article').filter((el) => el.hasAttribute('data-step-id'))
const openApp = async () => {
  const view = renderWithProviders(<App />)
  await screen.findByRole('article', { name: /Step 9: Send the launch email/ })
  return view
}

describe('signing in', () => {
  it('shows one calm message when the server says you are not signed in', async () => {
    configureMock({ signedIn: false })
    renderWithProviders(<App />)
    expect(await screen.findByText('Open CloudBuddy from the link it printed in your terminal.')).toBeInTheDocument()
    expect(screen.queryByRole('complementary', { name: 'Navigation' })).toBeNull()
  })
})

describe('the sidebar', () => {
  it('lists the chats with their status', async () => {
    renderWithProviders(<App />)
    const nav = within(await screen.findByRole('complementary', { name: 'Navigation' }))
    expect(await nav.findByText('Launch the paid newsletter')).toBeInTheDocument()
    expect(nav.getByText('Claude Code · Waiting for your OK')).toBeInTheDocument()
    expect(nav.getByText(/Claude Code · 10 steps · /)).toBeInTheDocument()
    expect(nav.getByText('Codex · 6 steps · Yesterday')).toBeInTheDocument()
    expect(nav.getByText(/Claude Code · 7 steps · /)).toBeInTheDocument()
  })

  it('shows how many approvals are waiting, and the agents on this computer', async () => {
    renderWithProviders(<App />)
    const nav = within(await screen.findByRole('complementary', { name: 'Navigation' }))
    expect(await nav.findByLabelText('1 waiting')).toBeInTheDocument()
    expect(await nav.findByText('2.1.311')).toBeInTheDocument()
    expect(nav.getByText('0.163.0')).toBeInTheDocument()
  })

  it('opens another chat', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByText('Remove unused EC2 test servers'))
    expect(await screen.findByRole('heading', { name: 'Remove unused EC2 test servers' })).toBeInTheDocument()
    expect(await screen.findByRole('article', { name: /Step 5: Terminate server test-db-2/ })).toBeInTheDocument()
  })
})

describe('chat and canvas', () => {
  it('shows every step in the chat but only cloud steps on the canvas', async () => {
    await openApp()
    // 11 steps in the chat rows (9 cloud + 2 local), 9 cards on the canvas.
    expect(screen.getByText('Saved the 3 DNS records to a file')).toBeInTheDocument()
    expect(cards().filter((c) => c.className.includes('result'))).toHaveLength(0)
    expect(cards()).toHaveLength(9)
    expect(screen.queryByRole('article', { name: /Saved the 3 DNS records/ })).toBeNull()
  })

  it('shows code spans in agent text as code', async () => {
    await openApp()
    expect(screen.getByText('shipyard.dev', { selector: 'code' })).toBeInTheDocument()
  })

  it('"Changes only" hides the reads and brings them back', async () => {
    await openApp()
    await userEvent.click(screen.getByRole('button', { name: 'Changes only' }))
    await waitFor(() => expect(cards()).toHaveLength(7))
    expect(screen.queryByRole('article', { name: /Step 1:/ })).toBeNull()
    expect(screen.getByRole('article', { name: /Step 9:/ })).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /^Go to step/ })).toHaveLength(7)
    await userEvent.click(screen.getByRole('button', { name: 'All steps' }))
    await waitFor(() => expect(cards()).toHaveLength(9))
  })

  it('has a scrubber with one tick per step, and replay controls', async () => {
    await openApp()
    expect(screen.getAllByRole('button', { name: /^Go to step/ })).toHaveLength(9)
    expect(screen.getByText(/Step 9 of 9/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Go to step 3: Added 3 email DNS records' }))
    expect(screen.getByText(/Step 3 of 9/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Latest' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('article', { name: /Step 5:/ })).toHaveClass('future'))
    expect(screen.getByRole('article', { name: /Step 3:/ })).toHaveClass('current')
    await userEvent.click(screen.getByRole('button', { name: 'Latest' }))
    expect(screen.queryByRole('button', { name: 'Latest' })).toBeNull()
    expect(screen.getByRole('article', { name: /Step 5:/ })).not.toHaveClass('future')
  })

  it('opens the details panel from a card, and closes it with Escape', async () => {
    await openApp()
    const card = screen.getByRole('article', { name: /Step 3:/ })
    // fireEvent, not userEvent: user-event's synthetic mousedown has no `view`, which d3-zoom needs in jsdom.
    fireEvent.click(within(card).getByText('Added 3 email DNS records'))
    const panel = await screen.findByRole('complementary', { name: 'Step details' })
    expect(within(panel).getByText('Cloudflare · DNS · via API')).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('complementary', { name: 'Step details' })).toBeNull()
  })

  it('"Approvals" jumps to the waiting step and opens its details', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: /Approvals/ }))
    const panel = await screen.findByRole('complementary', { name: 'Step details' })
    expect(within(panel).getByRole('heading', { name: 'Send the launch email to 1,240 people' })).toBeInTheDocument()
  })

  it('"Review on canvas" in the chat opens the step on the canvas', async () => {
    await openApp()
    await userEvent.click(screen.getByRole('button', { name: 'Review on canvas' }))
    const panel = await screen.findByRole('complementary', { name: 'Step details' })
    expect(within(panel).getByText('Why CloudBuddy asked')).toBeInTheDocument()
  })
})

describe('approving', () => {
  it('approve: the card runs, the chat finishes, and the result card appears', async () => {
    await openApp()
    // One Approve in the chat block, one on the canvas card.
    expect(screen.getAllByRole('button', { name: 'Approve' })).toHaveLength(2)
    await userEvent.click(screen.getAllByRole('button', { name: 'Approve' })[0]!)
    await screen.findByRole('article', { name: 'Run finished' }, { timeout: 4000 })
    expect(screen.getByText('Newsletter is live')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull()
    expect(screen.getByText(/Step 10 of 10/)).toBeInTheDocument()
    await waitFor(() => expect(within(sidebar()).queryByLabelText('1 waiting')).toBeNull())
  })

  it('says no: the step is not run, the stamp says so, the chat finishes', async () => {
    await openApp()
    await userEvent.click(screen.getAllByRole('button', { name: 'No' })[0]!)
    const card = await screen.findByRole('article', { name: /Step 9:/ })
    await waitFor(() => expect(within(card).getByText(/You said no ·/)).toBeInTheDocument())
    expect(within(card).getByText('Not run')).toBeInTheDocument()
    expect(await screen.findByText('Newsletter is set up')).toBeInTheDocument()
  })

  it('does not change anything on screen until the server has answered', async () => {
    configureMock({ speed: 1 }) // real delays: the request takes a moment
    await openApp()
    await userEvent.click(screen.getAllByRole('button', { name: 'Approve' })[0]!)
    // Still waiting for the server: the card is unchanged and its buttons are disabled.
    const card = screen.getByRole('article', { name: /Step 9:/ })
    expect(within(card).getByText('Waiting for your approval')).toBeInTheDocument()
    expect(within(card).getByRole('button', { name: 'Approving…' })).toBeDisabled()
    await waitFor(() => expect(within(card).queryByText('Waiting for your approval')).toBeNull(), { timeout: 3000 })
  })
})

describe('the composer and new chats', () => {
  it('sends a message to a chat that is not busy', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByText('Set up a monthly billing alarm'))
    const box = await screen.findByRole('textbox', { name: 'Message' })
    await userEvent.type(box, 'Thanks{Enter}')
    expect(await screen.findByText('Thanks')).toBeInTheDocument()
    expect(await screen.findByText(/This is mock mode, so nothing runs here/)).toBeInTheDocument()
  })

  it('says so when the agent is still working', async () => {
    await openApp()
    await userEvent.type(await screen.findByRole('textbox', { name: 'Message' }), 'hello{Enter}')
    expect(await screen.findByText(/Claude Code is still working/)).toBeInTheDocument()
  })

  it('"New chat" opens a fresh chat in the middle; the first message starts it with the agent from Settings', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'New chat' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(await screen.findByRole('heading', { name: 'New chat' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'What should Claude Code do?' })).toBeInTheDocument()
    // the open chat was in the Newsletter project, so the new one starts there too
    expect(screen.getByLabelText('Project')).toHaveValue('newsletter')
    const box = screen.getByRole('textbox', { name: 'Message' })
    expect(box).toHaveFocus()
    expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled()
    await userEvent.type(box, 'List my S3 buckets{Enter}')
    expect(await screen.findByRole('heading', { name: 'List my S3 buckets' })).toBeInTheDocument()
    expect(await within(sidebar()).findByText('List my S3 buckets')).toBeInTheDocument()
    expect(screen.getByText('~/code/shipyard-newsletter')).toBeInTheDocument()
  })

  it('a chat outside a project uses the folder from Settings, and says so when there is none', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'New chat' }))
    await userEvent.selectOptions(await screen.findByLabelText('Project'), 'No project')
    expect(screen.getByText('~/code/shipyard-infra')).toBeInTheDocument()

    await userEvent.click(within(sidebar()).getByRole('button', { name: /Settings/ }))
    const folder = await screen.findByLabelText('Folder for chats outside a project')
    await waitFor(() => expect(folder).toHaveValue('~/code/shipyard-infra'))
    await userEvent.clear(folder)
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    expect(await screen.findByText('Settings saved. New chats use them.')).toBeInTheDocument()

    await userEvent.click(within(sidebar()).getByRole('button', { name: /Chats/ }))
    expect(await screen.findByText(/Outside a project, the agent needs a folder/)).toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: 'Message' }), 'hello')
    expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Choose one in Settings' }))
    expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument()
  })
})

describe('projects', () => {
  it('narrows the chat list to a project, and a new chat starts in it', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: /^Shipyard shop/ }))
    expect(within(sidebar()).getByText('Chats in Shipyard shop')).toBeInTheDocument()
    expect(within(sidebar()).queryByText('Launch the paid newsletter')).toBeNull()
    expect(await screen.findByRole('heading', { name: 'Deploy shop site to S3 + CloudFront' })).toBeInTheDocument()
    // the chat shows its project and both folders
    expect(screen.getByText('~/code/shipyard-web')).toBeInTheDocument()
    expect(screen.getByText('~/code/shipyard-infra')).toBeInTheDocument()

    await userEvent.click(within(sidebar()).getByRole('button', { name: 'New chat in Shipyard shop' }))
    expect(await screen.findByLabelText('Project')).toHaveValue('shop')
    expect(screen.getByText('starts here')).toBeInTheDocument()
    expect(screen.getByText('Claude Code can read and change files in all 2 folders.')).toBeInTheDocument()

    await userEvent.click(within(sidebar()).getByRole('button', { name: /All chats/ }))
    expect(within(sidebar()).getByText('Launch the paid newsletter')).toBeInTheDocument()
  })

  it('creates a project from the sidebar and opens a new chat in it', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'New project' }))
    const dialog = await screen.findByRole('dialog', { name: 'New project' })
    const create = within(dialog).getByRole('button', { name: 'Create project' })
    expect(create).toBeDisabled()
    await userEvent.type(within(dialog).getByLabelText('Name'), 'Billing')
    await userEvent.type(within(dialog).getByLabelText('First folder (the agent starts here)'), 'code/billing')
    await userEvent.click(create)
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Use a full folder path')
    await userEvent.clear(within(dialog).getByLabelText('First folder (the agent starts here)'))
    await userEvent.type(within(dialog).getByLabelText('First folder (the agent starts here)'), '~/code/billing')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add a folder' }))
    await userEvent.type(within(dialog).getByLabelText('Folder 2'), '~/code/infra')
    await userEvent.click(create)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(await screen.findByLabelText('Project')).toHaveDisplayValue('Billing')
    expect(within(sidebar()).getByText('Chats in Billing')).toBeInTheDocument()
    expect(within(sidebar()).getByText('No chats in this project yet.')).toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: 'Message' }), 'Deploy billing{Enter}')
    expect(await screen.findByRole('heading', { name: 'Deploy billing' })).toBeInTheDocument()
    expect(await within(sidebar()).findByText('Deploy billing')).toBeInTheDocument()
  })

  it('edits and deletes a project; its chats stay', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'Edit Shipyard shop' }))
    const dialog = await screen.findByRole('dialog', { name: 'Edit project' })
    expect(within(dialog).getByLabelText('Folder 2')).toHaveValue('~/code/shipyard-infra')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Delete project' }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Delete it. Chats stay.' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(within(sidebar()).queryByRole('button', { name: /Shipyard shop/ })).toBeNull())
    expect(within(sidebar()).getByText('Deploy shop site to S3 + CloudFront')).toBeInTheDocument()
  })

  it('closes the project form with Escape', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'New project' }))
    await screen.findByRole('dialog')
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})

describe('Settings', () => {
  it('saves the agent; new chats use it', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: /Settings/ }))
    const codex = await screen.findByRole('radio', { name: /Codex/ })
    await waitFor(() => expect(screen.getByRole('radio', { name: /Claude Code/ })).toBeChecked())
    await userEvent.click(codex)
    await userEvent.click(screen.getByRole('button', { name: 'Save settings' }))
    expect(await screen.findByText('Settings saved. New chats use them.')).toBeInTheDocument()
    await userEvent.click(within(sidebar()).getByRole('button', { name: 'New chat' }))
    expect(await screen.findByRole('heading', { name: 'What should Codex do?' })).toBeInTheDocument()
  })

  it('lists the projects with their folders', async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: /Settings/ }))
    const page = await screen.findByRole('region', { name: 'Settings' })
    expect(within(page).getByText('2 projects')).toBeInTheDocument()
    expect(within(page).getByText('~/code/shipyard-newsletter')).toBeInTheDocument()
    await userEvent.click(within(page).getAllByRole('button', { name: 'Edit' })[0]!)
    expect(await screen.findByRole('dialog', { name: 'Edit project' })).toBeInTheDocument()
  })
})

describe('folding the canvas away', () => {
  it('hides the canvas to a strip, and opens it again', async () => {
    await openApp()
    await userEvent.click(screen.getByRole('button', { name: 'Hide the canvas' }))
    expect(cards()).toHaveLength(0)
    const strip = screen.getByRole('button', { name: 'Show the canvas: 9 steps, 1 waiting for you' })
    // the approval can still be answered in the chat
    expect(screen.getAllByRole('button', { name: 'Approve' })).toHaveLength(1)
    await userEvent.click(strip)
    await waitFor(() => expect(cards()).toHaveLength(9))
  })

  it('remembers it is folded, and opens it to show a step from the chat', async () => {
    const view = await openApp()
    await userEvent.click(screen.getByRole('button', { name: 'Hide the canvas' }))
    view.unmount()
    renderWithProviders(<App />)
    expect(await screen.findByRole('button', { name: /Show the canvas/ })).toBeInTheDocument()
    expect(cards()).toHaveLength(0)
    await userEvent.click(await screen.findByRole('button', { name: 'Review on canvas' }))
    expect(await screen.findByRole('complementary', { name: 'Step details' })).toBeInTheDocument()
  })
})

describe('the Connections page', () => {
  const openConnections = async () => {
    await openApp()
    await userEvent.click(within(sidebar()).getByRole('button', { name: /Connections/ }))
    return screen.findByRole('heading', { name: 'Connections' })
  }

  it('lists command-line tools and MCP servers with the adapter legend', async () => {
    await openConnections()
    expect(await screen.findByText('6 command-line tools · 4 MCP servers · checked at', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('Command-line tools')).toBeInTheDocument()
    expect(screen.getByText('MCP servers')).toBeInTheDocument()
    expect(screen.getAllByText('Built-in adapter').length).toBeGreaterThan(1)
    expect(screen.getByText('One adapter covers every AWS service.')).toBeInTheDocument()
  })

  it('saves a rule change', async () => {
    await openConnections()
    const rule = await screen.findByLabelText('Rule', { selector: '#rule-aws' })
    await userEvent.selectOptions(rule, 'every')
    expect(await screen.findByText('AWS: rule saved. It applies from the next step.')).toBeInTheDocument()
    await waitFor(() => expect(rule).toHaveValue('every'))
  })

  it('scans again', async () => {
    await openConnections()
    await userEvent.click(await screen.findByRole('button', { name: 'Scan again' }))
    expect(await screen.findByText('Scanned. 6 tools and 4 MCP servers found.')).toBeInTheDocument()
  })

  it('marks an unsupported server and does not let you change its rule', async () => {
    await openConnections()
    expect(await screen.findByText('Not supported yet')).toBeInTheDocument()
    expect(screen.getByLabelText('Rule', { selector: '#rule-linear' })).toBeDisabled()
  })
})
