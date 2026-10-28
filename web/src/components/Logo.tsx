// The CloudBuddy mark (D-059): a blue cloud with a small teal buddy tucked beside it.
// Its colours stay the same in dark mode, like public/favicon.svg, which draws the same shapes.
export function Logo({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 64 64" aria-hidden="true" focusable="false">
      <rect width="64" height="64" rx="15" fill="#E1E8FA" />
      <g transform="translate(0 -1)">
        <circle cx="26" cy="33" r="13" fill="#2350C8" />
        <rect x="26" y="33" width="17" height="13" fill="#2350C8" />
        <circle cx="43" cy="38" r="10.5" fill="#E1E8FA" />
        <circle cx="43" cy="38" r="8" fill="#138A74" />
      </g>
    </svg>
  )
}

/** "CloudBuddy" with "Buddy" in the logo's teal. Screen readers still read one word. */
export function Wordmark() {
  return (
    <>
      Cloud<span className="brand-buddy">Buddy</span>
    </>
  )
}
