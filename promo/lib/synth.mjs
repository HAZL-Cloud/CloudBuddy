// The synthesizer both promo soundtracks use: instruments, a small room reverb, and a 16-bit stereo WAV.
// No samples, no downloads. synth(duration) gives a fresh mix; call its instruments, then render(file).
import { writeFileSync } from 'node:fs'

export function synth(DURATION) {
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

  function render(file) {
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
    writeFileSync(file, wav)
    console.log(file, `peak ${peak.toFixed(2)} → gain ${gain.toFixed(2)}`)
  }

  return { SR, rand, hz, pad, pluck, kick, hat, click, thud, bell, drone, noiseSweep, render }
}
