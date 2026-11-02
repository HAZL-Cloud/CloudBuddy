import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import { keys } from './queryKeys'
import type { ConnectionsResponse, DashboardDetail, Decision, ProjectInput, Rule, Settings, Step } from './types'

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
    mutationFn: (input: { text: string; projectId?: string }) => api.createConversation(input),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: keys.conversations })
      void client.invalidateQueries({ queryKey: keys.projects })
    },
  })
}

export function useSettings() {
  return useQuery({ queryKey: keys.settings, queryFn: api.settings })
}

export function useSaveSettings() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (input: Settings) => api.saveSettings(input),
    onSuccess: (saved) => client.setQueryData(keys.settings, saved),
  })
}

export function useProjects() {
  return useQuery({ queryKey: keys.projects, queryFn: api.projects })
}

/** Creates the project when there is no id, otherwise updates it. */
export function useSaveProject() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string | null; input: ProjectInput }) =>
      id ? api.updateProject(id, input) : api.createProject(input),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.projects }),
  })
}

export function useDeleteProject() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.deleteProject(id),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: keys.projects })
      void client.invalidateQueries({ queryKey: keys.conversations })
      void client.invalidateQueries({ queryKey: keys.conversationsAll })
    },
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

// ---------- dashboards (D-067) ----------

export function useDashboards() {
  return useQuery({ queryKey: keys.dashboards, queryFn: api.dashboards })
}

/** Keeps the last page on screen while another time range loads. */
export function useDashboard(id: string | null, range: string) {
  return useQuery({
    queryKey: keys.dashboardRange(id ?? '', range),
    queryFn: () => api.dashboard(id as string, range),
    enabled: id !== null,
    placeholderData: (prev) => prev,
    // the page's "updated 2 min ago" and charts stay current between refreshes
    refetchInterval: 60_000,
  })
}

function useDashboardMutation<V>(fn: (v: V) => Promise<DashboardDetail | void>) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ['dashboard'] })
      void client.invalidateQueries({ queryKey: keys.dashboards })
    },
  })
}

export function useDashboardSchedule() {
  return useDashboardMutation(({ id, refresh, paused }: { id: string; refresh: string; paused: boolean }) =>
    api.setDashboardSchedule(id, { refresh, paused }),
  )
}

export function useRefreshDashboard() {
  return useDashboardMutation((id: string) => api.refreshDashboard(id))
}

// Never optimistic, like approvals: the page shows what the server says after it answers.
export function useDecideDashboardSource() {
  return useDashboardMutation(({ id, key, decision }: { id: string; key: string; decision: 'allow' | 'deny' }) =>
    api.decideDashboardSource(id, key, decision),
  )
}

export function useDeleteDashboard() {
  return useDashboardMutation((id: string) => api.deleteDashboard(id))
}
