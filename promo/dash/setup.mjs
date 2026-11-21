// Makes the made-up shop the Dashboards demo films: a SQLite database of users, an API status file and a
// revenue script. Every name and number is invented here, from a fixed seed, so every build is the same.
//
//   node promo/dash/setup.mjs <empty folder>
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const dir = process.argv[2]
if (!dir) throw new Error('usage: node promo/dash/setup.mjs <empty folder>')
mkdirSync(join(dir, 'status'), { recursive: true })
mkdirSync(join(dir, 'scripts'), { recursive: true })
mkdirSync(join(dir, 'data'), { recursive: true })

let seed = 11
const rand = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296
const DAY = 86400_000
const now = Date.now()

// 860 users from before the last 30 days, then a growing trickle: 1,204 in all.
const created = []
for (let i = 0; i < 860; i++) created.push(now - (31 + rand() * 330) * DAY)
const perDay = []
for (let d = 29; d >= 0; d--) perDay.push(Math.round(7 + (29 - d) * 0.35 + rand() * 5))
let extra = 1204 - 860 - perDay.reduce((a, b) => a + b, 0)
for (let i = perDay.length - 1; extra !== 0; i = (i - 1 + perDay.length) % perDay.length) {
  const step = Math.sign(extra)
  if (perDay[i] + step >= 4) {
    perDay[i] += step
    extra -= step
  }
}
perDay.forEach((n, k) => {
  const d = 29 - k
  for (let i = 0; i < n; i++) created.push(now - d * DAY - rand() * (d === 0 ? 0.4 : 0.98) * DAY)
})
created.sort((a, b) => a - b)

const plans = Array(created.length).fill('free')
const pick = (count, plan) => {
  for (let placed = 0; placed < count; ) {
    const i = Math.floor(rand() * created.length)
    if (plans[i] === 'free') {
      plans[i] = plan
      placed++
    }
  }
}
pick(79, 'pro')
pick(8, 'team')

const iso = (ms) => new Date(ms).toISOString().replace('T', ' ').slice(0, 19)
const rows = created.map((t, i) => `(${i + 1},'user${i + 1}@example.com','${plans[i]}','${iso(t)}')`)
const sql = [
  'CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL, plan TEXT NOT NULL, created_at TEXT NOT NULL);',
  ...Array.from({ length: Math.ceil(rows.length / 200) }, (_, k) => `INSERT INTO users VALUES ${rows.slice(k * 200, k * 200 + 200).join(',')};`),
].join('\n')
execFileSync('sqlite3', [join(dir, 'shop.db')], { input: sql })

writeFileSync(join(dir, 'status', 'api.json'), JSON.stringify({ state: 'healthy', p95_ms: 182, checked: 'every minute' }, null, 2) + '\n')
const subs = { pro: { price: 15, count: 79 }, team: { price: 59, count: 8 } }
writeFileSync(join(dir, 'data', 'subscriptions.json'), JSON.stringify(subs, null, 2) + '\n')
writeFileSync(
  join(dir, 'scripts', 'mrr.js'),
  `// Monthly recurring revenue from the subscriptions export.
const subs = require('../data/subscriptions.json')
console.log(Object.values(subs).reduce((sum, p) => sum + p.price * p.count, 0))
`,
)

// The history the video shows: one number per hour for 30 days, ending at today's counts.
const hourly = []
for (let h = 30 * 24; h >= 1; h--) {
  const t = now - h * 3600_000
  hourly.push({ t: new Date(t).toISOString(), users: created.filter((c) => c <= t).length, paying: Math.round(87 - (h / 720) * 19) })
}
writeFileSync(join(dir, '..', 'shop-history.json'), JSON.stringify(hourly))
console.log(`users ${created.length} · pro 79 · team 8 · today ${perDay.at(-1)} · history ${hourly.length} hours`)
