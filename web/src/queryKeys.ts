export const keys = {
  session: ['session'] as const,
  conversations: ['conversations'] as const,
  conversation: (id: string) => ['conversation', id] as const,
  conversationsAll: ['conversation'] as const,
  connections: ['connections'] as const,
  projects: ['projects'] as const,
  settings: ['settings'] as const,
  dashboards: ['dashboards'] as const,
  dashboard: (id: string) => ['dashboard', id] as const,
  dashboardRange: (id: string, range: string) => ['dashboard', id, range] as const,
}
