// The promo, as a function of time. window.seek(t) draws the frame at t seconds; render.mjs calls it for
// every frame. Nothing animates on its own, so every render of a frame is the same.
import { DURATION, PROMPT, T, linesBy, lineRate } from './timeline.mjs'

// ---------- helpers ----------
const clamp = (x, a = 0, b = 1) => Math.min(b, Math.max(a, x))
const lerp = (a, b, k) => a + (b - a) * k
const inOut = (k) => (k < 0.5 ? 4 * k * k * k : 1 - Math.pow(-2 * k + 2, 3) / 2)
const out3 = (k) => 1 - Math.pow(1 - k, 3)
const back = (k) => 1 + 2.2 * Math.pow(k - 1, 3) + 1.2 * Math.pow(k - 1, 2)
const prog = (t, start, dur) => clamp((t - start) / dur)
/** 0 → 1 over [a, a+fin], 1 → 0 over [b, b+fout]. */
const window_ = (t, a, b, fin = 0.35, fout = 0.35) => Math.min(prog(t, a, fin), 1 - prog(t, b, fout))

const stage = document.getElementById('stage')
function el(tag, cls, parent, html) {
  const e = document.createElement(tag)
  if (cls) e.className = cls
  if (html !== undefined) e.innerHTML = html
  parent.appendChild(e)
  return e
}
const layer = (cls = '') => el('div', `layer ${cls}`, stage)
const show = (e, o) => {
  e.style.opacity = String(o)
  e.style.visibility = o <= 0.001 ? 'hidden' : 'visible'
}

/** Keyframes [t, ...values] → values at t, eased between neighbours. */
function track(keys, t) {
  if (t <= keys[0][0]) return keys[0].slice(1)
  for (let i = 1; i < keys.length; i++) {
    if (t <= keys[i][0]) {
      const [t0, ...a] = keys[i - 1]
      const [t1, ...b] = keys[i]
      const k = inOut((t - t0) / (t1 - t0))
      return a.map((v, j) => lerp(v, b[j], k))
    }
  }
  return keys[keys.length - 1].slice(1)
}

const LOGO = `<svg viewBox="0 0 64 64" width="100%" height="100%">
  <g class="tile"><rect width="64" height="64" rx="15" fill="#E1E8FA"/></g>
  <g transform="translate(0 -1)">
    <g class="cloud"><circle cx="26" cy="33" r="13" fill="#2350C8"/><rect x="26" y="33" width="17" height="13" fill="#2350C8"/></g>
    <g class="buddy"><circle cx="43" cy="38" r="10.5" fill="#E1E8FA"/><circle cx="43" cy="38" r="8" fill="#138A74"/></g>
  </g></svg>`
const GITHUB = `<svg viewBox="0 0 16 16" fill="currentColor"><path d="M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z"/></svg>`

// ---------- layers ----------
const bgGrid = layer('grid')
const bgGlow = layer('glow')

// Scenes 1–3: the agent CLI in a terminal.
const termLayer = layer()
const term = el('div', 'term', termLayer)
el('div', 'term-bar', term, '<span><i></i><i></i><i></i></span><span>agent — zsh</span>')
const termBody = el('div', 'term-body', term)
const termLines = el('div', 'term-lines', termBody)
const POOL = 26
const lineEls = Array.from({ length: POOL }, () => el('p', 'tl', termLines))

const BLOCKS = [
  [['cmd', 'aws sts get-caller-identity'], ['out', '{ "UserId": "AIDA…", "Account": "…", "Arn": "arn:aws:iam::…:user/cli-admin" }']],
  [['cmd', 'aws s3api list-buckets --query "Buckets[].Name"'], ['out', '[ "shipyard-web-staging-old", "shipyard-assets", "shipyard-logs" ]']],
  [['cmd', 'npm run build'], ['out', 'vite building for production…'], ['out', '✓ 42 modules transformed · dist/ 418 kB']],
  [['cmd', 'aws s3api create-bucket --bucket shipyard-web-prod --region us-east-1'], ['out', '{ "Location": "/shipyard-web-prod" }']],
  [['cmd', 'aws s3api put-public-access-block --bucket shipyard-web-prod --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true']],
  [['cmd', 'aws s3 sync ./dist s3://shipyard-web-prod'], ['out', 'upload: dist/index.html to s3://shipyard-web-prod/index.html'], ['out', 'upload: dist/assets/index-3f9c1a.js to s3://shipyard-web-prod/assets/index-3f9c1a.js'], ['out', 'upload: dist/assets/index-8b20de.css to s3://shipyard-web-prod/assets/index-8b20de.css']],
  [['cmd', 'aws cloudfront create-origin-access-control --origin-access-control-config file://oac.json'], ['out', '{ "OriginAccessControl": { "Id": "E1OAC7Q2K9XYZ" } }']],
  [['cmd', 'aws cloudfront create-distribution --distribution-config file://cf.json'], ['out', '{ "Distribution": { "Id": "E2QW8XK4N1ZP7B", "Status": "InProgress", "DomainName": "d3k9x7f2m1q8.cloudfront.net" } }']],
  [['cmd', 'aws s3api put-bucket-policy --bucket shipyard-web-prod --policy file://policy.json']],
  [['cmd', 'stripe products create --name "Shipyard Weekly"'], ['out', '{ "id": "prod_R2xK8sJ1", "object": "product", "livemode": false }']],
  [['cmd', 'stripe prices create --currency usd --unit-amount 800 -d "recurring[interval]=month"'], ['out', '{ "id": "price_1QxR8sKj2LmN7pQ", "object": "price", "livemode": false }']],
  [['cmd', 'curl -s -X POST https://api.cloudflare.com/client/v4/zones/…/dns_records -d \'{"type":"CNAME","name":"cdn",…}\''], ['out', '{ "success": true, "result": { "id": "372e6795…", "type": "CNAME" } }']],
  [['cmd', 'aws cloudfront get-distribution --id E2QW8XK4N1ZP7B --query Distribution.Status'], ['out', '"InProgress"']],
  [['cmd', 'gcloud run services describe newsletter-api --region us-central1 --format json'], ['out', '{ "status": { "url": "https://newsletter-api-…-uc.a.run.app" } }']],
  [['cmd', 'aws iam list-attached-role-policies --role-name shipyard-deploy'], ['out', '{ "AttachedPolicies": [ { "PolicyName": "AmazonS3FullAccess" } ] }']],
  [['cmd', 'aws logs tail /aws/cloudfront/shipyard --since 5m'], ['out', '11:47:02 GET /index.html 200'], ['out', '11:47:03 GET /assets/index-3f9c1a.js 200']],
  [['cmd', 'aws acm list-certificates --region us-east-1'], ['out', '{ "CertificateSummaryList": [ { "DomainName": "shipyard.dev" } ] }']],
]
const FILLER_COUNT = Math.floor(linesBy(T.wallStop - 0.02))
const FILLER = []
for (let i = 0; FILLER.length < FILLER_COUNT; i++) FILLER.push(...BLOCKS[i % BLOCKS.length])
FILLER.length = FILLER_COUNT
const LAST = [['cmd', 'aws s3 rb s3://shipyard-web-staging-old --force'], ['ask', 'Allow this command? [y/N] ']]
const AFTER = [
  ['out', 'remove_bucket: shipyard-web-staging-old'],
  ...BLOCKS.slice(9, 14).flat(),
]

const qLayer = layer()
const QUESTIONS = [
  { text: 'Which account?', x: 150, y: 210, at: 22.7 },
  { text: 'What gets deleted?', x: 1160, y: 190, at: 24.0 },
  { text: 'Can it be undone?', x: 760, y: 860, at: 25.3 },
].map((q) => ({ ...q, e: el('div', 'ask-tag', qLayer, q.text) }))
const pressY = el('div', 'say md', qLayer, 'You press <span class="c">y</span>.')
pressY.style.top = '96px'
const andHope = el('div', 'say md', qLayer, '…and hope.')
andHope.style.top = '176px'
andHope.style.color = 'var(--ink-2)'

// Scene 4: the problem, in words.
const wordsLayer = layer()
const w1 = el('div', 'say', wordsLayer, 'Agent CLIs are great at DevOps.')
w1.style.top = '380px'
const w2 = el('div', 'say', wordsLayer, 'You just <span class="c">can’t see the work.</span>')
w2.style.top = '500px'

// Scenes 5 and 10: the logo.
const brandLayer = layer()
const logo = el('div', 'logo', brandLayer, LOGO)
const tile = logo.querySelector('.tile')
const cloud = logo.querySelector('.cloud')
const buddy = logo.querySelector('.buddy')
const word = el('div', 'wordmark', brandLayer, 'Cloud<span class="b">Buddy</span>')
const tagline = el('div', 'tagline', brandLayer, 'See what your agent does to <span class="grad">your cloud.</span>')
const endline = el('div', 'endline', brandLayer, `<span>runs on 127.0.0.1</span><span>Claude Code + Codex</span><span>${GITHUB} open source · coming soon</span>`)

// Scenes 6–9: the real app, filmed by capture.mjs.
const footLayer = layer()
const cam = el('div', 'cam', footLayer)
const FOOT = [
  [45.8, '002-new-chat'], [47.3, '003-typing'], [47.6, '004-typing'], [47.9, '005-typing'], [48.2, '006-typing'], [48.5, '007-typing'], [48.8, '008-typed'],
  [T.send, '009-run'], [T.firstCard, '010-run'], [52.6, '011-run'], [54.0, '012-run'], [57.8, '016-run'],
  [T.gate, '017-wait2'], [T.approved, '019-approved2'], [T.stripe, '020-wait3'],
  ...Array.from({ length: 9 }, (_, i) => [T.replay + i * 0.62, `${String(28 + i).padStart(3, '0')}-replay`]),
].map(([at, name]) => ({ at, img: el('img', '', cam) , name }))
FOOT.forEach((f) => (f.img.src = `footage/${f.name}.png`))
const ring = el('div', 'ring', cam)
// Footage coordinates are the app's own pixels (1600 × 900). z = 1 fills the frame.
const CAM = [
  [45.8, 800, 450, 0.84], [47.0, 800, 450, 0.84], [47.9, 516, 610, 1.55], [49.4, 516, 610, 1.55], [50.6, 800, 450, 1.0],
  [52.4, 1100, 450, 1.22], [55.2, 1120, 450, 1.22], [57.0, 900, 450, 1.0], [59.4, 900, 450, 1.0],
  [60.8, 1210, 480, 1.6], [70.6, 1222, 486, 1.6], [72.6, 1300, 400, 1.42], [74.8, 1300, 400, 1.42],
  [76.2, 1000, 590, 1.6], [78.7, 1006, 596, 1.6], [79.6, 800, 450, 1.0], [84.8, 1040, 520, 1.05],
]
const cursor = el('div', 'cursor', stage, `<svg viewBox="0 0 34 44" width="34" height="44"><path d="M4 3 L4 35 L12 27.5 L17.5 40 L23 37.6 L17.6 25.4 L28.5 25.4 Z" fill="#fff" stroke="#000" stroke-width="2" stroke-linejoin="round"/></svg>`)
const ripple = el('div', 'ripple', stage)

// Captions.
const capLayer = layer()
const CAPS = [
  [8.6, 12.3, 'Dozens of commands.'],
  [12.5, 16.2, 'AWS, Stripe, Cloudflare, Google Cloud…'],
  [16.4, 20.8, 'Scrolling <span class="c">faster than you can read</span>.'],
  [46.4, 49.4, 'CloudBuddy runs the agent CLI <span class="t">you already use</span>.', 'top'],
  [50.2, 54.4, 'Every cloud step becomes a <span class="t">card</span>.'],
  [54.6, 58.9, 'One lane per service. Left to right, in order.'],
  [61.0, 65.2, 'Anything you <span class="c">can’t undo</span> stops and waits.'],
  [65.4, 70.4, 'What will be lost. The agent’s reason. <span class="t">Your call.</span>'],
  [71.7, 74.8, 'Approved by you, and <span class="t">on the record</span>.'],
  [75.5, 78.8, 'Live payments and real emails <span class="c">wait for you</span> too.'],
  [79.5, 84.5, 'Every chat keeps its canvas. <span class="t">Replay it</span>, step by step.', 'top'],
].map(([a, b, html, pos]) => {
  const e = el('div', 'cap', capLayer, html)
  if (pos === 'top') Object.assign(e.style, { bottom: 'auto', top: '56px' })
  if (a < 30) Object.assign(e.style, { bottom: 'auto', top: '470px', fontSize: '64px', padding: '28px 52px' })
  return { a, b, e, top: pos === 'top' || a < 30 }
})

const black = layer('black')

// ---------- drawing ----------
function drawTerminal(t) {
  const o = Math.min(prog(t, 0.2, 0.6), 1 - prog(t, T.termOut, 0.6))
  show(termLayer, o)
  drawQuestions(t)
  if (o <= 0) return

  // Where the window sits: still, then filling the screen, then zoomed onto the y/N prompt.
  const [x, y, s] = track(
    [[0, 370, 200, 1], [7.2, 370, 200, 1], [12.2, 45, 13, 1.55], [21.2, 45, 13, 1.55], [22.6, 70, -854, 2.3], [29.3, 70, -854, 2.3], [30.4, 45, 13, 1.55]],
    t,
  )
  term.style.transform = `translate(${x}px, ${y}px) scale(${s})`

  const lines = []
  const typed = Math.floor(prog(t, T.promptType[0], T.promptType[1] - T.promptType[0]) * PROMPT.length)
  lines.push(['me', PROMPT.slice(0, typed), t < T.wallStart])
  const n = Math.floor(linesBy(t))
  lines.push(...FILLER.slice(0, n))
  if (t >= T.wallStop) {
    lines.push(LAST[0])
    const yTyped = t >= T.pressY
    lines.push(['ask', LAST[1][1] + (yTyped ? 'y' : ''), !yTyped || t < T.resume])
    if (t >= T.resume) lines.push(...AFTER.slice(0, Math.floor((t - T.resume) * 9)))
  }
  const shown = lines.slice(-POOL)
  const focus = t >= T.wallStop + 0.4 && t < T.resume + 0.3
  lineEls.forEach((p, i) => {
    const line = shown[i - (POOL - shown.length)]
    if (!line) {
      p.textContent = ''
      p.className = 'tl'
      return
    }
    const [kind, text, caret] = line
    p.className = `tl ${kind}`
    p.textContent = text
    if (caret && Math.floor(t * 2) % 2 === 0) el('span', 'caret', p)
    const last = i >= POOL - 2
    p.style.opacity = focus && !last ? '0.22' : '1'
  })
  const rate = lineRate(t)
  termLines.style.filter = rate > 16 ? `blur(${clamp((rate - 16) / 30) * 1.3}px)` : 'none'

}

function drawQuestions(t) {
  for (const q of QUESTIONS) {
    const k = window_(t, q.at, 27.0, 0.35, 0.4)
    show(q.e, k)
    q.e.style.left = `${q.x}px`
    q.e.style.top = `${q.y}px`
    q.e.style.transform = `scale(${lerp(0.85, 1, back(prog(t, q.at, 0.35)))})`
  }
  show(pressY, window_(t, 27.4, 31.6, 0.3, 0.4))
  show(andHope, window_(t, 29.0, 31.6, 0.4, 0.4))
}

function drawWords(t) {
  show(wordsLayer, t > T.problem - 0.1 && t < 40.2 ? 1 : 0)
  for (const [e, at] of [[w1, T.problem], [w2, T.problem2]]) {
    const k = window_(t, at, 39.2, 0.6, 0.6)
    show(e, k)
    e.style.transform = `translateY(${lerp(24, 0, out3(prog(t, at, 0.6)))}px)`
  }
}

function drawBrand(t) {
  const reveal = t >= T.reveal - 0.1 && t < T.product + 0.6
  const end = t >= T.end - 0.1
  show(brandLayer, reveal || end ? 1 : 0)
  if (!reveal && !end) return
  const at = reveal ? T.reveal : T.end
  const out = reveal ? 1 - prog(t, T.product - 0.3, 0.6) : 1
  const big = reveal ? 1 : 0.8
  logo.style.transform = `translateY(${reveal ? 0 : -40}px) scale(${big * lerp(0.4, 1, back(prog(t, at, 0.5)))})`
  show(logo, Math.min(prog(t, at, 0.3), out))
  const sc = (g, cx, cy, k) => g.setAttribute('transform', `translate(${cx} ${cy}) scale(${k}) translate(${-cx} ${-cy})`)
  sc(tile, 32, 32, 1)
  sc(cloud, 30, 37, lerp(0.2, 1, back(prog(t, at + 0.25, 0.45))))
  sc(buddy, 43, 38, lerp(0, 1, back(prog(t, at + 0.6, 0.4))))
  const wk = Math.min(prog(t, at + 0.9, 0.5), out)
  show(word, wk)
  word.style.transform = `translateY(${lerp(20, 0, out3(prog(t, at + 0.9, 0.5))) + (reveal ? 0 : -60)}px) scale(${reveal ? 1 : 0.85})`
  const tk = Math.min(prog(t, at + 1.7, 0.6), out)
  show(tagline, tk)
  tagline.style.transform = `translateY(${reveal ? 0 : -90}px)`
  show(endline, end ? prog(t, at + 2.3, 0.6) : 0)
  endline.style.transform = `translateY(${-90}px)`
}

let S = 1.2
let TX = 0
let TY = 0
const toScreen = (x, y) => [TX + x * S, TY + y * S]

function drawFootage(t) {
  const o = Math.min(prog(t, T.product - 0.2, 0.6), 1 - prog(t, T.end - 0.6, 0.6))
  show(footLayer, o)
  if (o <= 0) {
    show(cursor, 0)
    show(ripple, 0)
    return
  }
  const [cx, cy, z] = track(CAM, t)
  S = 1.2 * z
  const w = 1600 * S
  const h = 900 * S
  TX = z >= 1 ? clamp(960 - cx * S, 1920 - w, 0) : (1920 - w) / 2
  TY = z >= 1 ? clamp(540 - cy * S, 1080 - h, 0) : (1080 - h) / 2
  cam.style.transform = `translate(${TX}px, ${TY}px) scale(${S})`
  const framed = clamp((1 - z) / 0.16)
  cam.style.borderRadius = `${(18 * framed) / S}px`
  cam.style.boxShadow = framed > 0 ? `0 0 0 ${1 / S}px rgba(255,255,255,.08), 0 ${40 / S}px ${120 / S}px ${-30 / S}px #000` : 'none'

  FOOT.forEach((f, i) => {
    const next = FOOT[i + 1]
    const k = next && t >= next.at + 0.25 ? 0 : prog(t, f.at, i === 0 ? 0.01 : 0.22)
    show(f.img, k)
  })

  // The "what will be lost" box, ringed while the caption names it.
  const rk = window_(t, 66.0, 70.2, 0.3, 0.3)
  show(ring, rk)
  Object.assign(ring.style, { left: '1100px', top: '424px', width: '228px', height: '94px', borderWidth: `${3 / S}px`, borderRadius: `${8}px` })

  // The pointer goes to Delete and clicks it.
  const ck = window_(t, 68.8, 72.0, 0.3, 0.4)
  show(cursor, ck)
  if (ck > 0) {
    const [bx, by] = toScreen(1267, 579)
    const m = inOut(prog(t, 69.0, 1.7))
    const px = lerp(1640, bx, m)
    const py = lerp(980, by, m) + Math.sin(m * Math.PI) * -60
    const press = t > T.click - 0.08 && t < T.click + 0.12 ? 0.86 : 1
    cursor.style.transform = `translate(${px - 4}px, ${py - 4}px) scale(${press})`
  }
  const rp = prog(t, T.click, 0.5)
  show(ripple, rp > 0 && rp < 1 ? 1 - rp : 0)
  if (rp > 0 && rp < 1) {
    const [bx, by] = toScreen(1267, 579)
    ripple.style.transform = `translate(${bx}px, ${by}px) scale(${lerp(1, 5, out3(rp))})`
  }
}

function drawCaptions(t) {
  for (const c of CAPS) {
    const k = window_(t, c.a, c.b, 0.3, 0.3)
    show(c.e, k)
    c.e.style.transform = `translateX(-50%) translateY(${lerp(16, 0, out3(prog(t, c.a, 0.35)))}px)`
  }
}

window.seek = (t) => {
  t = clamp(t, 0, DURATION)
  show(bgGlow, Math.max(window_(t, T.reveal - 0.4, T.product, 0.8, 0.8), window_(t, T.end - 0.4, 99, 0.8, 0.1)))
  drawTerminal(t)
  drawWords(t)
  drawBrand(t)
  drawFootage(t)
  drawCaptions(t)
  // From black at the start, to black at the end.
  show(black, Math.max(1 - prog(t, 0, 0.5), prog(t, DURATION - 0.9, 0.85)))
}

// Ready once the fonts and every footage frame have loaded.
window.ready = Promise.all([
  document.fonts.ready,
  ...['700 20px "Bricolage Grotesque"', '600 20px "Bricolage Grotesque"', '400 20px "IBM Plex Sans"', '500 20px "IBM Plex Sans"', '400 20px "IBM Plex Mono"', '500 20px "IBM Plex Mono"'].map((f) => document.fonts.load(f)),
  ...FOOT.map((f) => f.img.decode().catch(() => {})),
]).then(() => true)

const start = Number(new URLSearchParams(location.search).get('t') ?? 0)
window.seek(start)
