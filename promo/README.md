# Promo video

A 90-second promo: an agent CLI floods a terminal with commands and asks a bare y/N before deleting a bucket; then the same job in CloudBuddy, where every step is a card and the delete waits on the canvas.

Everything is made here, from code: the terminal scenes and titles are drawn by `video.js`, the app shots are screenshots of the real app, and the soundtrack is synthesized by `music.mjs`. Nothing is downloaded.

## Rebuild it

You need Node 20+, ffmpeg and Google Chrome.

1. **Film the app** (only when the app's look changes). Start a throwaway CloudBuddy with the demo agent and an empty database. The demo agent replays a recorded deploy through the real gate and never runs a command.

   ```bash
   CLOUDBUDDY_PORT=7421 CLOUDBUDDY_HOME=/tmp/cb-promo \
   CLOUDBUDDY_DB="postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/<empty database>?sslmode=disable" \
   bin/cloudbuddy serve --no-open --fake testdata/scenarios/demo.json
   node promo/capture.mjs http://127.0.0.1:7421 <any empty folder>
   ```

   This writes `promo/footage/`. On screen the demo agent reads "Claude Code", and the folder and port read `~/code/shipyard-web` and 7420. Don't film the Connections page: it shows the real logins on your computer.

2. **Make the soundtrack:** `node promo/music.mjs` writes `promo/out/music.wav`.
3. **Render:** `node promo/render.mjs video` writes `promo/out/cloudbuddy-promo.mp4` (1920×1080, 30 fps, H.264 and AAC). To check single frames, run `node promo/render.mjs stills 12 61.5`, or open `video.html?t=61.5` through any local web server.

`timeline.mjs` holds the times both the picture and the sound use. Change a moment there and both follow.

## Dashboards demo (60 seconds)

The same question asked three mornings in a row; then CloudBuddy's answer: ask once, and a dashboard keeps the numbers fresh with no agent and no tokens. The agent checks its queries read-only and saves "Shop pulse"; the page shows a month of history; a script waits for one OK; and the command list shows what runs by itself. Its files are in `promo/dash/`, and it shares the renderer, the styles and the synthesizer (`lib/synth.mjs`) with the promo.

1. **Make the shop and film the app.** `setup.mjs` makes a made-up shop (a SQLite database of 1,204 invented users, an API status file, a revenue script) and a month of history next to it. The demo agent's script is `dash/scenario.json`. Use an empty database: the capture adds the made-up history to it.

   ```bash
   node promo/dash/setup.mjs /tmp/cb-dash/shop-web
   CLOUDBUDDY_PORT=7421 CLOUDBUDDY_HOME=/tmp/cb-dash/home \
   CLOUDBUDDY_DB="postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/<empty database>?sslmode=disable" \
   bin/cloudbuddy serve --no-open --fake promo/dash/scenario.json
   node promo/dash/capture.mjs http://127.0.0.1:7421 /tmp/cb-dash/shop-web <empty database>
   ```

   This writes `promo/dash/footage/`, with where the clicked buttons are in `manifest.json`. The dashboard's commands really run, against the made-up shop.
2. **Make the soundtrack:** `node promo/dash/music.mjs` writes `promo/out/dash-music.wav`.
3. **Render:** `node promo/render.mjs video --cut dash` writes `promo/out/cloudbuddy-dashboards.mp4`. Single frames: `node promo/render.mjs stills --cut dash 30.5`.

`dash/timeline.mjs` holds the times both the picture and the sound use.
