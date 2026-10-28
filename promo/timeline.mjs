// Shared by the picture (video.js) and the soundtrack (music.mjs), so sounds land on the frames they belong to.
export const FPS = 30
export const DURATION = 90

export const PROMPT = '$ agent "deploy the shop site to S3 + CloudFront, then delete the old staging bucket"'

export const T = {
  promptType: [1.0, 4.0], // typing the prompt
  wallStart: 4.3, // output starts
  wallStop: 21.0, // output stops on the y/N prompt
  pressY: 28.4, // you press y
  resume: 29.4, // output resumes after y
  termOut: 32.2, // terminal fades away
  problem: 33.4, // "Agent CLIs are great at DevOps."
  problem2: 35.4, // "You just can't see the work."
  reveal: 40.0, // logo
  product: 45.8, // the app
  typing: [47.3, 48.8], // the prompt typed in CloudBuddy
  send: 49.6,
  firstCard: 51.2,
  gate: 59.4, // the delete waits
  click: 70.9, // Delete is clicked
  approved: 71.2,
  stripe: 75.0,
  replay: 79.0,
  end: 85.2, // end card
}

/** Lines per second of terminal output at time t: a trickle that turns into a flood, then stops. */
export function lineRate(t) {
  if (t < T.wallStart || t >= T.wallStop) return 0
  if (t < 7) return 1.4
  if (t < 12) return 1.4 + ((t - 7) / 5) * 8
  return 9.4 + Math.min(1, (t - 12) / 7.5) * 38
}

/** Cumulative output lines by time t (stepped at 1/240 s, so the picture and sound agree exactly). */
export function linesBy(t) {
  let n = 0
  for (let x = T.wallStart; x < Math.min(t, T.wallStop); x += 1 / 240) n += lineRate(x) / 240
  return n
}
