import type { ReactNode } from 'react'

// The mockup's 16px line icons, written as React elements so no markup is ever injected as a string.
const PATHS = {
  eye: (
    <>
      <path d="M1.5 8S3.9 3.5 8 3.5 14.5 8 14.5 8 12.1 12.5 8 12.5 1.5 8 1.5 8Z" />
      <circle cx="8" cy="8" r="2" />
    </>
  ),
  plus: <path d="M8 3v10M3 8h10" />,
  minus: <path d="M3.5 8h9" />,
  pencil: <path d="M10.5 2.8l2.7 2.7L6 12.7H3.3V10Z" />,
  trash: <path d="M2.8 4.5h10.4M6.3 4.5V2.8h3.4v1.7M4.3 4.5l.6 8.7h6.2l.6-8.7" />,
  plane: <path d="M14 2 7.2 8.8M14 2 9.6 14l-2.4-5.2L2 6.4Z" />,
  check: <path d="M3 8.5l3.2 3L13 4.8" />,
  x: <path d="M4 4l8 8M12 4l-8 8" />,
  clock: (
    <>
      <circle cx="8" cy="8" r="6" />
      <path d="M8 4.8V8l2.2 1.4" />
    </>
  ),
  hand: (
    <path d="M5.5 8V3.8a1 1 0 0 1 2 0V7.5M7.5 7V3a1 1 0 0 1 2 0v4M9.5 7.2V4a1 1 0 0 1 2 0v5c0 2.6-1.6 4.5-4 4.5-1.6 0-2.6-.8-3.4-2L2.8 8.9a1 1 0 0 1 1.6-1.1L5.5 9" />
  ),
  play: <path d="M5 3.3v9.4L12.4 8Z" fill="currentColor" stroke="none" />,
  pause: <path d="M5.5 3.5v9M10.5 3.5v9" />,
  cloud: <path d="M4.6 12.5h6.9a3 3 0 0 0 .3-6 4 4 0 0 0-7.7-.9A3.5 3.5 0 0 0 4.6 12.5Z" />,
  shield: (
    <>
      <path d="M8 1.8l5 1.9v3.9c0 3.1-2.1 5.4-5 6.6-2.9-1.2-5-3.5-5-6.6V3.7Z" />
      <path d="M5.8 8.1l1.6 1.5 2.9-3" />
    </>
  ),
  menu: <path d="M2.5 4.5h11M2.5 8h11M2.5 11.5h11" />,
  fit: <path d="M2.5 6V2.5H6M10 2.5h3.5V6M13.5 10v3.5H10M6 13.5H2.5V10" />,
  folder: <path d="M2 4.5a1 1 0 0 1 1-1h3l1.5 1.5H13a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1Z" />,
  stack: <path d="M8 2.2 14 5.3 8 8.4 2 5.3ZM2 8l6 3.1L14 8M2 10.7l6 3.1 6-3.1" />,
  panelOpen: <path d="M2.5 3.2h11v9.6h-11ZM9.8 3.2v9.6M7.4 6.2 5.6 8l1.8 1.8" />,
  panelClose: <path d="M2.5 3.2h11v9.6h-11ZM9.8 3.2v9.6M5.6 6.2 7.4 8 5.6 9.8" />,
  bot: (
    <>
      <rect x="2.5" y="5" width="11" height="8" rx="2" />
      <path d="M8 2.5V5M6 9h.01M10 9h.01" />
    </>
  ),
  send: <path d="M8 13V3.5M3.8 7.6 8 3.4l4.2 4.2" />,
  inbox: (
    <>
      <path d="M2 9.5l1.8-5.2a1 1 0 0 1 .9-.8h6.6a1 1 0 0 1 .9.8L14 9.5V12a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1Z" />
      <path d="M2 9.5h3.5l1 1.5h3l1-1.5H14" />
    </>
  ),
  chat: (
    <path d="M3 3.5h10a.5.5 0 0 1 .5.5v6.5a.5.5 0 0 1-.5.5H7l-3 2.5V11H3a.5.5 0 0 1-.5-.5V4a.5.5 0 0 1 .5-.5Z" />
  ),
  grid: <path d="M2.5 2.5h4.3v4.3H2.5ZM9.2 2.5h4.3v4.3H9.2ZM2.5 9.2h4.3v4.3H2.5ZM9.2 9.2h4.3v4.3H9.2Z" />,
  plug: <path d="M6 1.8v3M10 1.8v3M4.3 4.8h7.4v2.7a3.7 3.7 0 0 1-7.4 0ZM8 11.2v3" />,
  gear: (
    <>
      <circle cx="8" cy="8" r="2.2" />
      <path d="M8 1.8v1.6M8 12.6v1.6M14.2 8h-1.6M3.4 8H1.8M12.4 3.6l-1.1 1.1M4.7 11.3l-1.1 1.1M12.4 12.4l-1.1-1.1M4.7 4.7 3.6 3.6" />
    </>
  ),
  copy: (
    <>
      <rect x="5" y="5" width="8.5" height="8.5" rx="1.5" />
      <path d="M3 10.5V4a1.5 1.5 0 0 1 1.5-1.5H11" />
    </>
  ),
  sun: (
    <>
      <circle cx="8" cy="8" r="2.8" />
      <path d="M8 1.5v1.3M8 13.2v1.3M14.5 8h-1.3M2.8 8H1.5M12.6 3.4l-.9.9M4.3 11.7l-.9.9M12.6 12.6l-.9-.9M4.3 4.3l-.9-.9" />
    </>
  ),
  moon: <path d="M13.2 9.8A5.6 5.6 0 0 1 6.2 2.8a5.6 5.6 0 1 0 7 7Z" />,
  monitor: (
    <>
      <rect x="2" y="2.8" width="12" height="8.4" rx="1.2" />
      <path d="M5.5 13.8h5M8 11.2v2.6" />
    </>
  ),
  reset: <path d="M2.8 8a5.2 5.2 0 1 0 1.6-3.8M2.8 2.8v2.6h2.6" />,
  bolt: <path d="M9 1.5 3.5 9H8l-1 5.5L12.5 7H8Z" />,
  lock: (
    <>
      <rect x="3.5" y="7" width="9" height="6.5" rx="1.5" />
      <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
    </>
  ),
  dollar: <path d="M8 1.8v12.4M11 4.5H6.8a2 2 0 0 0 0 4h2.4a2 2 0 0 1 0 4H5" />,
  people: (
    <>
      <circle cx="6" cy="5.5" r="2.2" />
      <path d="M2 13c.4-2.3 2-3.6 4-3.6s3.6 1.3 4 3.6" />
      <circle cx="11.2" cy="6.2" r="1.7" />
      <path d="M11 9.6c1.6.1 2.7 1.2 3 3" />
    </>
  ),
  link: <path d="M6.5 9.5l3-3M7 4.5l1-1a2.5 2.5 0 0 1 3.5 3.5l-1 1M9 11.5l-1 1A2.5 2.5 0 0 1 4.5 9l1-1" />,
  question: (
    <>
      <circle cx="8" cy="8" r="6" />
      <path d="M6.3 6.2a1.8 1.8 0 0 1 3.4.8c0 1.2-1.7 1.5-1.7 2.6M8 11.6h.01" />
    </>
  ),
} satisfies Record<string, ReactNode>

export type IconName = keyof typeof PATHS

export function Icon({ name, small = false }: { name: IconName; small?: boolean }) {
  return (
    <svg
      className={small ? 'ic sm' : 'ic'}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {PATHS[name]}
    </svg>
  )
}
