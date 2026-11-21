// Films the real app for the Dashboards demo: a chat with the demo agent builds "Shop pulse" over a
// made-up shop (setup.mjs), then the Dashboards page, the range switch, the one OK and the command list.
// Saves a screenshot each time the screen changes, and where the buttons the video clicks are.
//
//   node promo/dash/setup.mjs <folder>/shop-web
//   CLOUDBUDDY_PORT=7421 CLOUDBUDDY_HOME=<folder>/home CLOUDBUDDY_DB=postgres://…/<empty database> \
//     bin/cloudbuddy serve --no-open --fake promo/dash/scenario.json
//   node promo/dash/capture.mjs http://127.0.0.1:7421 <folder>/shop-web <empty database>
//
// The demo agent is labelled "Claude Code" on screen, and the folder and port read as the defaults.
import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openChrome, sleep } from '../lib/chrome.mjs'

const [, , address, workdirArg, database] = process.argv
if (!address || !workdirArg || !database) throw new Error('usage: node promo/dash/capture.mjs <address> <shop folder> <database>')
const workdir = realpathSync(workdirArg)
const here = dirname(fileURLToPath(import.meta.url))
const out = join(here, 'footage')
rmSync(out, { recursive: true, force: true })
mkdirSync(out, { recursive: true })
const origin = new URL(address).origin

const RELABEL = [
  ['Demo agent', 'Claude Code'],
  ['demo-replay', 'Opus 5.5'],
  [workdir, '~/code/shop-web'],
  [workdirArg, '~/code/shop-web'],
  [new URL(address).port, '7420'],
]
// Runs in every page before its own scripts: dark theme, and the labels above, kept up to date as React renders.
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

const page = await openChrome({ width: 1600, height: 900, scale: 2, port: 9342, profile: join(here, '..', '.chrome-dash-capture') })
const manifest = []
let n = 0
const t0 = Date.now()

// Where things are on screen, in the app's own pixels: [x, y, width, height].
const RECTS = `(() => {
  const r = (el) => { if (!el) return null; const b = el.getBoundingClientRect(); return b.width ? [b.x, b.y, b.width, b.height].map(Math.round) : null };
  const byText = (sel, text) => [...document.querySelectorAll(sel)].find((e) => e.textContent.trim().startsWith(text));
  const out = {
    open: r(byText('button', 'Open dashboard')),
    allow: r(byText('button', 'Allow on the timer')),
    approvals: r(document.querySelector('.dash-approvals')),
    range30: r(byText('.seg button', '30 days')),
    sources: r(document.querySelector('.dash-sources')),
    textarea: r(document.querySelector('textarea')),
    send: r(document.querySelector('.send')),
    navDash: r(byText('.nav-item', 'Dashboards')),
  };
  return out;
})()`

async function snap(label) {
  await page.eval('document.querySelector("textarea")?.blur(), 0')
  const file = `${String(++n).padStart(3, '0')}-${label}.png`
  writeFileSync(join(out, file), await page.shot())
  manifest.push({ file, label, at: (Date.now() - t0) / 1000, rects: await page.eval(RECTS), scrollY: await page.eval('Math.round(document.querySelector(".dash-page")?.scrollTop ?? 0)') })
  console.log(file)
}
const signature = () =>
  page.eval(`[document.querySelectorAll('[data-step-id]').length, document.querySelector('.messages')?.textContent.length ?? 0,
    document.querySelector('.canvas-panel')?.textContent.length ?? 0].join('|')`)
const runDone = () => page.eval(`!!document.querySelector('.recent.on')?.textContent.match(/steps? ·/)`)
const waitFor = async (expr, what, limit = 20000) => {
  const end = Date.now() + limit
  while (Date.now() < end) {
    if (await page.eval(expr)) return
    await sleep(150)
  }
  throw new Error(`timed out waiting for ${what}`)
}
const clickText = (sel, text) =>
  page.eval(`(() => { const b = [...document.querySelectorAll(${JSON.stringify(sel)})].find((e) => e.textContent.trim().startsWith(${JSON.stringify(text)})); if (!b) return false; b.click(); return true })()`)

// Record every change until the run ends.
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
    if (await runDone()) return 'done'
  }
  return 'timeout'
}

// A month of made-up history for the number tiles, tagged like the refresh's own points so the page shows it.
function seedHistory() {
  const hours = JSON.parse(readFileSync(join(workdir, '..', 'shop-history.json'), 'utf8'))
  const rows = []
  for (const h of hours) rows.push(`('users|', '${h.t}', ${h.users})`, `('paying|', '${h.t}', ${h.paying})`)
  const sql = `
    CREATE TEMP TABLE hist(series text, ts timestamptz, value float8);
    INSERT INTO hist VALUES ${rows.join(',')};
    INSERT INTO dashboard_points(dashboard_id, series, source_rev, ts, value)
      SELECT p.dashboard_id, h.series, p.source_rev, h.ts, h.value
      FROM hist h JOIN (SELECT DISTINCT dashboard_id, series, source_rev FROM dashboard_points) p USING (series);
    SELECT series, count(*) FROM dashboard_points GROUP BY 1 ORDER BY 1;`
  console.log(execFileSync('docker', ['exec', '-i', 'cloudbuddy-postgres', 'psql', '-U', 'cloudbuddy', '-d', database, '-v', 'ON_ERROR_STOP=1', '-At'], { input: sql }).toString().trim())
}

try {
  await page.send('Page.addScriptToEvaluateOnNewDocument', { source: PRELUDE })
  await page.goto(`${origin}/app`, 2500)
  const saved = await page.eval(`(async () => {
    const r = await fetch('/api/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ agent: 'fake', folder: ${JSON.stringify(workdir)} }) });
    return r.status;
  })()`)
  if (saved !== 200) throw new Error(`saving settings: ${saved}`)
  await page.goto(`${origin}/app`, 2500)
  await snap('new-chat')

  const message = 'Make me a dashboard of our users, signups and revenue'
  await page.eval(`document.querySelector('textarea').focus(), 0`)
  for (let i = 0; i < message.length; i += 6) {
    await page.send('Input.insertText', { text: message.slice(i, i + 6) })
    await sleep(60)
    if (i % 18 === 0) await snap('typing')
  }
  await snap('typed')
  await page.eval(`document.querySelector('.send').click(), 0`)
  console.log('run ended:', await follow('run'))
  await waitFor(`[...document.querySelectorAll('button')].some((b) => b.textContent.includes('Open dashboard'))`, 'the Open dashboard button')
  await sleep(1200)
  await snap('saved')

  seedHistory()
  if (!(await clickText('button', 'Open dashboard'))) throw new Error('no Open dashboard button')
  await waitFor(`!!document.querySelector('.dash-page .seg')`, 'the dashboard page')
  await sleep(2500)
  await snap('dash')
  await clickText('.seg button', '30 days')
  await sleep(2500)
  await snap('dash-30d')

  // The revenue script waits for one OK.
  await page.eval(`document.querySelector('.dash-approvals')?.scrollIntoView({ block: 'center' }), 0`)
  await sleep(900)
  await snap('approval')
  if (!(await clickText('button', 'Allow on the timer'))) throw new Error('no Allow button')
  await sleep(600)
  await snap('allowing')
  await waitFor(`![...document.querySelectorAll('button')].some((b) => b.textContent.includes('Allow on the timer'))`, 'the OK to land')
  await sleep(2500)
  await page.eval(`document.querySelector('.dash-page').scrollTo({ top: 0 }), 0`)
  await sleep(900)
  await snap('allowed')

  // Every command and what CloudBuddy decided about it.
  await page.eval(`(() => { const d = document.querySelector('.dash-sources'); d.open = true; d.scrollIntoView({ block: 'start' }); return 0 })()`)
  await sleep(1200)
  await snap('sources')

  // The Dashboards list.
  await clickText('.nav-item', 'Dashboards')
  await sleep(600)
  await clickText('button', 'All dashboards') || (await clickText('.dash-back', ''))
  await sleep(1800)
  await snap('list')
  // No Connections page: it lists the real CLI logins on this computer.
} finally {
  writeFileSync(join(out, 'manifest.json'), JSON.stringify(manifest, null, 2))
  page.close()
}
