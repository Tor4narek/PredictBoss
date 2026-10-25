import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './index'
import type { CompanyProfileInput, Credentials, MaterialsQuery } from './types'

/*
 * Хуки для страниц. Они берут данные через api и сами ведут состояния
 * isPending (загрузка), isError (ошибка) и data (результат).
 */

export const queryKeys = {
  session: ['session'] as const,
  profile: ['profile'] as const,
  sources: ['sources'] as const,
  materials: (query: MaterialsQuery) => ['materials', query] as const,
  chat: ['chat'] as const,
}

// ——— Вход ———

export function useSession() {
  return useQuery({ queryKey: queryKeys.session, queryFn: api.getSession, staleTime: Infinity })
}

export function useLogin() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (credentials: Credentials) => api.login(credentials),
    onSuccess: (session) => client.setQueryData(queryKeys.session, session),
  })
}

export function useRegister() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (credentials: Credentials) => api.register(credentials),
    onSuccess: (session) => {
      client.removeQueries()
      client.setQueryData(queryKeys.session, session)
    },
  })
}

export function useLogout() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      client.removeQueries()
      client.setQueryData(queryKeys.session, null)
    },
  })
}

// ——— Профиль ———

export function useProfile() {
  return useQuery({ queryKey: queryKeys.profile, queryFn: api.getProfile })
}

export function useSaveProfile() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (input: CompanyProfileInput) => api.saveProfile(input),
    onSuccess: (profile) => {
      client.setQueryData(queryKeys.profile, profile)
      // Новый профиль делает старый анализ устаревшим.
      void client.invalidateQueries({ queryKey: queryKeys.chat })
      void client.invalidateQueries({ queryKey: queryKeys.session })
    },
  })
}

// ——— Источники ———

export function useSources() {
  return useQuery({ queryKey: queryKeys.sources, queryFn: api.getSources })
}

export function useAddSource() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (url: string) => api.addSource(url),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.sources }),
  })
}

export function useSetSourcePaused() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ id, paused }: { id: string; paused: boolean }) => api.setSourcePaused(id, paused),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.sources }),
  })
}

export function useDeleteSource() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.deleteSource(id),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.sources }),
  })
}

// ——— Материалы ———

export function useMaterials(query: MaterialsQuery) {
  return useQuery({
    queryKey: queryKeys.materials(query),
    queryFn: () => api.getMaterials(query),
    // При смене фильтра или «Показать ещё» старая таблица остаётся видна, пока грузится новая.
    placeholderData: keepPreviousData,
  })
}

export function useAddMaterial() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (url: string) => api.addMaterial(url),
    onSuccess: () => client.invalidateQueries({ queryKey: ['materials'] }),
  })
}

// ——— Диалог ———

export function useChat() {
  return useQuery({ queryKey: queryKeys.chat, queryFn: api.getChat })
}

export function useSendMessage() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (text: string) => api.sendMessage(text),
    onSuccess: (chat) => client.setQueryData(queryKeys.chat, chat),
  })
}
