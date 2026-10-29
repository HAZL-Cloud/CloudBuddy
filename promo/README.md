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
