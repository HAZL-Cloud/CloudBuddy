import { Logo, Wordmark } from './Logo'

export function NotSignedIn() {
  return (
    <main className="centered">
      <div className="empty">
        <Logo className="brand-mark" />
        <b>
          <Wordmark />
        </b>
        <p>Open CloudBuddy from the link it printed in your terminal.</p>
      </div>
    </main>
  )
}
