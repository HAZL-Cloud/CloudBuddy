// Renders video.html frame by frame in headless Chrome and encodes it with ffmpeg.
//
//   node promo/render.mjs stills 0 12.5 61      one PNG per time, in promo/out/stills
//   node promo/render.mjs video                 promo/out/cloudbuddy-promo.mp4 (with music.wav when it exists)
import { spawn } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { dirname, extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openChrome } from './lib/chrome.mjs'
import { DURATION, FPS } from './timeline.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, '..')
const outDir = join(here, 'out')
mkdirSync(outDir, { recursive: true })
const [, , mode = 'video', ...args] = process.argv

// The page loads fonts from web/node_modules, so it is served over HTTP from the repo root (fonts don't load from file://).
const TYPES = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.woff2': 'font/woff2' }
const server = createServer((req, res) => {
  const path = normalize(decodeURIComponent(new URL(req.url, 'http://x').pathname))
  const file = join(root, path)
  if (!file.startsWith(root) || !existsSync(file) || !statSync(file).isFile()) {
    res.writeHead(404).end()
    return
  }
  res.writeHead(200, { 'Content-Type': TYPES[extname(file)] ?? 'application/octet-stream' })
  createReadStream(file).pipe(res)
})
await new Promise((r) => server.listen(0, '127.0.0.1', r))
const url = `http://127.0.0.1:${server.address().port}/promo/video.html`

const page = await openChrome({ width: 1920, height: 1080, port: 9341, profile: join(here, '.chrome-render') })
try {
  await page.goto(url, 1500)
  await page.eval('window.ready')
  const draw = (t) => page.eval(`(window.seek(${t}), new Promise((r) => requestAnimationFrame(() => r(0))))`)

  if (mode === 'stills') {
    mkdirSync(join(outDir, 'stills'), { recursive: true })
    for (const a of args) {
      await draw(Number(a))
      const file = join(outDir, 'stills', `t${String(a).padStart(5, '0')}.png`)
      writeFileSync(file, await page.shot())
      console.log(file)
    }
  } else {
    const music = join(outDir, 'music.wav')
    const withMusic = existsSync(music)
    const target = join(outDir, withMusic ? 'cloudbuddy-promo.mp4' : 'cloudbuddy-promo-silent.mp4')
    const ff = spawn('ffmpeg', [
      '-loglevel', 'error', '-y',
      '-f', 'image2pipe', '-framerate', String(FPS), '-c:v', 'png', '-i', '-',
      ...(withMusic ? ['-i', music, '-c:a', 'aac', '-b:a', '192k', '-shortest'] : []),
      '-c:v', 'libx264', '-preset', 'slow', '-crf', '17', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', target,
    ], { stdio: ['pipe', 'inherit', 'inherit'] })
    const frames = Math.round(DURATION * FPS)
    const started = Date.now()
    for (let i = 0; i < frames; i++) {
      await draw(i / FPS)
      const png = await page.shot()
      if (!ff.stdin.write(png)) await new Promise((r) => ff.stdin.once('drain', r))
      if (i % 150 === 0) console.log(`frame ${i}/${frames} · ${Math.round((Date.now() - started) / 1000)}s`)
    }
    ff.stdin.end()
    await new Promise((r, j) => ff.on('close', (code) => (code === 0 ? r() : j(new Error(`ffmpeg exited ${code}`)))))
    console.log(target)
  }
} finally {
  page.close()
  server.close()
}
