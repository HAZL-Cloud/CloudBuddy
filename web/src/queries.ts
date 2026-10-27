import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import { keys } from './queryKeys'
import type { AgentName, ConnectionsResponse, Decision, Rule, Step } from './types'

export function useSession() {
  return useQuery({ queryKey: keys.session, queryFn: api.session, staleTime: Infinity })
}

export function useConversations() {
  return useQuery({ queryKey: keys.conversations, queryFn: api.conversations })
}

export function useConversation(id: string | null) {
  return useQuery({
    queryKey: keys.conversation(id ?? ''),
    queryFn: () => api.conversation(id as string),
    enabled: id !== null,
  })
}

export function useConnections() {
  return useQuery({ queryKey: keys.connections, queryFn: api.connections })
}

export function useCreateConversation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (input: { agent: AgentName; workdir: string; text: string }) => api.createConversation(input),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.conversations }),
  })
}

export function useSendMessage(id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (text: string) => api.sendMessage(id, text),
    // A 409 means a run is active; refetching shows the real state either way.
    onSettled: () => {
      void client.invalidateQueries({ queryKey: keys.conversation(id) })
      void client.invalidateQueries({ queryKey: keys.conversations })
    },
  })
}

export function useStopConversation(id: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => api.stop(id),
    onSettled: () => client.invalidateQueries({ queryKey: keys.conversation(id) }),
  })
}

// Approve and deny never touch the cache before the server answers. We refetch afterwards and show what it says.
export function useDecide() {
  const client = useQueryClient()
  return useMutation<Step, Error, { approvalId: string; decision: Decision }>({
    mutationFn: ({ approvalId, decision }) => api.decide(approvalId, decision),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: keys.conversationsAll })
      void client.invalidateQueries({ queryKey: keys.conversations })
    },
  })
}

export function useSetRule() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ id, rule }: { id: string; rule: Rule }) => api.setRule(id, rule),
    onSuccess: (conn) => {
      client.setQueryData<ConnectionsResponse>(keys.connections, (old) =>
        old && {
          ...old,
          cli: old.cli.map((c) => (c.id === conn.id ? conn : c)),
          mcp: old.mcp.map((c) => (c.id === conn.id ? conn : c)),
        },
      )
    },
  })
}

export function useScanConnections() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => api.scanConnections(),
    onSuccess: (data) => client.setQueryData(keys.connections, data),
  })
}
