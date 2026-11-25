import { ApiError } from '../api'
import type { DashboardDetail, DashboardSummary, Point, StreamEvent, Tile, TileValue } from '../types'

// Dashboards for the mock server (D-067): the same shapes and status codes as docs/api.md.

const iso = (msAgo: number) => new Date(Date.now() - msAgo).toISOString()
const H = 3600_000

function history(hours: number, start: number, end: number, step = 1): Point[] {
  const out: Point[] = []
  const n = Math.max(2, Math.floor(hours / step))
  for (let i = 0; i <= n; i++) {
    const f = i / n
    const wobble = Math.round(Math.sin(i * 1.7) * 3)
    out.push({ t: iso((n - i) * step * H), v: Math.round(start + (end - start) * f) + (i === n ? 0 : wobble) })
  }
  return out
}

function usersDashboard(): DashboardDetail {
  const tiles: Tile[] = [
    { key: 'users', type: 'number', title: 'Users', source: 'users', width: 1, unit: 'users', good: 'up' },
    { key: 'paying', type: 'number', title: 'Paying customers', source: 'stripe', path: 'data.length', width: 1, good: 'up' },
    { key: 'mrr', type: 'number', title: 'Monthly revenue', source: 'mrr', width: 1, format: 'currency', currency: 'USD', good: 'up' },
    { key: 'api', type: 'status', title: 'API', source: 'health', width: 1 },
    { key: 'users-over-time', type: 'line', title: 'Users over time', source: 'users', width: 2 },
    { key: 'signups', type: 'bars', title: 'Signups per day', source: 'daily', labelField: 'day', valueField: 'signups', width: 2 },
    { key: 'plans', type: 'table', title: 'Users by plan', source: 'plans', width: 4 },
  ]
  const users = history(24, 1162, 1204)
  const values: TileValue[] = [
    { key: 'users', state: 'ok', number: 1204, compare: { value: 1162, at: iso(24 * H), label: 'in 24 hours' }, spark: users, updatedAt: iso(4 * 60_000) },
    { key: 'paying', state: 'ok', number: 87, compare: { value: 84, at: iso(24 * H), label: 'in 24 hours' }, spark: history(24, 84, 87), updatedAt: iso(4 * 60_000) },
    { key: 'mrr', state: 'waiting', message: 'Waiting for your OK to run this command on the timer.' },
    { key: 'api', state: 'ok', status: 'ok', text: 'OK', updatedAt: iso(4 * 60_000) },
    { key: 'users-over-time', state: 'ok', number: 1204, series: [{ label: 'Users over time', points: users }], updatedAt: iso(4 * 60_000) },
    {
      key: 'signups',
      state: 'ok',
      bars: [
        { label: 'Nov 2', value: 18 },
        { label: 'Nov 1', value: 24 },
        { label: 'Oct 31', value: 11 },
        { label: 'Oct 30', value: 15 },
        { label: 'Oct 29', value: 9 },
      ],
      updatedAt: iso(4 * 60_000),
    },
    {
      key: 'plans',
      state: 'ok',
      rows: [
        { plan: 'free', users: 1117, share: 92.8 },
        { plan: 'pro', users: 79, share: 6.6 },
        { plan: 'team', users: 8, share: 0.7 },
      ],
      columns: [
        { field: 'plan', label: 'Plan' },
        { field: 'users', label: 'Users' },
        { field: 'share', label: 'Share', format: 'percent' },
      ],
      updatedAt: iso(4 * 60_000),
    },
  ]
  const ok = (output: string, ms = 380) => ({ state: 'ok' as const, output, at: iso(4 * 60_000), durationMs: ms, exitCode: 0 })
  return {
    id: 'db_users',
    title: 'Users',
    description: 'Sign-ups, paying customers and revenue for the shop.',
    conversationId: 'deploy',
    refresh: '15m',
    paused: false,
    running: false,
    lastRunAt: iso(4 * 60_000),
    nextRunAt: iso(-11 * 60_000),
    updatedAt: iso(2 * H),
    needsOk: 1,
    failing: 0,
    tileCount: tiles.length,
    headlines: [],
    workdir: '/Users/you/code/shop-web',
    range: '24h',
    tiles,
    values,
    sources: [
      { key: 'users', label: 'All users', command: `psql "$DATABASE_URL" -XtAc 'select count(*) from users'`, cwd: '/Users/you/code/shop-web', format: 'auto', verdict: 'read', lastRun: ok('1204\n') },
      { key: 'stripe', label: 'Active subscriptions', command: 'stripe subscriptions list --status active --limit 100', cwd: '/Users/you/code/shop-web', format: 'auto', verdict: 'read', lastRun: ok('{"data":[…87 items]}', 910) },
      {
        key: 'mrr',
        label: 'Monthly revenue',
        revision: 'rev-mrr-1',
        command: 'node scripts/mrr.js',
        cwd: '/Users/you/code/shop-web',
        format: 'auto',
        verdict: 'needs_ok',
        reason: 'It runs a program or script, and CloudBuddy can’t see what it does inside.',
        lastRun: { state: 'waiting', output: '', at: iso(4 * 60_000), durationMs: 0, note: 'It runs a program or script, and CloudBuddy can’t see what it does inside.' },
      },
      { key: 'health', label: 'API health', command: 'cat status/api.json', cwd: '/Users/you/code/shop-web', format: 'auto', verdict: 'read', lastRun: ok('{"state":"healthy"}', 12) },
      {
        key: 'daily',
        label: 'Signups per day',
        command: `psql "$DATABASE_URL" -X --csv -c 'select date(created_at) as day, count(*) as signups from users group by 1 order by 1 desc limit 5'`,
        cwd: '/Users/you/code/shop-web',
        format: 'csv',
        verdict: 'read',
        lastRun: ok('day,signups\n2026-11-02,18\n…'),
      },
      {
        key: 'plans',
        label: 'Users by plan',
        command: `psql "$DATABASE_URL" -XtAc "select json_agg(t) from (select plan, count(*) as users from users group by 1) t"`,
        cwd: '/Users/you/code/shop-web',
        format: 'auto',
        verdict: 'read',
        lastRun: ok('[{"plan":"free","users":1117},…]'),
      },
    ],
    refreshChoices: ['1m', '5m', '15m', '30m', '1h', '6h', '12h', '1d'],
    rangeChoices: [
      { key: '1h', label: '1 hour' },
      { key: '24h', label: '24 hours' },
      { key: '7d', label: '7 days' },
      { key: '30d', label: '30 days' },
    ],
  }
}

/** A day `daysAgo` before today, as a plain date ("2026-11-14"). */
const dayISO = (daysAgo: number) => {
  const d = new Date(Date.now() - daysAgo * 24 * H)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

// A dashboard whose numbers stopped refreshing: its traffic command fails (the
// CLI's login expired) and a script waits for an OK again after a git checkout.
function trafficDashboard(): DashboardDetail {
  const D = 24 * H
  const tiles: Tile[] = [
    { key: 'visitors7d', type: 'number', title: 'Visitors, last 7 days', source: 'traffic', path: 'visitors7d', width: 1, unit: 'visitors', good: 'up', compare: '7d', note: 'Unique visitors, complete days' },
    { key: 'avg', type: 'number', title: 'Avg visitors / day', source: 'traffic', path: 'avg', width: 1, good: 'up' },
    { key: 'pageviews7d', type: 'number', title: 'Page views, last 7 days', source: 'traffic', path: 'pageViews7d', width: 1, good: 'up', compare: '7d' },
    { key: 'signins', type: 'number', title: 'Signed in today', source: 'signins', path: 'today', width: 1, good: 'up' },
    { key: 'signins-over-time', type: 'line', title: 'Sign-ins over time', series: [{ source: 'signins', path: 'today', label: 'Signed in' }], width: 4 },
    { key: 'daily', type: 'line', title: 'Visitors per day (30d)', source: 'traffic', path: 'daily', labelField: 'date', valueField: 'visitors', width: 2 },
    { key: 'dailypv', type: 'bars', title: 'Page views per day (30d)', source: 'traffic', path: 'daily', labelField: 'date', valueField: 'pageViews', limit: 14, width: 2 },
  ]
  const days = Array.from({ length: 30 }, (_, i) => {
    const visitors = 160 + Math.round(Math.sin(i * 1.3) * 45 + Math.cos(i * 0.7) * 25)
    return { date: dayISO(39 - i), visitors, pageViews: Math.round(visitors * 0.7 + ((i * 37) % 60)) }
  })
  const failNote = [
    'node:internal/errors:983',
    '  const err = new Error(message);',
    '              ^',
    '',
    'Error: Command failed: npx wrangler whoami',
    '    at genericNodeError (node:internal/errors:983:15)',
    '    at checkExecSyncError (node:child_process:892:11)',
    '',
    'Node.js v24.2.0',
  ].join('\n')
  const stale = { stale: true, updatedAt: iso(10 * D), message: 'The last refresh failed, so this is from an earlier run. The command failed (exit code 1): Error: Command failed: npx wrangler whoami' }
  const values: TileValue[] = [
    { key: 'visitors7d', state: 'ok', number: 1397, compare: { value: 1271, at: iso(17 * D), label: 'in 7 days' }, ...stale },
    { key: 'avg', state: 'ok', number: 200, compare: { value: 182, at: iso(11 * D), label: 'in 24 hours' }, ...stale },
    { key: 'pageviews7d', state: 'ok', number: 1075, compare: { value: 1120, at: iso(17 * D), label: 'in 7 days' }, ...stale },
    { key: 'signins', state: 'waiting', message: 'CloudBuddy runs this tile’s command once you allow it.' },
    { key: 'daily', state: 'ok', series: [{ label: 'Visitors per day (30d)', bars: days.map((d) => ({ label: d.date, value: d.visitors })) }], ...stale },
    // limit 14 of 30 days, oldest first: the server keeps the newest
    { key: 'dailypv', state: 'ok', bars: days.slice(-14).map((d) => ({ label: d.date, value: d.pageViews })), more: days.length - 14, oldestFirst: true, ...stale },
    {
      key: 'signins-over-time',
      state: 'ok',
      waiting: true,
      message: 'A command for this chart is waiting for your OK.',
      series: [{ label: 'Signed in', points: history(24 * 6, 14, 23, 6).map((p) => ({ ...p, t: new Date(new Date(p.t).getTime() - 3 * D).toISOString() })) }],
      updatedAt: iso(3 * D),
    },
  ]
  const cwd = '/Users/you/code/site'
  return {
    id: 'db_traffic',
    title: 'Site traffic',
    description: 'Visitors and page views from the CDN, and sign-ins.',
    conversationId: 'deploy',
    refresh: '1h',
    paused: false,
    running: false,
    lastRunAt: iso(49 * 60_000),
    nextRunAt: iso(-11 * 60_000),
    updatedAt: iso(14 * D),
    needsOk: 1,
    failing: 1,
    tileCount: tiles.length,
    headlines: [],
    workdir: cwd,
    range: '24h',
    tiles,
    values,
    sources: [
      {
        key: 'traffic',
        label: 'CDN daily traffic, last 30 days',
        command: 'node scripts/cdn-traffic.mjs 30',
        cwd,
        format: 'json',
        verdict: 'needs_ok',
        approval: { allowed: true, decidedAt: iso(14 * D) },
        lastRun: { state: 'failed', exitCode: 1, output: '', note: failNote, at: iso(49 * 60_000), durationMs: 1210 },
      },
      {
        key: 'signins',
        label: 'Sign-in numbers',
        revision: 'rev-signins-2',
        command: 'python3 scripts/signins.py',
        cwd,
        format: 'json',
        verdict: 'needs_ok',
        reason: 'The files this command runs changed after you allowed it, so it needs your OK again. CloudBuddy can’t tell that this only reads: it runs python3.',
        approval: { allowed: true, decidedAt: iso(3 * D), changed: true },
        lastRun: {
          state: 'waiting',
          output: '',
          at: iso(49 * 60_000),
          durationMs: 0,
          note: 'The files this command runs changed after you allowed it, so it needs your OK again.',
        },
      },
    ],
    refreshChoices: ['1m', '5m', '15m', '30m', '1h', '6h', '12h', '1d'],
    rangeChoices: [
      { key: '1h', label: '1 hour' },
      { key: '24h', label: '24 hours' },
      { key: '7d', label: '7 days' },
      { key: '30d', label: '30 days' },
    ],
  }
}

export function seedDashboards(): DashboardDetail[] {
  return [usersDashboard(), trafficDashboard()]
}

function summary(d: DashboardDetail): DashboardSummary {
  const needsOk = d.sources.filter((s) => s.lastRun?.state === 'waiting').length
  const failing = d.sources.filter((s) => s.lastRun?.state === 'failed' || s.lastRun?.state === 'blocked').length
  const headlines = d.tiles
    .map((tile, i) => ({ tile, value: d.values[i] as TileValue }))
    .filter((h) => h.tile.type === 'number')
    .slice(0, 3)
  return {
    id: d.id,
    title: d.title,
    description: d.description,
    conversationId: d.conversationId,
    refresh: d.refresh,
    paused: d.paused,
    running: d.running,
    lastRunAt: d.lastRunAt,
    nextRunAt: d.paused ? undefined : d.nextRunAt,
    updatedAt: d.updatedAt,
    needsOk,
    failing,
    tileCount: d.tiles.length,
    headlines,
  }
}

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T

export interface DashMock {
  dashboards: DashboardDetail[]
  later: (ms: number, fn: () => void) => void
  emit: (ev: StreamEvent) => void
}

function find(m: DashMock, id: string): DashboardDetail {
  const d = m.dashboards.find((x) => x.id === id)
  if (!d) throw new ApiError(404, 'not_found')
  return d
}

function detail(d: DashboardDetail): DashboardDetail {
  const s = summary(d)
  return clone({ ...d, ...s, nextRunAt: s.nextRunAt })
}

/** Answers a dashboards request, or returns undefined for paths it doesn't know. */
export function dashboardRoute(m: DashMock, method: string, fullPath: string, b: Record<string, unknown>): { hit: boolean; value?: unknown } {
  const [path, query = ''] = fullPath.split('?')
  let r: RegExpMatchArray | null
  if (method === 'GET' && path === '/api/dashboards') return { hit: true, value: clone(m.dashboards.map(summary)) }
  if ((r = (path ?? '').match(/^\/api\/dashboards\/([^/]+)$/))) {
    const id = decodeURIComponent(r[1] as string)
    if (method === 'GET') {
      const d = find(m, id)
      const range = new URLSearchParams(query).get('range')
      if (range && d.rangeChoices.some((c) => c.key === range)) d.range = range
      return { hit: true, value: detail(d) }
    }
    if (method === 'DELETE') {
      find(m, id)
      m.dashboards = m.dashboards.filter((x) => x.id !== id)
      m.emit({ type: 'dashboards' })
      return { hit: true, value: undefined }
    }
  }
  if ((r = (path ?? '').match(/^\/api\/dashboards\/([^/]+)\/schedule$/)) && method === 'PUT') {
    const d = find(m, decodeURIComponent(r[1] as string))
    const refresh = String(b.refresh ?? '')
    if (!d.refreshChoices.includes(refresh)) throw new ApiError(400, 'refresh isn’t one of the choices')
    d.refresh = refresh
    d.paused = Boolean(b.paused)
    m.emit({ type: 'dashboard', dashboardId: d.id })
    return { hit: true, value: detail(d) }
  }
  if ((r = (path ?? '').match(/^\/api\/dashboards\/([^/]+)\/refresh$/)) && method === 'POST') {
    const d = find(m, decodeURIComponent(r[1] as string))
    d.running = true
    m.emit({ type: 'dashboard', dashboardId: d.id })
    m.later(900, () => {
      d.running = false
      d.lastRunAt = new Date().toISOString()
      m.emit({ type: 'dashboard', dashboardId: d.id })
    })
    return { hit: true, value: undefined }
  }
  if ((r = (path ?? '').match(/^\/api\/dashboards\/([^/]+)\/sources\/([^/]+)\/decision$/)) && method === 'POST') {
    const d = find(m, decodeURIComponent(r[1] as string))
    const key = decodeURIComponent(r[2] as string)
    const src = d.sources.find((s) => s.key === key)
    if (!src) throw new ApiError(404, 'not_found')
    const allow = b.decision === 'allow'
    if (b.revision !== (src.revision ?? '')) throw new ApiError(409, 'changed')
    src.approval = { allowed: allow, decidedAt: new Date().toISOString() }
    if (src.verdict === 'needs_ok') {
      src.lastRun = allow
        ? { state: 'ok', output: '3480\n', at: new Date().toISOString(), durationMs: 640, exitCode: 0 }
        : { state: 'declined', output: '', at: new Date().toISOString(), durationMs: 0 }
      d.tiles.forEach((t, i) => {
        if (t.source !== key) return
        d.values[i] = allow
          ? { key: t.key, state: 'ok', number: d.values[i]?.number ?? 3480, updatedAt: new Date().toISOString() }
          : { key: t.key, state: 'declined', message: 'You didn’t allow this command, so it doesn’t run.' }
      })
    }
    m.emit({ type: 'dashboard', dashboardId: d.id })
    return { hit: true, value: detail(d) }
  }
  return { hit: false }
}
