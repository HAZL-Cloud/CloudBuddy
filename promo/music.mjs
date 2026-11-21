// Synthesizes the promo's soundtrack into promo/out/music.wav: no samples, no downloads.
// Tension under the terminal, a hard stop on the y/N prompt, a lift for the logo, a light groove for the app,
// and small sounds on the moments the picture shows (typing, cards, the click on Delete). Times come from timeline.mjs.
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { synth } from './lib/synth.mjs'
import { DURATION, PROMPT, T, linesBy, lineRate } from './timeline.mjs'

const { rand, pad, pluck, kick, hat, click, thud, bell, drone, noiseSweep, render } = synth(DURATION)

// ---------- the score ----------
// 1. Terminal: a low drone, a clock, typing, one tick per line, and a rush as the lines flood.
drone(0.3, T.wallStop - 0.3 + 0.05, 38, 0.16) // D2
drone(4, T.wallStop - 4 + 0.05, 45, 0.07) // A2
for (let t = 1.0; t < T.wallStop; t += 0.5) click(t, 0.05 + 0.1 * Math.min(1, t / T.wallStop), 1800, -0.3)
const typeStep = (T.promptType[1] - T.promptType[0]) / PROMPT.length
for (let i = 0; i < PROMPT.length; i += 2) click(T.promptType[0] + i * typeStep + rand() * 0.012, 0.22, 2600 + rand() * 500, 0.1)
let shown = 0
for (let t = T.wallStart; t < T.wallStop; t += 1 / 240) {
  const n = Math.floor(linesBy(t))
  if (n > shown) {
    const rate = lineRate(t)
    if (rate < 14 || n % Math.ceil(rate / 14) === 0) click(t, 0.11 * Math.max(0.35, 1 - rate / 60), 3200 + rand() * 900, rand() * 0.5)
    shown = n
  }
}
noiseSweep(11, T.wallStop - 11, 0.32)
for (let b = 0; b < 8; b++) kick(T.wallStop - 4 + b * 0.5, 0.18 + b * 0.04) // the build before the stop
// 2. The stop on y/N, a heartbeat, the key press, and it all carries on.
thud(T.wallStop, 0.75)
for (let t = T.wallStop + 1.2; t < T.pressY - 0.2; t += 0.95) {
  kick(t, 0.28)
  kick(t + 0.22, 0.16)
}
click(T.pressY, 0.6, 1900)
thud(T.pressY + 0.05, 0.25)
for (let i = 0; i < 28; i++) click(T.resume + i * 0.1, 0.08, 3400 + rand() * 600, rand() * 0.5)
pad(T.resume, 3.0, [38, 44, 51], 0.12, { a: 0.8, r: 1.2, bright: 0.2 }) // D, A♭, E♭: uneasy
// 3. The problem, in words: a quiet minor pad, then a rise into the logo.
pad(T.problem - 0.3, 3.0, [50, 53, 57], 0.16, { a: 1.0, bright: 0.25 }) // D minor
pad(T.problem2 - 0.3, 3.4, [46, 50, 53], 0.16, { a: 1.0, bright: 0.25 }) // B♭
noiseSweep(T.reveal - 2.2, 2.2, 0.28)
// 4. The logo: a bell and a warm D major.
bell(T.reveal + 0.3, 74, 0.32, -0.2)
bell(T.reveal + 0.65, 81, 0.22, 0.25)
thud(T.reveal, 0.35)
pad(T.reveal, 5.6, [50, 54, 57, 62, 66], 0.24, { a: 0.4, r: 2.0, bright: 0.6 })
// 5. The app: 96 BPM, D – A – Bm – G, with a plucked arpeggio. Quieter while the delete waits.
const BEAT = 60 / 96
const BAR = BEAT * 4
const CHORDS = [[50, 54, 57], [45, 52, 57, 61], [47, 50, 54], [43, 50, 55, 59]] // D, A, Bm, G
const ARP = [[62, 66, 69, 74], [61, 64, 69, 73], [59, 62, 66, 71], [59, 62, 67, 71]]
const grooveStart = T.product + 0.2
for (let bar = 0; grooveStart + bar * BAR < T.end; bar++) {
  const t0 = grooveStart + bar * BAR
  const c = bar % 4
  const waiting = t0 >= T.gate - 0.5 && t0 < T.approved - 0.5
  pad(t0, BAR, CHORDS[c], waiting ? 0.13 : 0.17, { a: 0.5, r: 0.9, bright: 0.45 })
  if (t0 < T.send) continue // the first bars: just the pad while the prompt is typed
  for (let b = 0; b < 4; b++) {
    const tb = t0 + b * BEAT
    if (!waiting && (b === 0 || b === 2)) kick(tb, 0.34)
    if (!waiting) hat(tb + BEAT / 2, 0.05)
    if (waiting && b === 0) kick(tb, 0.2)
  }
  for (let e = 0; e < 8; e++) {
    const note = ARP[c][[0, 1, 2, 3, 2, 1, 2, 3][e]]
    pluck(t0 + e * (BEAT / 2), note, waiting ? 0.07 : 0.11, e % 2 ? 0.35 : -0.35)
  }
}
for (let i = 0; i < 10; i++) click(T.typing[0] + i * 0.15 + rand() * 0.02, 0.16, 2600 + rand() * 400)
noiseSweep(T.send - 0.05, 0.4, 0.12, false)
for (const t of [T.firstCard, 52.6, 54.0, 57.8]) bell(t + 0.12, 86, 0.06, 0.4)
bell(T.gate + 0.2, 69, 0.16, -0.1)
bell(T.gate + 0.45, 68, 0.13, 0.1) // a falling half step: something waits for you
click(T.click, 0.5, 1500)
bell(T.approved + 0.05, 74, 0.2, -0.15)
bell(T.approved + 0.22, 78, 0.18, 0.15)
bell(T.approved + 0.4, 81, 0.16, 0)
bell(T.stripe + 0.2, 69, 0.12, -0.1)
bell(T.stripe + 0.45, 68, 0.1, 0.1)
for (let i = 0; i < 9; i++) bell(T.replay + i * 0.62 + 0.05, 74 + [0, 2, 4, 5, 7, 9, 11, 12, 14][i], 0.05, -0.5 + i / 8)
// 6. The end: one last chord and a bell.
thud(T.end, 0.3)
bell(T.end + 0.2, 74, 0.26, 0)
bell(T.end + 0.5, 78, 0.18, -0.3)
bell(T.end + 0.8, 81, 0.16, 0.3)
pad(T.end, DURATION - T.end - 1.8, [38, 50, 54, 57, 61, 64], 0.26, { a: 0.6, r: 1.6, bright: 0.55 }) // Dmaj9

const here = dirname(fileURLToPath(import.meta.url))
mkdirSync(join(here, 'out'), { recursive: true })
render(join(here, 'out', 'music.wav'))
