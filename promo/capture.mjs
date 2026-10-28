// Films the real app for the promo: one chat with the demo agent, which replays a recorded deploy
// through the real gate and never runs a command. Saves a screenshot each time the screen changes.
//
//   bin/cloudbuddy serve --no-open --fake testdata/scenarios/demo.json   (use a throwaway CLOUDBUDDY_DB)
//   node promo/capture.mjs <launch link> <working folder for the chat>
//
// The demo agent is labelled "Claude Code" on screen, and the folder and port read as the defaults.
import { mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openChrome, sleep } from './lib/chrome.mjs'

const [, , launch, workdir] = process.argv
if (!launch || !workdir) throw new Error('usage: node promo/capture.mjs <launch link> <working folder>')
const here = dirname(fileURLToPath(import.meta.url))
const out = join(here, 'footage')
rmSync(out, { recursive: true, force: true })
mkdirSync(out, { recursive: true })
const origin = new URL(launch).origin

const RELABEL = [
  ['Demo agent', 'Claude Code'],
  ['demo-replay', 'Opus 5.5'],
  [workdir, '~/code/shipyard-web'],
  [new URL(launch).port, '7420'],
]
// Runs in every page before its own scripts: dark theme, and the labels above, kept up to date as React renders.
// (document.documentElement can still be missing when this runs, so everything waits for it.)
const PRELUDE = `
(() => {
  const pairs = ${JSON.stringify(RELABEL)};
  const fix = (s) => pairs.reduce((acc, [a, b]) => acc.split(a).join(b), s);
  const walk = (root) => {
    const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let n = w.nextNode(); n; n = w.nextNode()) { const v = fix(n.nodeValue); if (v !== n.nodeValue) n.nodeValue = v; }
    root.querySelectorAll?.('[placeholder],[aria-label],[title]').forEach((el) => {
      for (const a of ['placeholder', 'aria-label', 'title']) { const v = el.getAttribute(a); if (v && fix(v) !== v) el.setAttribute(a, fix(v)); }
    });
    document.querySelectorAll('.runtime').forEach((el) => { if (el.textContent.includes('replay')) el.style.display = 'none'; });
  };
  const dark = () => { if (document.documentElement && document.documentElement.dataset.theme !== 'dark') document.documentElement.dataset.theme = 'dark'; };
  new MutationObserver(() => { dark(); if (document.body) walk(document.body); }).observe(document, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ['placeholder', 'aria-label', 'title', 'data-theme'] });
  document.addEventListener('DOMContentLoaded', () => { dark(); walk(document.body); });
})();`

const page = await openChrome({ width: 1600, height: 900, scale: 2, profile: join(here, '.chrome-capture') })
const manifest = []
let n = 0
const t0 = Date.now()
async function snap(label) {
  await page.eval('document.querySelector("textarea")?.blur(), 0')
  const file = `${String(++n).padStart(3, '0')}-${label}.png`
  writeFileSync(join(out, file), await page.shot())
  manifest.push({ file, label, at: (Date.now() - t0) / 1000 })
  console.log(file)
}
const signature = () =>
  page.eval(`[document.querySelectorAll('[data-step-id]').length, document.querySelector('.messages')?.textContent.length ?? 0,
    document.querySelectorAll('.approval:not(.done) .ap-actions').length, document.querySelector('.canvas-panel')?.textContent.length ?? 0].join('|')`)
const waiting = () => page.eval(`document.querySelectorAll('.approval:not(.done) .ap-actions .btn.danger:not(:disabled)').length`)
const closeDetails = async () => {
  await page.eval(`document.querySelector('.inspector button[aria-label^="Close"]')?.click(), 0`)
  await sleep(400)
}
const runDone = () => page.eval(`!!document.querySelector('.recent.on')?.textContent.match(/steps ·/)`)

// Record every change until the run waits for a decision or ends.
async function follow(label, limit = 45000) {
  let last = await signature()
  const end = Date.now() + limit
  while (Date.now() < end) {
    await sleep(150)
    const now = await signature()
    if (now !== last) {
      await sleep(650) // let the card's entrance finish
      last = await signature()
      await snap(label)
    }
    if (await waiting()) return 'waiting'
    if (await runDone()) return 'done'
  }
  return 'timeout'
}

try {
  await page.send('Page.addScriptToEvaluateOnNewDocument', { source: PRELUDE })
  await page.goto(launch, 2500)
  await snap('landing')

  // Settings for new chats: the demo agent, working in the given folder.
  await page.goto(`${origin}/app`, 2500)
  const saved = await page.eval(`(async () => {
    const s = await (await fetch('/api/session')).json();
    const r = await fetch('/api/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json', 'X-CB-CSRF': s.csrfToken }, body: JSON.stringify({ agent: 'fake', folder: ${JSON.stringify(workdir)} }) });
    return r.status;
  })()`)
  if (saved !== 200) throw new Error(`saving settings: ${saved}`)
  await page.goto(`${origin}/app`, 2500)
  await snap('new-chat')

  const message = 'Deploy the shop site to S3 + CloudFront, then delete the old staging bucket'
  await page.eval(`document.querySelector('textarea').focus(), 0`)
  for (let i = 0; i < message.length; i += 6) {
    await page.send('Input.insertText', { text: message.slice(i, i + 6) })
    await sleep(60)
    if (i % 18 === 0) await snap('typing')
  }
  await snap('typed')
  await page.eval(`document.querySelector('.send').click(), 0`)

  let state = await follow('run')
  let decisions = 0
  while (state === 'waiting' && decisions < 4) {
    decisions++
    await closeDetails()
    await snap(`wait${decisions}`)
    // Show the waiting card on the canvas, then approve it from there.
    await page.eval(`[...document.querySelectorAll('.approval:not(.done) .btn.link')].find((b) => b.textContent.includes('Review on canvas'))?.click(), 0`)
    await sleep(1200)
    await snap(`wait${decisions}-canvas`)
    await page.eval(`document.querySelector('.approval:not(.done) .ap-actions .btn.danger').click(), 0`)
    await sleep(900)
    await snap(`approved${decisions}`)
    await closeDetails()
    state = await follow('run')
  }
  console.log('run ended:', state)
  await sleep(1200)
  await snap('done')

  // Replay, step by step, at 100% so the cards stay readable.
  await page.eval(`document.querySelector('.btn.play')?.click(), 0`)
  let lastStep = ''
  const until = Date.now() + 30000
  while (Date.now() < until) {
    await sleep(120)
    const at = await page.eval(`document.querySelector('.scrubber, .replay')?.textContent ?? document.body.textContent.match(/Step \\d+ of \\d+/)?.[0] ?? ''`)
    const step = (at.match(/Step (\d+) of (\d+)/) ?? [])[0] ?? ''
    if (step && step !== lastStep) {
      lastStep = step
      await sleep(350)
      await snap('replay')
      const [, a, b] = step.match(/Step (\d+) of (\d+)/)
      if (a === b) break
    }
  }

  // No Connections page: it lists the real CLI logins on this computer (account numbers, emails).
} finally {
  writeFileSync(join(out, 'manifest.json'), JSON.stringify(manifest, null, 2))
  page.close()
}
