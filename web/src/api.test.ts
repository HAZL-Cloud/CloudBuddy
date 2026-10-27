import { describe, expect, it } from 'vitest'
import { ApiError, NotSignedInError, api } from './api'
import { mockRequest, configureMock } from './mock/mockApi'

describe('the API client against the mock server', () => {
  it('reads the session, including the agents', async () => {
    const session = await api.session()
    expect(session.csrfToken).toBeTruthy()
    expect(session.agents.map((a) => a.name)).toEqual(['claude', 'codex'])
  })

  it('lists conversations newest first, with waiting counts', async () => {
    const list = await api.conversations()
    expect(list.map((c) => c.id)).toEqual(['news', 'deploy', 'alarm', 'ec2'])
    expect(list[0]).toMatchObject({ status: 'waiting', waitingCount: 1 })
  })

  it('sends the CSRF token on writes: the server refuses a write without it', async () => {
    await expect(mockRequest({ method: 'POST', path: '/api/approvals/ap-n9', body: { decision: 'approve' }, csrf: null })).rejects.toMatchObject({ status: 403 })
    const step = await api.decide('ap-n9', 'deny')
    expect(step.status).toBe('denied')
  })

  it('turns a 401 into NotSignedInError', async () => {
    configureMock({ signedIn: false })
    await expect(api.conversations()).rejects.toBeInstanceOf(NotSignedInError)
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

  it('starts a new chat and replies', async () => {
    const { id } = await api.createConversation({ agent: 'codex', workdir: '~/code/x', text: 'List my buckets' })
    await vi.waitFor(async () => expect((await api.conversation(id)).status).toBe('done'))
    const c = await api.conversation(id)
    expect(c).toMatchObject({ agent: 'codex', workdir: '~/code/x', title: 'List my buckets' })
    expect(c.chat.map((i) => i.type)).toEqual(['user', 'agent'])
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
