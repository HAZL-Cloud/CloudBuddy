import './landing.css'

import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon, type IconName } from '../components/Icon'
import { Logo, Wordmark } from '../components/Logo'
import { APP_PATH } from '../route'
import { GITHUB_URL } from './links'

// The landing page at / (D-060): the project's front page, for developers. No sign-in anywhere (D-063):
// Open CloudBuddy goes straight to the app.
export function Landing({ github = GITHUB_URL }: { github?: string }) {
  useEffect(() => {
    document.title = 'CloudBuddy · See what your agent does to your cloud'
  }, [])

  return (
    <div className="landing">
      <a className="l-skip" href="#main">
        Skip to content
      </a>
      <header className="l-nav">
        <div className="l-wrap l-nav-in">
          <a className="l-brand" href="/">
            <Logo className="l-mark" />
            <span className="l-word">
              <Wordmark />
            </span>
          </a>
          <nav className="l-links" aria-label="On this page">
            <a href="#problem">Why</a>
            <a href="#features">Features</a>
            <a href="#gate">How it works</a>
            <a href="#start">Quick start</a>
          </nav>
          <div className="l-nav-cta">
            <GitHubButton url={github} />
            <OpenButton />
          </div>
        </div>
      </header>

      <main id="main">
        <Hero github={github} />
        <Problem />
        <Features />
        <Gate />
        <QuickStart github={github} />
        <Closing github={github} />
      </main>

      <footer className="l-foot">
        <div className="l-wrap l-foot-in">
          <span className="l-foot-brand">
            <Logo className="l-mark sm" />
            <span>
              <Wordmark />
            </span>
          </span>
          <span>Apache-2.0</span>
          <span>v0.1 · macOS · Claude Code and Codex</span>
          <span className="mono">127.0.0.1:7420</span>
        </div>
      </footer>
    </div>
  )
}

function OpenButton({ big = false }: { big?: boolean }) {
  return (
    <a className={`l-btn primary ${big ? 'big' : ''}`} href={APP_PATH}>
      Open CloudBuddy
      <Arrow />
    </a>
  )
}

/** Links to the repository once it has a URL; until then it is shown, marked Soon, and does nothing. */
export function GitHubButton({ url, big = false }: { url: string; big?: boolean }) {
  const cls = `l-btn ghost ${big ? 'big' : ''}`
  if (url) {
    return (
      <a className={cls} href={url} target="_blank" rel="noopener noreferrer">
        <GitHubMark />
        GitHub
      </a>
    )
  }
  return (
    <span className={`${cls} is-soon`} role="link" aria-disabled="true" aria-label="GitHub, coming soon" title="The repository link is coming soon">
      <GitHubMark />
      GitHub
      <span className="l-soon">Soon</span>
    </span>
  )
}

function Hero({ github }: { github: string }) {
  return (
    <section className="l-hero" aria-labelledby="hero-title">
      <div className="l-wrap l-hero-copy">
        <p className="l-eyebrow mono">
          <span className="l-live" aria-hidden="true" />
          runs on 127.0.0.1 · Claude Code + Codex · open source
        </p>
        <h1 id="hero-title">
          See what your agent does to <span className="l-grad">your cloud.</span>
        </h1>
        <p className="l-lede">
          CloudBuddy runs the Claude Code or Codex CLI already on your machine and draws every cloud step as a card on a
          canvas. Anything you can't undo waits for your OK.
        </p>
        <div className="l-ctas">
          <OpenButton big />
          <GitHubButton url={github} big />
        </div>
        <p className="l-fine">Local only. Uses your existing CLI logins. Never stores your keys.</p>
      </div>
      <div className="l-wrap wide">
        <SplitDemo />
      </div>
    </section>
  )
}

// ---------- The hero picture: one deploy, in a terminal and on the canvas ----------

type Line = { k: 'me' | 'cmd' | 'out' | 'ask'; t: string }
const TERMINAL: Line[] = [
  { k: 'me', t: '$ agent "deploy the shop site to S3 + CloudFront, then delete the old staging bucket"' },
  { k: 'cmd', t: 'aws sts get-caller-identity' },
  { k: 'out', t: '{ "UserId": "AIDA…", "Arn": "arn:aws:iam::…:user/cli-admin" }' },
  { k: 'cmd', t: 'aws s3api list-buckets --query "Buckets[].Name"' },
  { k: 'out', t: '[ "shipyard-web-staging-old", "shipyard-assets", "shipyard-logs", … ]' },
  { k: 'cmd', t: 'aws s3api create-bucket --bucket shipyard-web-prod --region us-east-1' },
  { k: 'out', t: '{ "Location": "/shipyard-web-prod" }' },
  { k: 'cmd', t: 'aws s3api put-public-access-block --bucket shipyard-web-prod --public-access-block-configuration BlockPublicAcls=true,…' },
  { k: 'cmd', t: 'npm run build' },
  { k: 'out', t: '✓ built in 4.1s' },
  { k: 'cmd', t: 'aws s3 sync ./dist s3://shipyard-web-prod --delete' },
  { k: 'out', t: 'upload: dist/index.html to s3://shipyard-web-prod/index.html' },
  { k: 'out', t: 'upload: dist/assets/index-3f9c1a.js to s3://shipyard-web-prod/assets/index-3f9c1a.js' },
  { k: 'cmd', t: 'aws cloudfront create-distribution --distribution-config file://cf.json' },
  { k: 'out', t: '{ "Distribution": { "Id": "E2QW8XK4N1ZP7B", "Status": "InProgress", … } }' },
  { k: 'cmd', t: 'aws s3api put-bucket-policy --bucket shipyard-web-prod --policy file://oac.json' },
  { k: 'cmd', t: 'aws s3 rb s3://shipyard-web-staging-old --force' },
  { k: 'ask', t: 'Allow this command? [y/N]' },
]

function SplitDemo() {
  return (
    <figure className="l-split">
      <div className="l-pane term">
        <div className="l-pane-bar">
          <span className="l-dots" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span className="mono">agent — zsh</span>
        </div>
        <div className="l-term-body" aria-label="An agent CLI deploying a site, as terminal text" role="img">
          <div className="l-term-lines" aria-hidden="true">
            {TERMINAL.map((line, i) => (
              <p key={i} className={`l-tl ${line.k}`} style={{ animationDelay: `${0.15 + i * 0.09}s` }}>
                {line.k === 'cmd' && <span className="l-bullet">●</span>}
                {line.t}
                {line.k === 'ask' && <span className="l-cursor" />}
              </p>
            ))}
          </div>
        </div>
        <p className="l-pane-label">
          <b>Agent CLI.</b> Nine commands, one y/N, and no word on what <span className="mono">--force</span> removes.
        </p>
      </div>
      <div className="l-pane canvas">
        <div className="l-pane-bar">
          <span className="l-bar-title">
            Steps <span className="l-bar-sub">AWS · us-east-1</span>
          </span>
          <span className="l-tally" aria-hidden="true">
            <span className="risk-read">
              <Icon name="eye" small />2
            </span>
            <span className="risk-create">
              <Icon name="plus" small />3
            </span>
            <span className="risk-delete on">
              <Icon name="trash" small />1 waiting
            </span>
          </span>
        </div>
        <MiniCanvas />
        <p className="l-pane-label">
          <b>CloudBuddy.</b> The same run as a map, and it stops before the delete.
        </p>
      </div>
      <figcaption className="sr-only">The same deploy, shown as terminal output and as the CloudBuddy canvas.</figcaption>
    </figure>
  )
}

type Card = { id: string; action: 'read' | 'create' | 'delete'; step: string; title: string; res: string; x: number; y: number; chip?: string }
const CARDS: Card[] = [
  { id: 'c1', action: 'read', step: 'Steps 1–2', title: 'Checked the account', res: 'cli-admin · 4 buckets', x: 13.5, y: 8.5 },
  { id: 'c2', action: 'create', step: 'Step 3', title: 'Created bucket', res: 'shipyard-web-prod', x: 29.5, y: 25.5 },
  { id: 'c3', action: 'create', step: 'Step 4', title: 'Uploaded the built site', res: '42 files', x: 45.5, y: 25.5 },
  { id: 'c4', action: 'create', step: 'Step 5', title: 'Created a distribution', res: 'E2QW8XK4N1ZP7B', x: 61.5, y: 49, chip: 'from 3' },
]
const LANES = [
  { name: 'Account', top: 7, h: 15 },
  { name: 'S3', top: 23.5, h: 22 },
  { name: 'CloudFront', top: 47.5, h: 15 },
]
// Step numbers over the middle of each card's column.
const TICKS: [string, number][] = [['1–2', 21], ['3', 37], ['4', 53], ['5', 69], ['6', 88.75]]
const ACTION_ICON: Record<Card['action'], IconName> = { read: 'eye', create: 'plus', delete: 'trash' }

function MiniCanvas() {
  const scroller = useRef<HTMLDivElement>(null)
  // On a phone the picture scrolls sideways; start at the right end, where the approval waits.
  useEffect(() => {
    const el = scroller.current
    if (el) el.scrollLeft = el.scrollWidth
  }, [])

  return (
    <div className="l-mini-scroll" ref={scroller}>
      <div
        className="l-mini"
        role="img"
        aria-label="The CloudBuddy canvas: lanes for Account, S3 and CloudFront, a card for each step, and a delete waiting for approval"
      >
        <div className="l-stage" aria-hidden="true">
          <div className="l-ruler">
            <span className="l-ruler-label">STEP</span>
            {TICKS.map(([n, x]) => (
              <span key={n} className="l-tick" style={{ left: `${x}cqw` }}>
                {n}
              </span>
            ))}
          </div>
          {LANES.map((l) => (
            <div key={l.name} className="l-lane" style={{ top: `${l.top}cqw`, height: `${l.h}cqw` }}>
              <span className="l-lane-name">
                {l.name} <small>AWS</small>
              </span>
            </div>
          ))}
          <svg className="l-wires" viewBox="0 0 100 68.75" preserveAspectRatio="none">
            {/* From the bucket (step 3) to the upload and the distribution that use it */}
            <path d="M37 37 V41.5 H69 V49 M53 41.5 V37" />
          </svg>
          {CARDS.map((c, i) => (
            <div
              key={c.id}
              className={`l-card risk-${c.action} ${c.id === 'c1' ? 'stacked' : ''}`}
              style={{ left: `${c.x}cqw`, top: `${c.y}cqw`, animationDelay: `${0.5 + i * 0.25}s` }}
            >
              <span className="l-chiprow">
                <span className="l-act">
                  <Icon name={ACTION_ICON[c.action]} small />
                  {c.action}
                </span>
                {c.step}
              </span>
              <b>{c.title}</b>
              <span className="l-resrow">
                <span className="l-res mono">{c.res}</span>
                {c.chip && <span className="l-from">{c.chip}</span>}
              </span>
            </div>
          ))}
          <div className="l-gate risk-delete">
            <div className="l-gate-head">
              <Icon name="hand" small />
              Waiting for your approval
            </div>
            <div className="l-gate-body">
              <span className="l-chiprow">
                <span className="l-act">
                  <Icon name="trash" small />
                  delete
                </span>
                Step 6
              </span>
              <b>Delete bucket shipyard-web-staging-old</b>
              <span className="l-tag">
                <Icon name="lock" small />
                Can't undo
              </span>
              <div className="l-lost">
                <span>What will be lost</span>
                <ul>
                  <li>318 files in the bucket</li>
                  <li>CloudBuddy can't undo this</li>
                </ul>
              </div>
              <div className="l-gate-btns">
                <span>Keep it</span>
                <span className="go">Delete</span>
              </div>
            </div>
          </div>
          <div className="l-replay">
            <span className="l-replay-btn">
              <Icon name="play" small />
              Replay
            </span>
            <span className="l-seg read" />
            <span className="l-seg create" />
            <span className="l-seg create" />
            <span className="l-seg create" />
            <span className="l-seg wait" />
            <span className="l-replay-at mono">Step 6 of 6</span>
          </div>
        </div>
      </div>
    </div>
  )
}

// ---------- Sections ----------

function Section({ id, kicker, title, lede, children }: { id: string; kicker: string; title: ReactNode; lede?: ReactNode; children: ReactNode }) {
  return (
    <section className="l-sec" id={id} aria-labelledby={`${id}-title`}>
      <div className="l-wrap">
        <p className="l-kicker mono">{kicker}</p>
        <h2 id={`${id}-title`}>{title}</h2>
        {lede && <p className="l-sec-lede">{lede}</p>}
        {children}
      </div>
    </section>
  )
}

function Problem() {
  return (
    <Section
      id="problem"
      kicker="// the problem"
      title="Agent CLIs are great at DevOps. Watching them do it isn't."
      lede="Ask an agent to deploy a site, set up payments or add DNS records, and it gets there. What you see on the way is a terminal scrolling past."
    >
      <div className="l-grid3">
        <article className="l-pain">
          <span className="l-pain-ic mono" aria-hidden="true">
            $_
          </span>
          <h3>A wall of commands</h3>
          <p>Dozens of aws, wrangler and stripe calls with JSON in between. They scroll by faster than you can read them.</p>
        </article>
        <article className="l-pain">
          <span className="l-pain-ic mono" aria-hidden="true">
            y/N
          </span>
          <h3>Approvals with no context</h3>
          <p>A permission prompt shows one command line. Not which account it hits, what gets lost, or whether it can be undone.</p>
        </article>
        <article className="l-pain">
          <span className="l-pain-ic mono" aria-hidden="true">
            ↑↑↑
          </span>
          <h3>No map afterwards</h3>
          <p>When something breaks, the only record is the scrollback: command syntax, top to bottom, in one long chat.</p>
        </article>
      </div>
    </Section>
  )
}

function Features() {
  return (
    <Section
      id="features"
      kicker="// what CloudBuddy adds"
      title="A canvas for every run, and a stop before anything you can't undo."
      lede="Chat with your agent like you already do. CloudBuddy sits between the agent and your cloud and shows you the work."
    >
      <div className="l-feats">
        <article className="l-feat">
          <div className="l-feat-art">
            {(['read', 'create', 'change', 'send', 'delete'] as const).map((a) => (
              <span key={a} className={`l-pill risk-${a}`}>
                {a}
              </span>
            ))}
          </div>
          <h3>Every step is a card</h3>
          <p>One card per cloud step, left to right in order, one lane per service. The colour says what kind of step it was. Back-to-back reads stack into one card.</p>
        </article>
        <article className="l-feat">
          <div className="l-feat-art">
            <span className="l-pill risk-delete">
              <Icon name="lock" small />
              Can't undo
            </span>
            <span className="l-pill risk-change">
              <Icon name="dollar" small />
              Moves money
            </span>
            <span className="l-pill risk-send">
              <Icon name="people" small />
              Reaches people
            </span>
            <span className="l-pill risk-create">
              <Icon name="bolt" small />
              Live
            </span>
          </div>
          <h3>Approvals with the whole story</h3>
          <p>Deletes, payments, sends to real people and live-mode changes stop and wait. You see what will be lost and the agent's reason, then decide on the canvas.</p>
        </article>
        <article className="l-feat">
          <div className="l-feat-art lines" aria-hidden="true">
            <span className="l-node">bucket</span>
            <svg viewBox="0 0 60 24">
              <path d="M0 12 H24 M24 12 V4 H60 M24 12 V20 H60" />
            </svg>
            <span className="l-node-col">
              <span className="l-node">upload</span>
              <span className="l-node">CDN</span>
            </span>
          </div>
          <h3>Lines that mean something</h3>
          <p>A line runs from something a step made to each later step that uses it. A value found earlier is a chip you can hover to trace.</p>
        </article>
        <article className="l-feat">
          <div className="l-feat-art replay" aria-hidden="true">
            <span className="l-replay-btn">
              <Icon name="play" small />
            </span>
            <span className="l-seg read" />
            <span className="l-seg create" />
            <span className="l-seg change" />
            <span className="l-seg create" />
            <span className="l-seg delete" />
          </div>
          <h3>Replay any chat</h3>
          <p>Every chat keeps its canvas. Open it again next week and step through exactly what happened, in order.</p>
        </article>
        <article className="l-feat">
          <div className="l-feat-art rules" aria-hidden="true">
            <span>
              <i className="mono">stripe</i> Ask before every change
            </span>
            <span>
              <i className="mono">aws</i> Read-only
            </span>
            <span>
              <i className="mono">resend</i> Off
            </span>
          </div>
          <h3>Connections, with rules</h3>
          <p>See which CLIs and MCP servers the agent can reach and who they are signed in as. Give each one a rule: Off, Read-only, Ask before every change.</p>
        </article>
        <article className="l-feat">
          <div className="l-feat-art folders" aria-hidden="true">
            <span>
              <Icon name="folder" small />
              <i className="mono">~/code/shop-web</i>
            </span>
            <span>
              <Icon name="folder" small />
              <i className="mono">~/code/shop-infra</i>
            </span>
          </div>
          <h3>Projects</h3>
          <p>Group the folders that belong together, like an app and its infrastructure code. A chat in a project starts with all of them.</p>
        </article>
      </div>
    </Section>
  )
}

const PIPELINE: { name: string; does: string }[] = [
  { name: 'tool call', does: 'The agent wants to run a command or call an MCP tool.' },
  { name: 'recognizer', does: 'Works out the provider, the action, who is signed in, and warning tags.' },
  { name: 'policy', does: 'Decides: run it, ask you, or refuse.' },
  { name: 'you', does: 'Approve or deny, with what will be lost in front of you.' },
  { name: 'runs once', does: 'The approved call runs, once. Your yes does not carry over to the next call.' },
]

function Gate() {
  return (
    <Section
      id="gate"
      kicker="// how it works"
      title="Every tool call goes through the gate."
      lede="CloudBuddy starts the agent CLI with its own settings. Before any tool call runs, it works out what the call does and decides what happens next."
    >
      <ol className="l-pipe">
        {PIPELINE.map((p, i) => (
          <li key={p.name} className={p.name === 'you' ? 'you' : ''}>
            <span className="l-pipe-n mono">{String(i + 1).padStart(2, '0')}</span>
            <b className="mono">{p.name}</b>
            <span>{p.does}</span>
          </li>
        ))}
      </ol>
      <div className="l-arch">
        <pre className="mono" aria-label="Architecture: the browser talks to CloudBuddy on 127.0.0.1, which runs the claude or codex CLI, which reaches your cloud CLIs and MCP servers through the gate">
          {`browser ── CloudBuddy (Go, 127.0.0.1:7420) ── claude / codex CLI ── your cloud CLIs + MCP servers
                │
           gate: recognizer → policy → ask you → release once`}
        </pre>
      </div>
      <ul className="l-facts">
        <li>
          <Icon name="shield" />
          <span>
            <b>Fails closed.</b> If CloudBuddy can't decide, can't reach you, or crashes, the action doesn't run.
          </span>
        </li>
        <li>
          <Icon name="lock" />
          <span>
            <b>No network for local work.</b> Editing files and building run in a sandbox with no network.
          </span>
        </li>
        <li>
          <Icon name="plug" />
          <span>
            <b>MCP through a gateway.</b> MCP servers are reached only through CloudBuddy's gateway, which checks every call.
          </span>
        </li>
        <li>
          <Icon name="bot" />
          <span>
            <b>Your logins stay yours.</b> It runs on 127.0.0.1, uses the agent and cloud CLIs you already signed in to, and never stores your keys.
          </span>
        </li>
      </ul>
      <div className="l-works">
        <div>
          <span className="l-works-k mono">agents</span>
          <span className="l-chip">Claude Code</span>
          <span className="l-chip">Codex</span>
        </div>
        <div>
          <span className="l-works-k mono">knows</span>
          <span className="l-chip">AWS · every service</span>
          <span className="l-chip">Stripe</span>
          <span className="l-chip">Cloudflare</span>
          <span className="l-chip">Resend</span>
        </div>
        <p>
          Anything else with a CLI or an MCP server works too: CloudBuddy guesses from the command and asks unless it's clearly read-only. Adapters are YAML
          files.
        </p>
      </div>
      <p className="l-limits">
        <b>What it can't promise.</b> CloudBuddy can't see inside code you approve to run with network, like an install script or{' '}
        <code>cdk deploy</code>, and it can't stop cloud changes made outside the agent. Read <code>docs/security.md</code> before you point it at production.
      </p>
    </Section>
  )
}

function QuickStart({ github }: { github: string }) {
  const clone = `git clone ${github || '<repo-url>'} cloudbuddy && cd cloudbuddy`
  const lines: [string, string][] = [
    [clone, ''],
    ['make db-up', 'PostgreSQL in Docker on 127.0.0.1:54329'],
    ['make release', 'the frontend and one binary: bin/cloudbuddy'],
    ['bin/cloudbuddy', 'opens http://127.0.0.1:7420 in your browser'],
  ]
  return (
    <Section
      id="start"
      kicker="// quick start"
      title="Clone, build, run."
      lede={
        <>
          You need Go 1.24+, Node 20+, Docker, and Claude Code or Codex installed and signed in. It works on macOS today; Linux runs with limits;
          Windows isn't supported yet.
        </>
      }
    >
      <div className="l-shell">
        <div className="l-pane-bar">
          <span className="l-dots" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span className="mono">zsh</span>
          <CopyButton text={lines.map(([cmd]) => cmd).join('\n')} />
        </div>
        <pre className="mono" tabIndex={0}>
          {lines.map(([cmd, note]) => (
            <span key={cmd} className="l-shell-line">
              <span className="l-prompt" aria-hidden="true">
                ${' '}
              </span>
              {cmd}
              {note && <span className="l-comment">{`  # ${note}`}</span>}
              {'\n'}
            </span>
          ))}
        </pre>
      </div>
      <p className="l-try">
        <b>No agent handy?</b> <code>bin/cloudbuddy --fake testdata/scenarios/demo.json</code> replays a deploy, so you can try approvals without touching
        a cloud account.
      </p>
    </Section>
  )
}

function CopyButton({ text }: { text: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])

  const show = (next: 'copied' | 'failed') => {
    setState(next)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setState('idle'), 1800)
  }
  const copy = () => {
    if (!navigator.clipboard) return show('failed')
    navigator.clipboard.writeText(text).then(
      () => show('copied'),
      () => show('failed'),
    )
  }
  return (
    <button type="button" className="l-copy" onClick={copy}>
      <Icon name={state === 'copied' ? 'check' : 'copy'} small />
      {state === 'copied' ? 'Copied' : state === 'failed' ? 'Select and copy' : 'Copy'}
    </button>
  )
}

function Closing({ github }: { github: string }) {
  return (
    <section className="l-close" aria-labelledby="close-title">
      <div className="l-wrap l-close-in">
        <Logo className="l-mark big" />
        <h2 id="close-title">Your agent does the work. You see every step.</h2>
        <div className="l-ctas">
          <OpenButton big />
          <GitHubButton url={github} big />
        </div>
      </div>
    </section>
  )
}

function Arrow() {
  return (
    <svg className="ic" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M3 8h10M9 4l4 4-4 4" />
    </svg>
  )
}

// The GitHub mark from Octicons (MIT).
function GitHubMark() {
  return (
    <svg className="ic" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z" />
    </svg>
  )
}
