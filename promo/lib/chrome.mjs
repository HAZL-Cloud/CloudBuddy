// A small headless Chrome driver over the DevTools protocol, shared by capture.mjs and render.mjs.
import { spawn } from 'node:child_process'
import { mkdirSync } from 'node:fs'

const CHROME = process.env.CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

export async function openChrome({ width, height, scale = 1, port = 9340, profile }) {
  mkdirSync(profile, { recursive: true })
  const proc = spawn(
    CHROME,
    ['--headless=new', '--disable-gpu', '--hide-scrollbars', '--mute-audio', '--force-color-profile=srgb', `--window-size=${width},${height}`,
      `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, 'about:blank'],
    { stdio: 'ignore' },
  )
  let target
  for (let i = 0; i < 75 && !target; i++) {
    await sleep(200)
    try {
      target = (await (await fetch(`http://127.0.0.1:${port}/json`)).json()).find((t) => t.type === 'page')
    } catch {}
  }
  if (!target) {
    proc.kill()
    throw new Error('Chrome did not start')
  }
  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((r, j) => ((ws.onopen = r), (ws.onerror = j)))
  let id = 0
  const pending = new Map()
  ws.onmessage = (m) => {
    const d = JSON.parse(m.data)
    if (d.id && pending.has(d.id)) {
      const p = pending.get(d.id)
      pending.delete(d.id)
      d.error ? p.rej(new Error(JSON.stringify(d.error))) : p.res(d.result)
    }
  }
  const send = (method, params = {}) =>
    new Promise((res, rej) => {
      const i = ++id
      pending.set(i, { res, rej })
      ws.send(JSON.stringify({ id: i, method, params }))
    })
  await send('Page.enable')
  await send('Runtime.enable')
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: scale, mobile: false })

  const page = {
    send,
    async goto(url, settle = 1500) {
      await send('Page.navigate', { url })
      await sleep(settle)
    },
    async eval(expression) {
      const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
      if (r.exceptionDetails) throw new Error(`${expression.slice(0, 80)}: ${r.exceptionDetails.exception?.description ?? r.exceptionDetails.text}`)
      return r.result.value
    },
    async shot(format = 'png', quality) {
      const r = await send('Page.captureScreenshot', { format, ...(quality ? { quality } : {}), captureBeyondViewport: false })
      return Buffer.from(r.data, 'base64')
    },
    close() {
      ws.close()
      proc.kill()
    },
  }
  return page
}
