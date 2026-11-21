// The Dashboards demo, as a function of time. window.seek(t) draws the frame at t seconds; render.mjs
// (--cut dash) calls it for every frame. Nothing animates on its own, so every render of a frame is the same.
import { DURATION, T } from './timeline.mjs'

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

// Where capture.mjs found the buttons the pointer clicks, in the app's pixels.
const manifest = await (await fetch('footage/manifest.json')).json()
const rectOf = (label, name) => manifest.find((f) => f.label === label)?.rects?.[name]
const centre = (r) => [r[0] + r[2] / 2, r[1] + r[3] / 2]
const OPEN = centre(rectOf('saved', 'open'))
const RANGE30 = centre(rectOf('dash', 'range30'))
const ALLOW = centre(rectOf('dash', 'allow'))
const frame = (label) => manifest.filter((f) => f.label === label).map((f) => f.file.replace('.png', ''))

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

// Scene 1: the same question, three mornings.
const hookLayer = layer()
const hookA = el('div', 'hook-title', hookLayer, 'Every morning, the same question.')
const hookB = el('div', 'hook-title', hookLayer, 'Your agent runs the same queries. <span class="c">Every time.</span>')
const MORNINGS = ['Mon 9:02', 'Tue 9:05', 'Wed 8:58'].map((day, i) => {
  const row = el('div', 'morning', hookLayer, `<span class="day">${day}</span><span class="bubble">How many users signed up yesterday?</span>`)
  row.style.top = `${410 + i * 190}px`
  const agent = el('div', 'agent', row, '↳ <b>3 queries</b>, run again from scratch')
  return { row, agent, at: T.ask[i] }
})

// Scenes 2 and 8: the logo.
const brandLayer = layer()
const kicker = el('div', 'kicker', brandLayer, 'New in CloudBuddy · Dashboards')
const logo = el('div', 'logo', brandLayer, LOGO)
const tile = logo.querySelector('.tile')
const cloud = logo.querySelector('.cloud')
const buddy = logo.querySelector('.buddy')
const word = el('div', 'wordmark', brandLayer, 'Cloud<span class="b">Buddy</span>')
const tagline = el('div', 'tagline', brandLayer, 'Ask once. <span class="grad">It keeps watching.</span>')
const endline = el('div', 'endline', brandLayer, `<span>runs on 127.0.0.1</span><span>no agent · no tokens</span><span>${GITHUB} open source</span>`)

// Scenes 3–7: the real app, filmed by capture.mjs.
const footLayer = layer()
const cam = el('div', 'cam', footLayer)
const typing = frame('typing')
const runs = frame('run')
const FOOT = [
  [T.product, frame('new-chat')[0]],
  ...typing.map((f, i) => [T.typing[0] + i * 0.4, f]),
  [T.typing[1] - 0.1, frame('typed')[0]],
  ...runs.map((f, i) => [T.send + 0.3 + i * ((T.saved + 0.9 - T.send) / runs.length), f]),
  [T.saved + 1.1, frame('saved')[0]],
  [T.dash, frame('dash')[0]],
  [T.rangeClick + 0.25, frame('dash-30d')[0]],
  [T.allowed, frame('allowed')[0]],
  [T.sources, frame('sources')[0]],
].map(([at, name]) => ({ at, name, img: el('img', '', cam) }))
FOOT.forEach((f) => (f.img.src = `footage/${f.name}.png`))
const ring = el('div', 'ring', cam)

// Footage coordinates are the app's own pixels (1600 × 900). z = 1 fills the frame; below 1 it floats.
const APPROVALS = rectOf('dash', 'approvals')
const BOX = rectOf('typing', 'textarea') ?? [560, 700, 480, 90]
const TYPE_AT = [BOX[0] + BOX[2] / 2, BOX[1] + BOX[3] / 2 - 30]
const CAM = [
  [T.product, 800, 450, 0.84], [T.typing[0] - 0.5, 800, 450, 0.84], [T.typing[0] + 0.3, TYPE_AT[0], TYPE_AT[1], 1.5], [T.send + 0.2, TYPE_AT[0], TYPE_AT[1], 1.5],
  [T.send + 1.0, 560, 420, 1.2], [T.saved - 0.2, 560, 420, 1.2], [T.saved + 1.0, 900, 450, 1.0], [T.saved + 2.2, OPEN[0], OPEN[1], 1.45],
  [T.openClick + 0.2, OPEN[0], OPEN[1], 1.45], [T.dash + 0.5, 800, 450, 1.0], [27.6, 800, 450, 1.0], [28.8, 640, 540, 1.6], [32.6, 660, 540, 1.6],
  [33.6, 800, 450, 1.0], [T.rangeClick + 0.6, 800, 450, 1.0], [35.6, 900, 700, 1.15], [37.0, 900, 700, 1.15],
  [T.safety + 0.4, APPROVALS[0] + APPROVALS[2] / 2, APPROVALS[1] + APPROVALS[3] / 2, 1.4], [T.allowClick + 0.4, APPROVALS[0] + APPROVALS[2] / 2, APPROVALS[1] + APPROVALS[3] / 2, 1.4],
  [T.allowed + 0.4, 800, 450, 1.0], [46.6, 1064, 302, 1.6], [49.2, 1064, 302, 1.6],
  [T.sources + 0.1, 1000, 430, 1.0], [T.sources + 1.2, 1000, 430, 1.22], [T.end - 0.2, 1010, 450, 1.3],
]
const cursor = el('div', 'cursor', stage, `<svg viewBox="0 0 34 44" width="34" height="44"><path d="M4 3 L4 35 L12 27.5 L17.5 40 L23 37.6 L17.6 25.4 L28.5 25.4 Z" fill="#fff" stroke="#000" stroke-width="2" stroke-linejoin="round"/></svg>`)
const ripple = el('div', 'ripple', stage)
// The pointer's three clicks: where, when it sets off, when it clicks.
const CLICKS = [
  { at: OPEN, from: T.openClick - 1.5, click: T.openClick },
  { at: RANGE30, from: T.rangeClick - 1.4, click: T.rangeClick },
  { at: ALLOW, from: T.allowClick - 1.6, click: T.allowClick },
]

// Captions.
const capLayer = layer()
const CAPS = [
  [T.typing[0] - 0.3, T.send + 0.2, 'Ask your agent <span class="t">once</span>.', 'right'],
  [T.send + 0.8, T.saved - 0.3, 'It checks the numbers first, <span class="t">read-only</span>.'],
  [T.saved + 0.3, T.openClick - 0.2, 'Then it saves a dashboard <span class="t">CloudBuddy keeps fresh</span>.'],
  [T.dash + 0.6, 28.6, 'CloudBuddy reruns the commands itself, <span class="t">on a timer</span>.'],
  [28.8, 32.8, 'No agent. <span class="t">No tokens.</span>'],
  [T.rangeClick + 0.4, 37.3, 'A month of history, <span class="t">on your computer</span>.'],
  [T.safety + 0.4, 41.4, 'Reads run <span class="t">by themselves</span>.'],
  [41.6, T.allowClick - 0.1, 'A script waits for <span class="t">your one OK</span>.'],
  [T.allowed + 0.4, 49.2, 'Allowed once. It runs on <span class="t">every refresh</span>.'],
  [T.sources + 0.2, T.end - 0.3, 'Deletes, payments and sends <span class="c">never</span> run from a dashboard.'],
].map(([a, b, html, pos]) => {
  const e = el('div', 'cap', capLayer, html)
  if (pos === 'top') Object.assign(e.style, { bottom: 'auto', top: '56px' })
  if (pos === 'right') Object.assign(e.style, { bottom: 'auto', top: '440px', left: '1420px' }) // over the empty canvas
  return { a, b, e }
})

const black = layer('black')

// ---------- drawing ----------
function drawHook(t) {
  const on = t < T.reveal + 0.2
  show(hookLayer, on ? 1 : 0)
  if (!on) return
  const out = 1 - prog(t, T.reveal - 0.8, 0.6)
  show(hookA, Math.min(window_(t, 0.3, T.same - 0.3, 0.5, 0.3), out))
  show(hookB, Math.min(prog(t, T.same, 0.4), out))
  hookB.style.transform = `translateY(${lerp(18, 0, out3(prog(t, T.same, 0.5)))}px)`
  for (const m of MORNINGS) {
    const k = Math.min(prog(t, m.at, 0.4), out)
    show(m.row, k)
    m.row.style.transform = `translateY(${lerp(30, 0, out3(prog(t, m.at, 0.5)))}px) scale(${lerp(0.96, 1, back(prog(t, m.at, 0.45)))})`
    show(m.agent, prog(t, m.at + 0.45, 0.35))
    // Once the point is made, the three look like one thing on repeat.
    m.row.style.filter = t > T.same ? `saturate(${lerp(1, 0.4, prog(t, T.same, 0.6))})` : 'none'
  }
}

function drawBrand(t) {
  const reveal = t >= T.reveal - 0.1 && t < T.product + 0.6
  const end = t >= T.end - 0.1
  show(brandLayer, reveal || end ? 1 : 0)
  if (!reveal && !end) return
  const at = reveal ? T.reveal : T.end
  const out = reveal ? 1 - prog(t, T.product - 0.4, 0.5) : 1
  logo.style.transform = `translateY(${reveal ? 0 : -40}px) scale(${(reveal ? 1 : 0.8) * lerp(0.4, 1, back(prog(t, at, 0.5)))})`
  show(logo, Math.min(prog(t, at, 0.3), out))
  const sc = (g, cx, cy, k) => g.setAttribute('transform', `translate(${cx} ${cy}) scale(${k}) translate(${-cx} ${-cy})`)
  sc(tile, 32, 32, 1)
  sc(cloud, 30, 37, lerp(0.2, 1, back(prog(t, at + 0.25, 0.45))))
  sc(buddy, 43, 38, lerp(0, 1, back(prog(t, at + 0.6, 0.4))))
  show(kicker, reveal ? Math.min(prog(t, at + 1.4, 0.5), out) : 0)
  kicker.style.transform = `translateX(-50%) translateY(${lerp(-12, 0, out3(prog(t, at + 1.4, 0.5)))}px)`
  const wk = Math.min(prog(t, at + 0.9, 0.5), out)
  show(word, wk)
  word.style.transform = `translateY(${lerp(20, 0, out3(prog(t, at + 0.9, 0.5))) + (reveal ? 0 : -60)}px) scale(${reveal ? 1 : 0.85})`
  show(tagline, Math.min(prog(t, at + 1.9, 0.6), out))
  tagline.style.transform = `translateY(${reveal ? 0 : -90}px)`
  show(endline, end ? prog(t, at + 2.4, 0.6) : 0)
  endline.style.transform = 'translateY(-90px)'
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

  // The command that waits, ringed while the caption names it.
  const rk = window_(t, 42.0, T.allowClick - 0.2, 0.3, 0.3)
  show(ring, rk)
  if (rk > 0) {
    const [x, y, wd, ht] = APPROVALS
    Object.assign(ring.style, { left: `${x + 16}px`, top: `${y + 70}px`, width: `${wd - 32}px`, height: `${ht - 84}px`, borderWidth: `${3 / S}px`, borderRadius: '10px' })
  }

  // The pointer: in from the bottom right, onto the button, click.
  let shown = 0
  for (const c of CLICKS) {
    const k = window_(t, c.from, c.click + 0.5, 0.3, 0.4)
    if (k <= 0) continue
    shown = k
    const [bx, by] = toScreen(c.at[0], c.at[1])
    const m = inOut(prog(t, c.from + 0.1, c.click - c.from - 0.3))
    const px = lerp(1700, bx, m)
    const py = lerp(1000, by, m) + Math.sin(m * Math.PI) * -60
    const press = t > c.click - 0.08 && t < c.click + 0.12 ? 0.86 : 1
    cursor.style.transform = `translate(${px - 4}px, ${py - 4}px) scale(${press})`
  }
  show(cursor, shown)
  show(ripple, 0)
  for (const c of CLICKS) {
    const rp = prog(t, c.click, 0.5)
    if (rp > 0 && rp < 1) {
      const [bx, by] = toScreen(c.at[0], c.at[1])
      show(ripple, 1 - rp)
      ripple.style.transform = `translate(${bx}px, ${by}px) scale(${lerp(1, 5, out3(rp))})`
    }
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
  show(bgGrid, 1)
  drawHook(t)
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
