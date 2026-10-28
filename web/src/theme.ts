/** The app's colours: follow the computer's setting, or always light or always dark. Remembered in this browser. */
export type Theme = 'system' | 'light' | 'dark'

export const THEME_KEY = 'cloudbuddy.theme'

/** The remembered choice, stored as JSON like the rest of useStoredState. Anything unreadable means "system". */
export function storedTheme(): Theme {
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(THEME_KEY) ?? 'null')
    return value === 'light' || value === 'dark' ? value : 'system'
  } catch {
    return 'system'
  }
}

/** "system" removes the attribute, so tokens.css follows prefers-color-scheme and changes with the computer. */
export function applyTheme(theme: Theme) {
  const root = document.documentElement
  if (theme === 'light' || theme === 'dark') root.dataset.theme = theme
  else delete root.dataset.theme
}
