export const keys = {
  session: ['session'] as const,
  conversations: ['conversations'] as const,
  conversation: (id: string) => ['conversation', id] as const,
  conversationsAll: ['conversation'] as const,
  connections: ['connections'] as const,
}
