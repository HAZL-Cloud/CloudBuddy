import { Icon } from './Icon'

export function NotSignedIn() {
  return (
    <main className="centered">
      <div className="empty">
        <span className="brand-mark">
          <Icon name="cloud" />
        </span>
        <b>CloudBuddy</b>
        <p>Open CloudBuddy from the link it printed in your terminal.</p>
      </div>
    </main>
  )
}
