// The landing page is the front door at /. The app lives at /app (D-060).
export const APP_PATH = '/app'

export function isAppPath(pathname: string): boolean {
  return pathname === APP_PATH || pathname.startsWith(`${APP_PATH}/`)
}
