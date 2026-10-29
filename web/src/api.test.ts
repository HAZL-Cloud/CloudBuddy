import { describe, expect, it } from 'vitest'
import { ApiError, api } from './api'

describe('the API client against the mock server', () => {
  it('reads the session, including the agents', async () => {
    const session = await api.session()
    expect(session.agents.map((a) => a.name)).toEqual(['claude', 'codex'])
  })

  it('lists conversations newest first, with waiting counts', async () => {
    const list = await api.conversations()
    expect(list.map((c) => c.id)).toEqual(['news', 'deploy', 'alarm', 'ec2'])
    expect(list[0]).toMatchObject({ status: 'waiting', waitingCount: 1 })
  })

  it('reports an unknown conversation as a 404', async () => {
    await expect(api.conversation('nope')).rejects.toMatchObject({ status: 404 })
  })
})

describe('approving and denying in mock mode (as in the mockup)', () => {
  it('approve: the step runs, then finishes, a delivery check runs, and a result appears', async () => {
    const step = await api.decide('ap-n9', 'approve')
    expect(step.status).toBe('running')
    expect(step.approval?.state).toBe('approved')

    await vi.waitFor(async () => {
      const c = await api.conversation('news')
      expect(c.status).toBe('done')
      expect(c.result?.title).toBe('Newsletter is live')
      expect(c.steps.find((s) => s.id === 'n9')?.status).toBe('done')
      expect(c.steps.at(-1)).toMatchObject({ title: 'Checked delivery', status: 'done' })
      expect(c.waitingCount).toBe(0)
    })
  })

  it('deny: the step is not run and the chat finishes with a smaller result', async () => {
    await api.decide('ap-n9', 'deny')
    const c = await api.conversation('news')
    expect(c.steps.find((s) => s.id === 'n9')).toMatchObject({ status: 'denied', approval: { state: 'denied' } })
    expect(c).toMatchObject({ status: 'done', result: { title: 'Newsletter is set up' } })
  })

  it('answers 409 when the approval was already decided', async () => {
    await api.decide('ap-n9', 'deny')
    const second = api.decide('ap-n9', 'approve')
    await expect(second).rejects.toBeInstanceOf(ApiError)
    await expect(second).rejects.toMatchObject({ status: 409, code: 'not_pending' })
  })

  it('answers 409 when a message is sent while a run is active', async () => {
    await expect(api.sendMessage('news', 'hello')).rejects.toMatchObject({ status: 409 })
  })

  it('starts a new chat with the agent and folder from Settings, and replies', async () => {
    await api.saveSettings({ agent: 'codex', folder: '~/code/x' })
    const { id } = await api.createConversation({ text: 'List my buckets' })
    await vi.waitFor(async () => expect((await api.conversation(id)).status).toBe('done'))
    const c = await api.conversation(id)
    expect(c).toMatchObject({ agent: 'codex', workdir: '~/code/x', extraFolders: [], title: 'List my buckets' })
    expect(c.chat.map((i) => i.type)).toEqual(['user', 'agent'])
  })

  it('starts a chat in a project: the first folder is where it starts, the rest come along', async () => {
    const p = await api.createProject({ name: 'Billing', folders: ['~/code/billing', '~/code/infra'] })
    const { id } = await api.createConversation({ text: 'Deploy it', projectId: p.id })
    expect(await api.conversation(id)).toMatchObject({ projectId: p.id, workdir: '~/code/billing', extraFolders: ['~/code/infra'] })
    expect((await api.projects()).find((x) => x.id === p.id)?.chatCount).toBe(1)
  })

  it('says no_folder when there is no project and no folder in Settings', async () => {
    await api.saveSettings({ agent: 'claude', folder: '' })
    await expect(api.createConversation({ text: 'hi' })).rejects.toMatchObject({ status: 400, code: 'no_folder' })
  })
})

describe('projects in mock mode', () => {
  it('creates, renames and deletes a project; its chats stay', async () => {
    await expect(api.createProject({ name: 'x', folders: ['relative'] })).rejects.toMatchObject({ status: 400 })
    const p = await api.createProject({ name: '  Shop   site ', folders: ['~/a', '~/a', '/b'] })
    expect(p).toMatchObject({ name: 'Shop site', folders: ['~/a', '/b'] })
    const { id } = await api.createConversation({ text: 'hi', projectId: p.id })
    expect((await api.updateProject(p.id, { name: 'Shop', folders: ['~/a'] })).chatCount).toBe(1)
    await api.deleteProject(p.id)
    await expect(api.deleteProject(p.id)).rejects.toMatchObject({ status: 404 })
    expect((await api.conversation(id)).projectId).toBeUndefined()
  })
})

describe('connections in mock mode', () => {
  it('lists them, saves a rule and scans again', async () => {
    const before = await api.connections()
    expect(before.cli.length + before.mcp.length).toBe(10)
    const updated = await api.setRule('aws', 'every')
    expect(updated.rule).toBe('every')
    expect((await api.connections()).cli.find((c) => c.id === 'aws')?.rule).toBe('every')
    const scanned = await api.scanConnections()
    expect(Date.parse(scanned.scannedAt)).toBeGreaterThanOrEqual(Date.parse(before.scannedAt))
  })
})
