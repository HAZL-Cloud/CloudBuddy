// The Dashboards demo's soundtrack, into promo/out/dash-music.wav: no samples, no downloads.
// A quiet loop under the same question asked three mornings, a lift for the logo, a light groove for the app
// that steps back while a command waits for the OK, and small sounds on what the picture shows.
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { synth } from '../lib/synth.mjs'
import { DURATION, T } from './timeline.mjs'

const { rand, pad, pluck, kick, hat, click, thud, bell, noiseSweep, render } = synth(DURATION)

// 1. The same question, three mornings: a soft loop and a tick, one pop per message.
for (let t = 0.6; t < T.reveal - 0.6; t += 0.5) click(t, 0.05, 1700, -0.3)
pad(0.4, T.same - 0.4, [50, 53, 57], 0.13, { a: 1.0, r: 0.8, bright: 0.3 }) // D minor
pad(T.same - 0.2, T.reveal - T.same - 0.4, [46, 50, 53], 0.14, { a: 0.6, r: 0.8, bright: 0.3 }) // B♭
for (const at of T.ask) bell(at + 0.05, 81, 0.09, 0.3)
thud(T.same, 0.22)

// 2. The logo.
noiseSweep(T.reveal - 2.0, 2.0, 0.24)
thud(T.reveal, 0.35)
bell(T.reveal + 0.3, 74, 0.3, -0.2)
bell(T.reveal + 0.65, 81, 0.2, 0.25)
pad(T.reveal, T.product - T.reveal, [50, 54, 57, 62, 66], 0.22, { a: 0.4, r: 1.6, bright: 0.6 })

// 3–7. The app: 96 BPM, D – A – Bm – G, plucked arpeggios. Quieter while the script waits for the OK.
const BEAT = 60 / 96
const BAR = BEAT * 4
const CHORDS = [[50, 54, 57], [45, 52, 57, 61], [47, 50, 54], [43, 50, 55, 59]]
const ARP = [[62, 66, 69, 74], [61, 64, 69, 73], [59, 62, 66, 71], [59, 62, 67, 71]]
const start = T.product + 0.2
for (let bar = 0; start + bar * BAR < T.end; bar++) {
  const t0 = start + bar * BAR
  const c = bar % 4
  const waiting = t0 >= T.safety - 0.5 && t0 < T.allowed - 0.5
  pad(t0, BAR, CHORDS[c], waiting ? 0.12 : 0.16, { a: 0.5, r: 0.9, bright: 0.45 })
  if (t0 < T.send - 0.5) continue // just the pad while the request is typed
  for (let b = 0; b < 4; b++) {
    const tb = t0 + b * BEAT
    if (!waiting && (b === 0 || b === 2)) kick(tb, 0.32)
    if (!waiting) hat(tb + BEAT / 2, 0.05)
    if (waiting && b === 0) kick(tb, 0.18)
  }
  for (let e = 0; e < 8; e++) pluck(t0 + e * (BEAT / 2), ARP[c][[0, 1, 2, 3, 2, 1, 2, 3][e]], waiting ? 0.06 : 0.1, e % 2 ? 0.35 : -0.35)
}
const typeStep = (T.typing[1] - T.typing[0]) / 9
for (let i = 0; i < 9; i++) click(T.typing[0] + i * typeStep + rand() * 0.02, 0.16, 2600 + rand() * 400)
noiseSweep(T.send - 0.05, 0.4, 0.12, false)
for (const at of T.cards) bell(at + 0.1, 86, 0.06, 0.4)
bell(T.saved + 0.1, 74, 0.16, -0.15) // saved
bell(T.saved + 0.28, 78, 0.14, 0.15)
bell(T.saved + 0.46, 81, 0.13, 0)
click(T.openClick, 0.45, 1500)
noiseSweep(T.dash - 0.2, 0.5, 0.1, false)
click(T.rangeClick, 0.4, 1600)
bell(T.safety + 0.5, 69, 0.15, -0.1) // a falling half step: something waits for you
bell(T.safety + 0.75, 68, 0.12, 0.1)
click(T.allowClick, 0.5, 1500)
bell(T.allowed + 0.05, 74, 0.2, -0.15)
bell(T.allowed + 0.22, 78, 0.18, 0.15)
bell(T.allowed + 0.4, 81, 0.16, 0)
bell(T.sources + 0.2, 86, 0.05, 0.3)

// 8. The end: one last chord and a bell.
thud(T.end, 0.3)
bell(T.end + 0.2, 74, 0.26, 0)
bell(T.end + 0.5, 78, 0.18, -0.3)
bell(T.end + 0.8, 81, 0.16, 0.3)
pad(T.end, DURATION - T.end - 1.6, [38, 50, 54, 57, 61, 64], 0.24, { a: 0.6, r: 1.6, bright: 0.55 }) // Dmaj9

const here = dirname(fileURLToPath(import.meta.url))
mkdirSync(join(here, '..', 'out'), { recursive: true })
render(join(here, '..', 'out', 'dash-music.wav'))
