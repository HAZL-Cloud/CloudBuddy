import { useState } from 'react'
import { clockShort } from '../format'
import { useConnections, useScanConnections, useSetRule } from '../queries'
import type { Connection, Rule } from '../types'
import { ADAPTERS, RULES } from '../vocab'
import { Icon } from './Icon'
import { useToast } from './Toast'

function Row({ conn }: { conn: Connection }) {
  const setRule = useSetRule()
  const toast = useToast()
  // Shows the choice at once; the server's answer replaces it, and a failure puts the old value back.
  const [pending, setPending] = useState<Rule | null>(null)
  const rule = pending ?? conn.rule
  const adapter = ADAPTERS[conn.adapter]
  const onChange = (next: Rule) => {
    setPending(next)
    setRule.mutate(
      { id: conn.id, rule: next },
      {
        onSuccess: () => toast(`${conn.name}: rule saved. It applies from the next step.`),
        onError: () => toast(`Couldn't save the rule for ${conn.name}.`),
        onSettled: () => setPending(null),
      },
    )
  }
  return (
    <div className={`conn-row ${rule === 'off' ? 'off' : ''} ${conn.supported ? '' : 'unsupported'}`}>
      <div className="conn-svc">
        <span className="mark">{conn.mark}</span>
        <div>
          <b>
            {conn.name}
            {!conn.supported && <span className="soon">Not supported yet</span>}
          </b>
          <span className="mono">{conn.how}</span>
        </div>
      </div>
      <div className="cell">
        <span className="k">Signed in as</span>
        <b>{conn.account}</b>
        <span>{conn.detail}</span>
      </div>
      <div className="cell">
        <span className="k">Adapter</span>
        <span className={`ad ad-${conn.adapter}`}>{adapter.label}</span>
        <span>{conn.adapterNote ?? adapter.note}</span>
      </div>
      <div className="cell">
        <label className="k" htmlFor={`rule-${conn.id}`}>
          Rule
        </label>
        <select
          className="rule"
          id={`rule-${conn.id}`}
          value={rule}
          disabled={!conn.supported}
          onChange={(e) => onChange(e.target.value as Rule)}
        >
          {RULES.map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </div>
    </div>
  )
}

export function ConnectionsPage() {
  const { data, isLoading, isError, refetch } = useConnections()
  const scan = useScanConnections()
  const toast = useToast()

  const onScan = () =>
    scan.mutate(undefined, {
      onSuccess: (r) => toast(`Scanned. ${r.cli.length} tools and ${r.mcp.length} MCP servers found.`),
      onError: () => toast("Couldn't scan. Try again."),
    })

  return (
    <section className="connections" aria-label="Connections">
      <div className="conn-inner">
        <div className="conn-head">
          <div className="conn-head-text">
            <h2>Connections</h2>
            <p>
              Everything the agent can reach from this computer. CloudBuddy uses the logins already on your machine and
              never stores your keys.
            </p>
            {data && (
              <div className="conn-meta">
                {data.cli.length} command-line tools · {data.mcp.length} MCP servers · checked at {clockShort(data.scannedAt)}
              </div>
            )}
          </div>
          <button type="button" className="btn" onClick={onScan} disabled={scan.isPending}>
            <Icon name="reset" small />
            {scan.isPending ? 'Scanning…' : 'Scan again'}
          </button>
        </div>
        <div className="legend-grid">
          {Object.entries(ADAPTERS).map(([kind, a]) => (
            <div key={kind} className="legend-item">
              <span className={`ad ad-${kind}`}>{a.label}</span>
              {a.note}
            </div>
          ))}
        </div>
        {isError && (
          <div className="conn-foot">
            <p>
              <b>Could not load your connections.</b>{' '}
              <button type="button" className="btn link" onClick={() => void refetch()}>
                Try again
              </button>
            </p>
          </div>
        )}
        {isLoading && <p>Loading…</p>}
        {data && (
          <>
            <section className="conn-sec">
              <h3>
                Command-line tools <span>{data.cli.length} found on your PATH</span>
              </h3>
              <div className="conn-list">
                {data.cli.map((c) => (
                  <Row key={c.id} conn={c} />
                ))}
              </div>
            </section>
            <section className="conn-sec">
              <h3>
                MCP servers <span>{data.mcp.length} set up in your agents</span>
              </h3>
              <div className="conn-list">
                {data.mcp.map((c) => (
                  <Row key={c.id} conn={c} />
                ))}
              </div>
            </section>
          </>
        )}
        <div className="conn-foot">
          <Icon name="plug" />
          <p>
            <b>Using a service that isn&apos;t listed?</b> If the agent can reach it through a CLI or an MCP server,
            CloudBuddy already records it with generic checks. Adding an adapter file makes its cards clearer.
          </p>
        </div>
      </div>
    </section>
  )
}
