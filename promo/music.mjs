// Synthesizes the promo's soundtrack into promo/out/music.wav: no samples, no downloads.
// Tension under the terminal, a hard stop on the y/N prompt, a lift for the logo, a light groove for the app,
// and small sounds on the moments the picture shows (typing, cards, the click on Delete). Times come from timeline.mjs.
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { DURATION, PROMPT, T, linesBy, lineRate } from './timeline.mjs'

const SR = 48000
const N = Math.ceil(DURATION * SR)
const dry = [new Float32Array(N), new Float32Array(N)]
const wet = [new Float32Array(N), new Float32Array(N)] // sent to the reverb

// A tiny deterministic random source, so every build sounds the same.
let seed = 7
const rand = () => ((seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296) * 2 - 1
const hz = (midi) => 440 * Math.pow(2, (midi - 69) / 12)

function put(start, len, fn, { amp = 1, pan = 0, send = 0 } = {}) {
  const s0 = Math.max(0, Math.round(start * SR))
  const l = Math.cos(((pan + 1) * Math.PI) / 4) * amp
  const r = Math.sin(((pan + 1) * Math.PI) / 4) * amp
  for (let i = 0; i < len && s0 + i < N; i++) {
    const v = fn(i / SR, i)
    dry[0][s0 + i] += v * l
    dry[1][s0 + i] += v * r
    if (send) {
      wet[0][s0 + i] += v * l * send
      wet[1][s0 + i] += v * r * send
    }
  }
}
const env = (t, dur, a, r) => Math.min(1, t / a) * Math.min(1, Math.max(0, (dur - t) / r))

// ---------- instruments ----------
function pad(start, dur, notes, amp, { a = 1.2, r = 1.6, bright = 0.5 } = {}) {
  const len = Math.round((dur + r) * SR)
  notes.forEach((m, k) => {
    for (const [det, pan] of [[-0.0016, -0.6], [0.0016, 0.6]]) {
      const f = hz(m) * (1 + det)
      let lp = 0
      const cut = 1 - Math.exp((-2 * Math.PI * (500 + bright * 2200)) / SR)
      const ph = rand() * 6
      put(start, len, (t) => {
        let v = 0
        for (let h = 1; h <= 7; h++) v += Math.sin(2 * Math.PI * f * h * t + ph * h) / (h * h * 0.7 + 0.3)
        lp += (v - lp) * cut
        return lp * env(t, dur + r, a, r)
      }, { amp: amp / notes.length, pan: pan * (k % 2 ? 1 : -1) * 0.8, send: 0.55 })
    }
  })
}
function pluck(start, m, amp, pan = 0) {
  const f = hz(m)
  const p = Math.round(SR / f)
  const buf = Float32Array.from({ length: p }, () => rand())
  let idx = 0
  let prev = 0
  put(start, Math.round(1.4 * SR), () => {
    const v = buf[idx]
    const next = 0.5 * (v + buf[(idx + 1) % p]) * 0.994
    buf[idx] = next
    idx = (idx + 1) % p
    prev += (v - prev) * 0.5
    return prev
  }, { amp, pan, send: 0.4 })
}
function kick(start, amp) {
  let ph = 0
  put(start, Math.round(0.5 * SR), (t) => {
    ph += (2 * Math.PI * (46 + 70 * Math.exp(-t * 28))) / SR
    return Math.sin(ph) * Math.exp(-t * 7) + (t < 0.004 ? rand() * 0.3 : 0)
  }, { amp })
}
function hat(start, amp, pan = 0.25) {
  let last = 0
  put(start, Math.round(0.06 * SR), (t) => {
    const n = rand()
    const v = n - last
    last = n
    return v * Math.exp(-t * 90)
  }, { amp, pan })
}
function click(start, amp, pitch = 2400, pan = 0) {
  let last = 0
  put(start, Math.round(0.03 * SR), (t) => {
    const n = rand()
    const v = n - last * 0.6
    last = n
    return (v * 0.7 + Math.sin(2 * Math.PI * pitch * t) * 0.5) * Math.exp(-t * 260)
  }, { amp, pan })
}
function thud(start, amp) {
  put(start, Math.round(1.6 * SR), (t) => Math.sin(2 * Math.PI * (38 + 30 * Math.exp(-t * 10)) * t) * Math.exp(-t * 2.6), { amp, send: 0.2 })
}
function bell(start, m, amp, pan = 0) {
  const f = hz(m)
  put(start, Math.round(3 * SR), (t) =>
    [[1, 1, 1.6], [2.0, 0.5, 2.6], [3.01, 0.28, 3.4], [4.2, 0.14, 5]].reduce((v, [ratio, a, d]) => v + a * Math.sin(2 * Math.PI * f * ratio * t) * Math.exp(-t * d), 0) *
    Math.min(1, t / 0.003),
  { amp, pan, send: 0.7 })
}
function drone(start, dur, m, amp) {
  put(start, Math.round(dur * SR), (t) => {
    const f = hz(m)
    const wob = 1 + 0.25 * Math.sin(2 * Math.PI * 0.13 * t)
    return (Math.sin(2 * Math.PI * f * t) + 0.35 * Math.sin(2 * Math.PI * f * 2.003 * t) * wob + 0.2 * Math.sin(2 * Math.PI * f * 3.01 * t)) * env(t, dur, 2, 0.8)
  }, { amp, send: 0.3 })
}
function noiseSweep(start, dur, amp, rising = true) {
  let lp = 0
  put(start, Math.round(dur * SR), (t) => {
    const k = t / dur
    const cutHz = rising ? 200 + 6000 * k * k : 6000 - 5800 * k
    lp += (rand() - lp) * (1 - Math.exp((-2 * Math.PI * cutHz) / SR))
    return lp * (rising ? k * k : (1 - k) * (1 - k))
  }, { amp, send: 0.5 })
}

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

// ---------- reverb (a small Schroeder room), mix, master ----------
function reverb(input, offset) {
  const combs = [1557, 1617, 1491, 1422].map((d) => ({ buf: new Float32Array(d + offset), i: 0, lp: 0 }))
  const aps = [556, 441].map((d) => ({ buf: new Float32Array(d + offset), i: 0 }))
  const out = new Float32Array(N)
  for (let n = 0; n < N; n++) {
    let s = 0
    for (const c of combs) {
      const y = c.buf[c.i]
      c.lp = y * 0.7 + c.lp * 0.3
      c.buf[c.i] = input[n] * 0.3 + c.lp * 0.84
      c.i = (c.i + 1) % c.buf.length
      s += y
    }
    for (const a of aps) {
      const y = a.buf[a.i]
      a.buf[a.i] = s + y * 0.5
      a.i = (a.i + 1) % a.buf.length
      s = y - s * 0.5
    }
    out[n] = s
  }
  return out
}
const rev = [reverb(wet[0], 0), reverb(wet[1], 23)]
const mix = [new Float32Array(N), new Float32Array(N)]
let peak = 0
for (let c = 0; c < 2; c++) {
  for (let n = 0; n < N; n++) {
    const fadeOut = Math.min(1, (DURATION - n / SR) / 1.2)
    const v = Math.tanh((dry[c][n] + rev[c][n] * 0.32) * 1.1) * Math.max(0, fadeOut)
    mix[c][n] = v
    peak = Math.max(peak, Math.abs(v))
  }
}
const gain = 0.89 / peak // -1 dBFS peak; loudness is set when the video is encoded

const here = dirname(fileURLToPath(import.meta.url))
mkdirSync(join(here, 'out'), { recursive: true })
const wav = Buffer.alloc(44 + N * 4)
wav.write('RIFF', 0)
wav.writeUInt32LE(36 + N * 4, 4)
wav.write('WAVEfmt ', 8)
wav.writeUInt32LE(16, 16)
wav.writeUInt16LE(1, 20)
wav.writeUInt16LE(2, 22)
wav.writeUInt32LE(SR, 24)
wav.writeUInt32LE(SR * 4, 28)
wav.writeUInt16LE(4, 32)
wav.writeUInt16LE(16, 34)
wav.write('data', 36)
wav.writeUInt32LE(N * 4, 40)
for (let n = 0; n < N; n++) {
  wav.writeInt16LE(Math.round(Math.max(-1, Math.min(1, mix[0][n] * gain)) * 32767), 44 + n * 4)
  wav.writeInt16LE(Math.round(Math.max(-1, Math.min(1, mix[1][n] * gain)) * 32767), 46 + n * 4)
}
const file = join(here, 'out', 'music.wav')
writeFileSync(file, wav)
console.log(file, `peak ${peak.toFixed(2)} → gain ${gain.toFixed(2)}`)
