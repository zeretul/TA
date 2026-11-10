import { jsonHeaders, requestJSON } from '@/lib/api-client'

export interface PlayerAttention {
  v: 1
  characters: string[]
  quests: string[]
  head: string
}

export type AttentionKind = 'character' | 'quest'

export function getPlayerAttention(storyId: string, branchId: string): Promise<PlayerAttention> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/player-attention?branch=${encodeURIComponent(branchId)}`)
}

export function updatePlayerAttention(storyId: string, input: {
  branch: string; expected_head: string; kind: AttentionKind; id: string; following: boolean
}): Promise<PlayerAttention> {
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/player-attention`, {
    method: 'PUT', headers: jsonHeaders, body: JSON.stringify(input),
  })
}
