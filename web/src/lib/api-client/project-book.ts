import { jsonHeaders, requestJSON } from './client'
import { assertProjectScope } from './project-scope'
import type { LoreItem, LoreItemInput, WorkspaceSummary } from './types'

export interface ProjectBookFileNode {
  name: string
  type: 'file' | 'dir'
  children?: ProjectBookFileNode[]
}

export interface ProjectBookSnapshot {
  project_id: string
  workspace: string
  tree: ProjectBookFileNode[]
  summary: WorkspaceSummary
}

interface ProjectLoreItemsEnvelope {
  project_id: string
  items?: LoreItem[]
  world_scope?: string
  world_name?: string
}

/**
 * The lore library as one project reads it.
 *
 * `worldScope` is the world the author has open, or empty when no story is
 * open. It is what splits the library into its two pages: entries in this
 * scope are that world's own setting, and entries in the shared book or in
 * another world's scope are not. The browser cannot derive it from the entry
 * list alone — one world's scope looks like another's — so it comes from the
 * server.
 */
export interface ProjectLoreBook {
  items: LoreItem[]
  worldScope: string
  worldName: string
}

interface ProjectLoreItemEnvelope {
  project_id: string
  item?: LoreItem
}

function projectBookURL(projectId: string) {
  return `/api/projects/${encodeURIComponent(projectId)}/book`
}

export async function getProjectBookSnapshot(projectId: string): Promise<ProjectBookSnapshot> {
  const snapshot = await requestJSON<ProjectBookSnapshot>(projectBookURL(projectId))
  assertProjectScope(projectId, snapshot.project_id)
  return {
    ...snapshot,
    tree: Array.isArray(snapshot.tree) ? snapshot.tree : [],
    summary: {
      ...snapshot.summary,
      chapters: Array.isArray(snapshot.summary?.chapters) ? snapshot.summary.chapters : [],
      chapter_plans: Array.isArray(snapshot.summary?.chapter_plans) ? snapshot.summary.chapter_plans : [],
    },
  }
}

export async function getProjectBookTree(projectId: string): Promise<ProjectBookFileNode[]> {
  return (await getProjectBookSnapshot(projectId)).tree
}

export async function getProjectBookSummary(projectId: string): Promise<WorkspaceSummary> {
  return (await getProjectBookSnapshot(projectId)).summary
}

function projectLoreURL(projectId: string) {
  return `${projectBookURL(projectId)}/lore/items`
}

export async function getProjectLoreBook(projectId: string): Promise<ProjectLoreBook> {
  const data = await requestJSON<ProjectLoreItemsEnvelope>(projectLoreURL(projectId))
  assertProjectScope(projectId, data.project_id)
  return {
    items: data.items ?? [],
    worldScope: data.world_scope ?? '',
    worldName: data.world_name ?? '',
  }
}

export async function getProjectLoreItems(projectId: string): Promise<LoreItem[]> {
  return (await getProjectLoreBook(projectId)).items
}

export async function createProjectLoreItem(projectId: string, item: Partial<LoreItemInput>): Promise<LoreItem> {
  const response = await requestJSON<ProjectLoreItemEnvelope>(projectLoreURL(projectId), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(item),
  })
  assertProjectScope(projectId, response.project_id)
  if (!response.item) throw new Error('Invalid Project Lore item response')
  return response.item
}

export async function updateProjectLoreItem(
  projectId: string,
  id: string,
  item: LoreItemInput,
  baseRevision?: string,
): Promise<LoreItem> {
  const response = await requestJSON<ProjectLoreItemEnvelope>(`${projectLoreURL(projectId)}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...item, base_revision: baseRevision } : item),
  })
  assertProjectScope(projectId, response.project_id)
  if (!response.item) throw new Error('Invalid Project Lore item response')
  return response.item
}

export async function deleteProjectLoreItem(projectId: string, id: string): Promise<void> {
  const response = await requestJSON<{ project_id: string }>(
    `${projectLoreURL(projectId)}/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  assertProjectScope(projectId, response.project_id)
}
