import { useRef, useState } from 'react'
import { ApiError } from '../api'
import { formatValue, refreshLabel, untilLabel } from '../dashFormat'
import { duration, plural, relativeWhen } from '../format'
import { useDashboard, useDashboards, useDashboardSchedule, useDecideDashboardSource, useDeleteDashboard, useRefreshDashboard } from '../queries'
import type { DashboardDetail, DashboardSource, DashboardSummary } from '../types'
import { Sparkline, TileView } from './DashTiles'
import { Dialog } from './Dialog'
import { Icon } from './Icon'
import { useToast } from './Toast'

// The Dashboards page (D-067): every dashboard a chat built, refreshed by
// CloudBuddy on a timer, and the commands that wait for the user's OK.

export interface DashboardsPageProps {
  selectedId: string | null
  onSelect: (id: string | null) => void
  onOpenChat: (conversationId: string) => void
  onNewChat: () => void
}

export function DashboardsPage({ selectedId, onSelect, onOpenChat, onNewChat }: DashboardsPageProps) {
  return (
    <section className="dash-page" aria-label="Dashboards">
      {selectedId ? (
        <DashboardView id={selectedId} onBack={() => onSelect(null)} onOpenChat={onOpenChat} />
      ) : (
        <DashboardList onSelect={onSelect} onNewChat={onNewChat} />
      )}
    </section>
  )
}

function statusLine(d: DashboardSummary): { text: string; tone: '' | 'wait' | 'bad' } {
  if (d.needsOk > 0) return { text: `${plural(d.needsOk, 'command')} waiting for your OK`, tone: 'wait' }
  if (d.failing > 0) return { text: `${plural(d.failing, 'command')} not working`, tone: 'bad' }
  if (d.running) return { text: 'Refreshing…', tone: '' }
  if (d.paused) return { text: 'Paused', tone: '' }
  return { text: d.lastRunAt ? `Updated ${relativeWhen(d.lastRunAt).toLowerCase()}` : 'Not refreshed yet', tone: '' }
}

function DashboardList({ onSelect, onNewChat }: { onSelect: (id: string) => void; onNewChat: () => void }) {
  const { data, isLoading, isError, refetch } = useDashboards()
  return (
    <div className="dash-inner">
      <div className="dash-head">
        <div className="dash-head-text">
          <h2>Dashboards</h2>
          <p>
            Numbers, charts and tables that CloudBuddy keeps fresh. Build one in a chat, for example: &ldquo;Make a dashboard of
            how many users I have, refreshed every 15 minutes.&rdquo; After that CloudBuddy reruns its commands itself, without the
            agent.
          </p>
        </div>
        <button type="button" className="btn" onClick={onNewChat}>
          <Icon name="plus" small />
          New chat
        </button>
      </div>
      {isLoading && <p className="dash-muted">Loading…</p>}
      {isError && (
        <div className="field-error" role="alert">
          Couldn&apos;t load the dashboards.{' '}
          <button type="button" className="btn link inline" onClick={() => void refetch()}>
            Try again
          </button>
        </div>
      )}
      {data && data.length === 0 && (
        <div className="dash-empty">
          <Icon name="grid" />
          <div>
            <b>No dashboards yet</b>
            <p>
              Ask the agent in any chat for a dashboard. It finds the data, saves the dashboard, and a card on the chat&apos;s
              canvas opens it.
            </p>
          </div>
        </div>
      )}
      {data && data.length > 0 && (
        <ul className="dash-cards">
          {data.map((d) => {
            const s = statusLine(d)
            return (
              <li key={d.id}>
                <button type="button" className="dash-card" onClick={() => onSelect(d.id)}>
                  <span className="dash-card-top">
                    <b>{d.title}</b>
                    <span className="dash-card-every">{d.paused ? 'Paused' : refreshLabel(d.refresh)}</span>
                  </span>
                  {d.description && <span className="dash-card-desc">{d.description}</span>}
                  {d.headlines.length > 0 && (
                    <span className="dash-card-nums">
                      {d.headlines.map(({ tile, value }) => (
                        <span key={tile.key} className="dash-card-num">
                          <span className="k">{tile.title}</span>
                          <span className="v">{value.number !== undefined ? formatValue(value.number, tile) : '—'}</span>
                          {value.spark && value.spark.length > 1 && <Sparkline points={value.spark} label={`${tile.title} over time`} />}
                        </span>
                      ))}
                    </span>
                  )}
                  <span className={`dash-card-status ${s.tone}`}>
                    <span className={`dot ${s.tone === 'wait' ? 'wait' : s.tone === 'bad' ? 'bad' : d.running ? 'run' : 'ok'}`} />
                    {s.text}
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function verdictBadge(s: DashboardSource): { label: string; cls: string } {
  if (s.verdict === 'never') return { label: "Can't run from a dashboard", cls: 'never' }
  if (s.verdict === 'needs_ok') {
    if (s.approval?.allowed && !s.approval.changed) return { label: 'Allowed by you', cls: 'allowed' }
    if (s.approval && !s.approval.allowed && !s.approval.changed) return { label: 'Not allowed', cls: 'never' }
    return { label: 'Waiting for your OK', cls: 'wait' }
  }
  if (s.verdict === 'read') return { label: 'Runs by itself', cls: 'read' }
  return { label: 'Not run yet', cls: '' }
}

const waitsForYou = (s: DashboardSource) => s.verdict === 'needs_ok' && s.lastRun?.state === 'waiting'

function DashboardView({ id, onBack, onOpenChat }: { id: string; onBack: () => void; onOpenChat: (conversationId: string) => void }) {
  const [range, setRange] = useState('24h')
  const { data, isError, error, isPlaceholderData, refetch } = useDashboard(id, range)
  const approvalsRef = useRef<HTMLDivElement>(null)

  if (isError && error instanceof ApiError && error.status === 404) {
    return (
      <div className="dash-inner">
        <button type="button" className="btn link dash-back" onClick={onBack}>
          ← All dashboards
        </button>
        <p className="dash-muted">This dashboard was deleted.</p>
      </div>
    )
  }
  if (!data) {
    return (
      <div className="dash-inner">
        <button type="button" className="btn link dash-back" onClick={onBack}>
          ← All dashboards
        </button>
        {isError ? (
          <div className="field-error" role="alert">
            Couldn&apos;t load this dashboard.{' '}
            <button type="button" className="btn link inline" onClick={() => void refetch()}>
              Try again
            </button>
          </div>
        ) : (
          <p className="dash-muted">Loading…</p>
        )}
      </div>
    )
  }
  const waiting = data.sources.filter(waitsForYou)
  const hasCharts = data.tiles.some((t) => t.type === 'line' || t.type === 'number' || (t.type === 'status' && t.path))
  const span = { '1h': 3600_000, '24h': 86_400_000, '7d': 7 * 86_400_000, '30d': 30 * 86_400_000 }[data.range] ?? 86_400_000
  const values = new Map(data.values.map((v) => [v.key, v]))
  const review = () => approvalsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })

  return (
    <div className="dash-inner">
      <button type="button" className="btn link dash-back" onClick={onBack}>
        ← All dashboards
      </button>
      <Header d={data} onOpenChat={onOpenChat} onDeleted={onBack} />
      {waiting.length > 0 && (
        <div className="dash-approvals" ref={approvalsRef} role="region" aria-label="Commands waiting for your OK">
          <div className="dash-approvals-h">
            <Icon name="hand" />
            <div>
              <b>{waiting.length === 1 ? '1 command waits for your OK' : `${waiting.length} commands wait for your OK`}</b>
              <p>
                CloudBuddy can&apos;t tell that {waiting.length === 1 ? 'this command only reads' : 'these commands only read'}, so{' '}
                {waiting.length === 1 ? 'it doesn’t' : 'they don’t'} run on the timer until you allow{' '}
                {waiting.length === 1 ? 'it' : 'them'}. If a command or the script it runs changes, CloudBuddy asks again.
              </p>
            </div>
          </div>
          {waiting.map((s) => (
            <SourceDecision key={s.key} dashboardId={data.id} source={s} />
          ))}
        </div>
      )}
      {hasCharts && (
        <div className="dash-filters" role="group" aria-label="Time range for trends and charts">
          <span className="dash-filters-k">Show the last</span>
          <div className="seg">
            {data.rangeChoices.map((r) => (
              <button key={r.key} type="button" className={data.range === r.key ? 'on' : ''} aria-pressed={data.range === r.key} onClick={() => setRange(r.key)}>
                {r.label}
              </button>
            ))}
          </div>
        </div>
      )}
      <div className={`dash-grid ${isPlaceholderData ? 'loading' : ''}`}>
        {data.tiles.map((t) => (
          <TileView key={t.key} tile={t} value={values.get(t.key)} span={span} onReview={review} />
        ))}
      </div>
      <Sources d={data} />
    </div>
  )
}

function Header({ d, onOpenChat, onDeleted }: { d: DashboardDetail; onOpenChat: (id: string) => void; onDeleted: () => void }) {
  const toast = useToast()
  const schedule = useDashboardSchedule()
  const refresh = useRefreshDashboard()
  const del = useDeleteDashboard()
  const [confirming, setConfirming] = useState(false)
  const setSchedule = (refreshEvery: string, paused: boolean) =>
    schedule.mutate(
      { id: d.id, refresh: refreshEvery, paused },
      {
        onSuccess: () => toast(paused ? 'Paused. It won’t refresh until you resume it.' : `Refreshes ${refreshLabel(refreshEvery)}.`),
        onError: () => toast("Couldn't change the schedule. Try again."),
      },
    )
  const choices = d.refreshChoices.includes(d.refresh) ? d.refreshChoices : [d.refresh, ...d.refreshChoices]
  let meta = d.paused ? 'Paused' : `Refreshes ${refreshLabel(d.refresh)}`
  if (d.running) meta += ' · Refreshing now…'
  else if (d.lastRunAt) meta += ` · Updated ${relativeWhen(d.lastRunAt).toLowerCase()}`
  if (!d.paused && !d.running && d.nextRunAt) meta += ` · Next ${untilLabel(d.nextRunAt)}`

  return (
    <div className="dash-head">
      <div className="dash-head-text">
        <h2>{d.title}</h2>
        {d.description && <p>{d.description}</p>}
        <div className="dash-meta" aria-live="polite">
          {meta}
        </div>
      </div>
      <div className="dash-actions">
        <button
          type="button"
          className="btn"
          disabled={d.running || refresh.isPending}
          onClick={() => refresh.mutate(d.id, { onError: () => toast("Couldn't start a refresh. Try again.") })}
        >
          <Icon name="reset" small />
          {d.running ? 'Refreshing…' : 'Refresh now'}
        </button>
        <label className="dash-every">
          <span className="sr-only">How often</span>
          <select value={d.refresh} disabled={schedule.isPending} onChange={(e) => setSchedule(e.target.value, d.paused)}>
            {choices.map((c) => (
              <option key={c} value={c}>
                {refreshLabel(c).replace(/^every/, 'Every')}
              </option>
            ))}
          </select>
        </label>
        <button type="button" className="btn" disabled={schedule.isPending} onClick={() => setSchedule(d.refresh, !d.paused)}>
          <Icon name={d.paused ? 'play' : 'pause'} small />
          {d.paused ? 'Resume' : 'Pause'}
        </button>
        <button
          type="button"
          className="btn"
          disabled={!d.conversationId}
          title={d.conversationId ? 'Open the chat that built this dashboard, to change it' : 'The chat that built it is gone'}
          onClick={() => d.conversationId && onOpenChat(d.conversationId)}
        >
          <Icon name="chat" small />
          Change in chat
        </button>
        <button type="button" className="btn icon-only" aria-label="Delete this dashboard" title="Delete this dashboard" onClick={() => setConfirming(true)}>
          <Icon name="trash" small />
        </button>
      </div>
      {confirming && (
        <Dialog title={`Delete “${d.title}”?`} onClose={() => setConfirming(false)}>
          <p className="field-help">
            The dashboard and the history of its numbers go, and its commands stop running. The chat that built it stays, so
            you can ask for it again.
          </p>
          <div className="dialog-actions">
            <button type="button" className="btn" onClick={() => setConfirming(false)}>
              Keep it
            </button>
            <button
              type="button"
              className="btn danger"
              disabled={del.isPending}
              onClick={() =>
                del.mutate(d.id, {
                  onSuccess: () => {
                    toast('Dashboard deleted.')
                    setConfirming(false)
                    onDeleted()
                  },
                  onError: () => toast("Couldn't delete the dashboard. Try again."),
                })
              }
            >
              Delete dashboard
            </button>
          </div>
        </Dialog>
      )}
    </div>
  )
}

// The answer is only for the command the page showed; a 409 means the agent changed it since.
function decideError(err: unknown): string {
  return err instanceof ApiError && err.status === 409
    ? 'The command or the files it runs changed after this page showed them, so nothing was allowed. Look again.'
    : "Couldn't send your answer, so nothing changed. Try again."
}

function SourceDecision({ dashboardId, source }: { dashboardId: string; source: DashboardSource }) {
  const decide = useDecideDashboardSource()
  const toast = useToast()
  const send = (decision: 'allow' | 'deny') =>
    decide.mutate(
      { id: dashboardId, key: source.key, decision, revision: source.revision ?? '' },
      {
        onSuccess: () => toast(decision === 'allow' ? 'Allowed. It runs now and on every refresh.' : 'Not allowed. It won’t run.'),
        onError: (err) => toast(decideError(err)),
      },
    )
  const pending = decide.isPending ? decide.variables?.decision : null
  return (
    <div className="dash-decision">
      <div className="dash-decision-main">
        <b>{source.label || source.key}</b>
        <pre className="dash-cmd">{source.command}</pre>
        <span className="dash-cwd mono">in {source.cwd}</span>
        {source.reason && <p className="dash-why">{source.reason}</p>}
      </div>
      <div className="dash-decision-actions">
        <button type="button" className="btn" disabled={decide.isPending} onClick={() => send('deny')}>
          {pending === 'deny' ? 'Sending…' : 'Don’t allow'}
        </button>
        <button type="button" className="btn primary" disabled={decide.isPending} onClick={() => send('allow')}>
          {pending === 'allow' ? 'Sending…' : 'Allow on the timer'}
        </button>
      </div>
    </div>
  )
}

function Sources({ d }: { d: DashboardDetail }) {
  const decide = useDecideDashboardSource()
  const toast = useToast()
  const [open, setOpen] = useState<string | null>(null)
  return (
    <details className="dash-sources">
      <summary>
        Commands <span>{plural(d.sources.length, 'command')} · run in {d.workdir}</span>
      </summary>
      <ul>
        {d.sources.map((s) => {
          const b = verdictBadge(s)
          const r = s.lastRun
          const canStop = s.verdict === 'needs_ok' && s.approval?.allowed && !s.approval.changed
          const canAllow = s.verdict === 'needs_ok' && s.approval && !s.approval.allowed && !s.approval.changed
          return (
            <li key={s.key} className="dash-source">
              <div className="dash-source-top">
                <b>{s.label || s.key}</b>
                <span className={`verdict ${b.cls}`}>{b.label}</span>
              </div>
              <pre className="dash-cmd">{s.command}</pre>
              {s.verdict === 'never' && s.reason && <p className="dash-why">{s.reason}</p>}
              {r && (
                <div className="dash-run">
                  <span className={`dot ${r.state === 'ok' ? 'ok' : r.state === 'failed' ? 'bad' : 'wait'}`} />
                  <span>
                    {r.state === 'ok'
                      ? `Worked ${relativeWhen(r.at).toLowerCase()}, in ${duration(r.durationMs)}`
                      : r.state === 'failed'
                        ? `Failed ${relativeWhen(r.at).toLowerCase()}${r.exitCode !== undefined ? ` (exit code ${r.exitCode})` : ''}`
                        : r.state === 'waiting'
                          ? 'Not run: waiting for your OK'
                          : r.state === 'declined'
                            ? 'Not run: you didn’t allow it'
                            : 'Not run'}
                  </span>
                  {(r.output || r.note) && (
                    <button type="button" className="btn link inline" aria-expanded={open === s.key} onClick={() => setOpen(open === s.key ? null : s.key)}>
                      {open === s.key ? 'Hide output' : 'Show output'}
                    </button>
                  )}
                  {canStop && (
                    <button
                      type="button"
                      className="btn link inline"
                      disabled={decide.isPending}
                      onClick={() =>
                        decide.mutate(
                          { id: d.id, key: s.key, decision: 'deny', revision: s.revision ?? '' },
                          { onSuccess: () => toast('It won’t run any more.'), onError: (err) => toast(decideError(err)) },
                        )
                      }
                    >
                      Stop allowing
                    </button>
                  )}
                  {canAllow && (
                    <button
                      type="button"
                      className="btn link inline"
                      disabled={decide.isPending}
                      onClick={() =>
                        decide.mutate(
                          { id: d.id, key: s.key, decision: 'allow', revision: s.revision ?? '' },
                          { onSuccess: () => toast('Allowed. It runs now.'), onError: (err) => toast(decideError(err)) },
                        )
                      }
                    >
                      Allow
                    </button>
                  )}
                </div>
              )}
              {open === s.key && r && (
                <div className="dash-output">
                  {r.note && <pre className="dash-note">{r.note}</pre>}
                  {r.output && <pre>{r.output}</pre>}
                  {r.truncated && <p className="dash-muted">The output was too long; CloudBuddy kept the start.</p>}
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </details>
  )
}
