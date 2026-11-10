import { closeMobilePanes } from '@/components/layout/mobile-pane-events'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BookMarked, Bot, Database, Image as ImageIcon, Images, Search, SlidersHorizontal, Sparkles, Trash2, UserRound, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/lib/toast'
import { APIError, clearLoreItemImage, createAgentCommandID, deleteProjectLoreItem, generateLoreItemImage, getProjectLoreBook, getProjectLoreItems, projectFileAssetURL, readProjectFile, uploadLoreItemImage, type LoreItem } from '@/lib/api'
import { rebaseJSONValue, rebaseText } from '@/lib/three-way-rebase'
import { rebaseJSONWithRecovery, rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { LoadingState } from '@/components/common/LoadingState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { ResourceWorkspace } from '@/components/layout/resource-workspace'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import type { ResourceDirectoryBadge, ResourceDirectoryItem, ResourceDirectorySection } from '@/components/resource-directory/types'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { getImagePresets, getInteractiveStories, updateInteractiveStory } from '../api'
import type { ImagePreset, LocalEntitySettings } from '../types'
import { DEFAULT_IMAGE_PRESET_ID } from '../image-settings'
import { CreatorDirectory, CreatorEditor } from './setting-panel/CreatorEditor'
import { LoreEditor } from './setting-panel/LoreEditor'
import { LocalEntryPicture } from './setting-panel/LocalEntryPicture'
import { LocalEntrySettings } from './setting-panel/LocalEntrySettings'
import { EntryDetail } from './lore-overview/LoreOverviewPanel'
import { personAvatarSource } from './person-avatar'
import { buildLocalEntities, filterLoreEntries, type LoreEntry, type LoreSectionKey } from './lore-overview/model'
import { useLoreOverview } from './lore-overview/use-lore-overview'
import { loreImportanceLabel, loreLoadModeLabel, loreTypeLabel } from '@/features/lore/options'
import { presetActionButtonClassName as actionButtonClassName, presetIconActionClassName as iconActionClassName } from './preset-config/editor-styles'
import { loreAutosaveDraft, useLoreItemAutosave, type LoreAutosaveDraft } from '@/features/lore/use-lore-item-autosave'
import { hasLoreProtagonistTag } from '@/features/lore/tags'
import { LORE_UPDATED_EVENT, notifyLoreUpdated, type LoreUpdatedDetail } from '@/features/lore/events'
import { useProjectFileAutosave } from './setting-panel/use-project-file-autosave'
import { firstVisibleLoreItemId, KNOWLEDGE_SECTIONS, sectionItems, type LoreLoadModeFilter, type LoreType } from '@/features/lore/knowledge-sections'
import { isProjectChangeForProject, type WorkspaceChangeEvent } from '@/features/changes/types'
import type { DocumentReviewController, DocumentReviewNavigationIntent } from '@/features/document-review/controller'
import type { DocumentReviewSnapshot } from '@/components/Editor/documentReviewAnchors'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'
import type { AssistantHandoff } from '../assistant-handoff'

const CREATOR_PATH = 'CREATOR.md'
const CREATOR_ENTRY_ID = '__creator__'
const UTF8_ENCODER = new TextEncoder()

export type SettingPanelMode = 'lore' | 'creator'

const LORE_TYPE_FILTER_OPTIONS: LoreType[] = ['character', 'location', 'faction', 'world', 'item', 'other']
const EMPTY_IMAGE_PRESETS: ImagePreset[] = []
type LoreImageBusyAction = 'generate' | 'upload' | 'clear'

/**
 * 本局新建 files its entities under the same four categories the world's own
 * entries use, so a person is a person on both pages. 世界背景 is absent because
 * the story registers no background entry of its own: that prose belongs to the
 * world, and 世界包原生 is where it is edited.
 */
const LOCAL_LIBRARY_SECTIONS: { id: LoreSectionKey; labelKey: string; icon: LucideIcon }[] = KNOWLEDGE_SECTIONS
  .filter((section): section is typeof section & { id: LoreSectionKey } => (
    section.id === 'characters' || section.id === 'locations' || section.id === 'factions' || section.id === 'items'
  ))
  .map((section) => ({ id: section.id, labelKey: section.labelKey, icon: section.icon }))

interface SettingPanelProps {
  mode?: SettingPanelMode
  projectId: string
  /** The story whose branch the library is showing; empty when no game is open. */
  storyId?: string
  /** The branch those story facts belong to. */
  branchId?: string
  imagePresets?: ImagePreset[]
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  documentReview?: DocumentReviewController
  documentReviewNavigationIntent?: DocumentReviewNavigationIntent | null
  refreshSignal?: number
  embedded?: boolean
  onFlushHandlerChange?: (handler: (() => Promise<boolean>) | null) => void
  toolNavigationIntent?: ToolNavigationIntent | null
  /** Hands the entry being edited to the one assistant, which has its own navigation destination. */
  onOpenAssistant?: (handoff: AssistantHandoff) => void
}

export function SettingPanel({
  mode,
  projectId,
  storyId,
  branchId,
  imagePresets = EMPTY_IMAGE_PRESETS,
  onImagePresetsChange,
  documentReview,
  documentReviewNavigationIntent,
  refreshSignal = 0,
  embedded = false,
  onFlushHandlerChange,
  toolNavigationIntent,
  onOpenAssistant,
}: SettingPanelProps) {
  return <LoreSettingPanel mode={mode || 'lore'} projectId={projectId} storyId={storyId} branchId={branchId} imagePresets={imagePresets} onImagePresetsChange={onImagePresetsChange} documentReview={documentReview} documentReviewNavigationIntent={documentReviewNavigationIntent} refreshSignal={refreshSignal} embedded={embedded} onFlushHandlerChange={onFlushHandlerChange} toolNavigationIntent={toolNavigationIntent} onOpenAssistant={onOpenAssistant} />
}

function LoreSettingPanel({
  mode,
  projectId,
  storyId,
  branchId,
  imagePresets: externalImagePresets,
  onImagePresetsChange,
  documentReview,
  documentReviewNavigationIntent,
  refreshSignal,
  embedded,
  onFlushHandlerChange,
  toolNavigationIntent,
  onOpenAssistant,
}: {
  mode: SettingPanelMode
  projectId: string
  storyId?: string
  branchId?: string
  imagePresets: ImagePreset[]
  onImagePresetsChange?: (presets: ImagePreset[]) => void
  documentReview?: DocumentReviewController
  documentReviewNavigationIntent?: DocumentReviewNavigationIntent | null
  refreshSignal: number
  embedded: boolean
  onFlushHandlerChange?: (handler: (() => Promise<boolean>) | null) => void
  toolNavigationIntent?: ToolNavigationIntent | null
  onOpenAssistant?: (handoff: AssistantHandoff) => void
}) {
  const { t } = useTranslation()
  const activeMode = mode
  const [items, setItems] = useState<LoreItem[]>([])
  const [worldScope, setWorldScope] = useState('')
  const [worldName, setWorldName] = useState('')
  /**
   * Which of the library's two pages is showing. 世界包原生 holds the selected
   * world's own entries; 本局新建 holds what this story registered. The split is
   * a read of the same two stores, never a second copy of either.
   */
  const [libraryPage, setLibraryPage] = useState<'native' | 'local'>('native')
  // 本局新建's two player-owned fields. Read from the story summary, written back
  // as a whole map, so the page and the turn read the same record.
  const [localEntitySettings, setLocalEntitySettings] = useState<Record<string, LocalEntitySettings>>({})
  const [localEntitySettingsSaving, setLocalEntitySettingsSaving] = useState(false)
  const [localEntitySettingsError, setLocalEntitySettingsError] = useState('')
  const [loading, setLoading] = useState(Boolean(projectId))
  const [loadError, setLoadError] = useState<string | null>(null)
  const [activeId, setActiveId] = useState('')
  const [draft, setDraft] = useState<LoreItem | null>(null)
  const [tagDraft, setTagDraft] = useState('')
  const [query, setQuery] = useState('')
  const [loadModeFilter, setLoadModeFilter] = useState<LoreLoadModeFilter>('all')
  const [creatorContent, setCreatorContent] = useState('')
  const [creatorRevision, setCreatorRevision] = useState('')
  const [creatorProjectId, setCreatorProjectId] = useState('')
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>(externalImagePresets)
  const [activeImagePresetId, setActiveImagePresetId] = useState('')
  const [loreImageInstruction, setLoreImageInstruction] = useState('')
  const [loreImageGenerationMode, setLoreImageGenerationMode] = useState<'agent' | 'custom'>('agent')
  const [loreImageBusy, setLoreImageBusy] = useState<{ itemId: string; action: LoreImageBusyAction } | null>(null)
  const [loreImageBatchOpen, setLoreImageBatchOpen] = useState(false)
  const [loreImageBatchSelectedIds, setLoreImageBatchSelectedIds] = useState<string[]>([])
  const [loreImageBatchQuery, setLoreImageBatchQuery] = useState('')
  const [loreImageBatchType, setLoreImageBatchType] = useState<LoreType | 'all'>('all')
  const [loreImageBatchPresetId, setLoreImageBatchPresetId] = useState('')
  const [loreImageBatchInstruction, setLoreImageBatchInstruction] = useState('')
  const [loreImageBatchOverwrite, setLoreImageBatchOverwrite] = useState(false)
  const [deleteLoreTarget, setDeleteLoreTarget] = useState<LoreItem | null>(null)
  const [saving, setSaving] = useState(false)
  const loreDraftRef = useRef<LoreItem | null>(null)
  const loreTagDraftRef = useRef('')
  const loreBaselineDraftRef = useRef<LoreAutosaveDraft | null>(null)
  const creatorContentRef = useRef('')
  const creatorBaselineContentRef = useRef('')
  const creatorBaselineRevisionRef = useRef('')
  const loreRebaseSequenceRef = useRef(0)
  const refreshSignalRef = useRef(refreshSignal)
  const isCreatorActive = activeMode === 'creator' || (activeMode === 'lore' && activeId === CREATOR_ENTRY_ID)
  const documentReviewLoreID = useMemo(() => {
    if (!documentReview || !documentReviewNavigationIntent) return ''
    const target = documentReview.comments.find((comment) => comment.id === documentReviewNavigationIntent.commentID)?.target
    return target?.kind === 'lore_item' ? target.id : ''
  }, [documentReview, documentReviewNavigationIntent])
  creatorContentRef.current = creatorContent

  const selectedLoreBaseline = useMemo<LoreAutosaveDraft | null>(() => {
    const item = items.find((entry) => entry.id === activeId)
    return item ? loreAutosaveDraft(item) : null
  }, [activeId, items])
  const residentLoreBytes = useMemo(() => {
    const persistedBytes = items
      .filter((item) => item.id !== draft?.id && item.enabled !== false && item.load_mode === 'resident')
      .reduce((total, item) => total + UTF8_ENCODER.encode((item.content || '').trim()).length, 0)
    if (draft?.enabled === false || draft?.load_mode !== 'resident') return persistedBytes
    return persistedBytes + UTF8_ENCODER.encode((draft.content || '').trim()).length
  }, [draft, items])
  const loreAutosave = useLoreItemAutosave({
    draft,
    tagDraft,
    baseline: selectedLoreBaseline,
    active: activeMode === 'lore'
      && Boolean(draft)
      && activeId !== CREATOR_ENTRY_ID,
    projectId,
    onSaved: (item, submitted) => {
      setItems((current) => current.map((entry) => entry.id === item.id ? item : entry))
      const currentDraft = loreDraftRef.current
      const savedBaseline = loreAutosaveDraft(item)
      const currentAutosaveDraft = currentDraft?.id === item.id
        ? { ...currentDraft, tags: [...(currentDraft.tags || [])], tag_draft: loreTagDraftRef.current }
        : submitted
      const rebased = rebaseJSONValue(submitted, currentAutosaveDraft, savedBaseline)
      const { tag_draft: nextTagDraft, ...nextDraft } = rebased
      setDraft(nextDraft)
      setTagDraft(nextTagDraft)
      loreBaselineDraftRef.current = savedBaseline
    },
    onAutoSaveError: (error) => {
      console.warn('[lore-editor] failed to autosave lore item', error)
      toast.error(error instanceof Error ? error.message : t('editor.saveFailed'))
    },
  })

  const creatorAutosave = useProjectFileAutosave({
    projectId,
    path: CREATOR_PATH,
    content: creatorContent,
    revision: creatorRevision,
    fileProjectId: creatorProjectId,
    active: isCreatorActive,
    onSaved: (saved, submitted) => {
      if (saved.project_id !== projectId) return
      creatorBaselineContentRef.current = saved.content
      creatorBaselineRevisionRef.current = saved.updated_at || ''
      setCreatorContent((current) => current === submitted.content ? saved.content : current)
      setCreatorRevision(saved.updated_at || '')
    },
    onAutoSaveError: (error) => {
      console.error('[creator-editor] failed to autosave CREATOR.md', error)
      toast.error((error as Error).message || t('editor.saveFailed'))
    },
  })

  const reconcileCreatorFile = useCallback(async (file: Awaited<ReturnType<typeof readProjectFile>>) => {
    if (file.project_id !== projectId) return
    const fileContent = file.content || ''
    const previousBaseline = creatorBaselineContentRef.current
    const previousRevision = creatorBaselineRevisionRef.current
    const capturedDraft = creatorContentRef.current
    let rebasedContent = await rebaseTextWithRecovery({
      resource: 'project_file',
      scope: file.project_id,
      id: CREATOR_PATH,
      baseline: { revision: previousRevision, value: previousBaseline },
      local: { revision: previousRevision, value: capturedDraft },
      external: { revision: file.revision, value: fileContent },
    })
    if (creatorContentRef.current !== capturedDraft) {
      rebasedContent = rebaseText(capturedDraft, creatorContentRef.current, rebasedContent)
    }
    creatorAutosave.resetBaseline({
      id: CREATOR_PATH,
      content: fileContent,
      project_id: file.project_id,
      updated_at: file.revision || '',
    })
    creatorBaselineContentRef.current = fileContent
    creatorBaselineRevisionRef.current = file.revision || ''
    setCreatorContent(rebasedContent)
    setCreatorRevision(file.revision || '')
    setCreatorProjectId(file.project_id)
  }, [creatorAutosave.resetBaseline, projectId])

  const loadLoreItems = useCallback(async () => {
    if (!projectId) {
      setItems([])
      setWorldScope('')
      setWorldName('')
      setActiveId('')
      setLoadError(null)
      setLoading(false)
      return
    }
    setLoading(true)
    setLoadError(null)
    try {
      const book = await getProjectLoreBook(projectId)
      setItems(book.items)
      setWorldScope(book.worldScope)
      setWorldName(book.worldName)
      // Select the first visible lore item; an empty catalog is handled by the empty state.
      setActiveId(firstVisibleLoreItemId(book.items) ?? '')
    } catch (error) {
      setItems([])
      setWorldScope('')
      setWorldName('')
      setActiveId('')
      setLoadError(error instanceof Error ? error.message : String(error))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    setItems([])
    setActiveId('')
    setDraft(null)
    setTagDraft('')
    loreBaselineDraftRef.current = null
    setQuery('')
    void loadLoreItems()
  }, [loadLoreItems])

  useEffect(() => {
    const sequence = loreRebaseSequenceRef.current + 1
    loreRebaseSequenceRef.current = sequence
    const item = items.find((entry) => entry.id === activeId) || null
    const nextBaseline = item ? loreAutosaveDraft(item) : null
    const currentDraft = loreDraftRef.current
    const previousBaseline = loreBaselineDraftRef.current
    const currentAutosaveDraft = currentDraft && item && currentDraft.id === item.id
      ? { ...currentDraft, tags: [...(currentDraft.tags || [])], tag_draft: loreTagDraftRef.current }
      : null
    void (async () => {
      if (
        currentDraft
        && previousBaseline?.id === currentDraft.id
        && !items.some((entry) => entry.id === currentDraft.id)
      ) {
        await rebaseJSONWithRecovery<LoreAutosaveDraft | null>({
          resource: 'lore_item',
          scope: projectId,
          id: currentDraft.id,
          baseline: { revision: previousBaseline.updated_at, value: previousBaseline },
          local: {
            revision: previousBaseline.updated_at,
            value: { ...currentDraft, tags: [...(currentDraft.tags || [])], tag_draft: loreTagDraftRef.current },
          },
          external: { revision: 'deleted', value: null },
        })
      }
      let rebasedFromDraft = currentDraft
      let rebasedFromTagDraft = loreTagDraftRef.current
      let rebasedFromAutosaveDraft = currentAutosaveDraft
      let rebased = nextBaseline
        ? previousBaseline?.id === nextBaseline.id && currentAutosaveDraft
          ? await rebaseJSONWithRecovery({
              resource: 'lore_item',
              scope: projectId,
              id: nextBaseline.id,
              baseline: { revision: previousBaseline.updated_at, value: previousBaseline },
              local: { revision: previousBaseline.updated_at, value: currentAutosaveDraft },
              external: { revision: nextBaseline.updated_at, value: nextBaseline },
            })
          : nextBaseline
        : null
      while (
        sequence === loreRebaseSequenceRef.current
        && rebased
        && rebasedFromAutosaveDraft?.id === rebased.id
      ) {
        const latestDraft = loreDraftRef.current
        const latestTagDraft = loreTagDraftRef.current
        if (!latestDraft || latestDraft.id !== rebased.id) break
        if (Object.is(latestDraft, rebasedFromDraft) && latestTagDraft === rebasedFromTagDraft) break
        const latestAutosaveDraft = {
          ...latestDraft,
          tags: [...(latestDraft.tags || [])],
          tag_draft: latestTagDraft,
        }
        rebased = await rebaseJSONWithRecovery({
          resource: 'lore_item',
          scope: projectId,
          id: rebased.id,
          baseline: { revision: rebasedFromAutosaveDraft.updated_at, value: rebasedFromAutosaveDraft },
          local: { revision: rebasedFromAutosaveDraft.updated_at, value: latestAutosaveDraft },
          external: { revision: nextBaseline?.updated_at, value: rebased },
        })
        rebasedFromDraft = latestDraft
        rebasedFromTagDraft = latestTagDraft
        rebasedFromAutosaveDraft = latestAutosaveDraft
      }
      if (sequence !== loreRebaseSequenceRef.current) return
      if (rebased) {
        const { tag_draft: nextTagDraft, ...nextDraft } = rebased
        setDraft(nextDraft)
        setTagDraft(nextTagDraft)
      } else {
        setDraft(null)
        setTagDraft('')
      }
      loreBaselineDraftRef.current = nextBaseline
    })().catch((error) => console.error('[lore-editor] failed to reconcile external lore update', error))
    return () => {
      if (loreRebaseSequenceRef.current === sequence) loreRebaseSequenceRef.current += 1
    }
  }, [activeId, items, projectId])

  useEffect(() => {
    loreDraftRef.current = draft
    loreTagDraftRef.current = tagDraft
  }, [draft, tagDraft])

  useEffect(() => {
    if (!isCreatorActive) return
    let cancelled = false
    creatorContentRef.current = ''
    creatorBaselineContentRef.current = ''
    creatorBaselineRevisionRef.current = ''
    setCreatorContent('')
    setCreatorRevision('')
    setCreatorProjectId('')
    if (!projectId)
      return () => {
        cancelled = true
      }
    readProjectFile(projectId, CREATOR_PATH)
      .then(async (data) => {
        if (!cancelled) await reconcileCreatorFile(data)
      })
      .catch((error) => {
        if (!cancelled) {
          const missing = error instanceof APIError && error.status === 404
          if (missing) {
            creatorAutosave.resetBaseline({
              id: CREATOR_PATH,
              content: '',
              project_id: projectId,
              updated_at: 'missing',
            })
            creatorBaselineContentRef.current = ''
            creatorBaselineRevisionRef.current = 'missing'
          }
          setCreatorContent('')
          setCreatorRevision(missing ? 'missing' : '')
          setCreatorProjectId(missing ? projectId : '')
        }
      })
    return () => {
      cancelled = true
    }
  }, [creatorAutosave.resetBaseline, isCreatorActive, projectId, reconcileCreatorFile])

  useEffect(() => {
    const onWorkspaceChange = (event: Event) => {
      const detail = (event as CustomEvent<WorkspaceChangeEvent>).detail
      if (!isProjectChangeForProject(detail, projectId)) return
      const paths = detail?.paths
      if (isCreatorActive && (!paths || paths.includes(CREATOR_PATH))) {
        void readProjectFile(projectId, CREATOR_PATH)
          .then(reconcileCreatorFile)
          .catch((error) => console.warn('[creator-editor] failed to reload external CREATOR.md update', error))
      }
    }
    window.addEventListener('nova:workspace-change', onWorkspaceChange)
    return () => window.removeEventListener('nova:workspace-change', onWorkspaceChange)
  }, [isCreatorActive, projectId, reconcileCreatorFile])

  useEffect(() => {
    if (activeMode !== 'lore' || onImagePresetsChange || externalImagePresets.length > 0) return
    let cancelled = false
    getImagePresets()
      .then((data) => {
        if (cancelled) return
        setImagePresets(data)
        setActiveImagePresetId((current) => current || data[0]?.id || '')
      })
      .catch(() => {
        if (!cancelled) setImagePresets([])
      })
    return () => {
      cancelled = true
    }
  }, [activeMode, externalImagePresets.length, onImagePresetsChange])

  useEffect(() => {
    setImagePresets(externalImagePresets)
    setActiveImagePresetId((current) => {
      if (current && externalImagePresets.some((preset) => preset.id === current)) return current
      return externalImagePresets[0]?.id || ''
    })
  }, [externalImagePresets])

  const refreshItems = useCallback(async (nextActiveId?: string) => {
    const book = await getProjectLoreBook(projectId)
    setItems(book.items)
    setWorldScope(book.worldScope)
    setWorldName(book.worldName)
    // Preserve an existing selection, including virtual entries, then fall back to the first visible item.
    setActiveId((current) => {
      if (nextActiveId && book.items.some((item) => item.id === nextActiveId)) return nextActiveId
      if (current === CREATOR_ENTRY_ID) return current
      if (current && book.items.some((item) => item.id === current)) return current
      return firstVisibleLoreItemId(book.items) ?? ''
    })
  }, [projectId])

  useEffect(() => {
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<LoreUpdatedDetail>).detail
      if (detail?.projectId !== projectId) return
      void refreshItems(detail.ids?.[0])
    }
    window.addEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
    return () => window.removeEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
  }, [projectId, refreshItems])

  useEffect(() => {
    if (refreshSignalRef.current === refreshSignal) return
    refreshSignalRef.current = refreshSignal
    void refreshItems()
  }, [refreshItems, refreshSignal])

  const mergeSavedLoreItem = (item: LoreItem) => {
    setItems((current) => current.map((entry) => (entry.id === item.id ? item : entry)))
    if (loreDraftRef.current?.id === item.id) {
      const { tag_draft: nextTagDraft, ...nextDraft } = loreAutosaveDraft(item)
      setDraft(nextDraft)
      setTagDraft(nextTagDraft)
      loreBaselineDraftRef.current = { ...nextDraft, tag_draft: nextTagDraft }
    }
  }

  const handleDelete = () => {
    if (!draft) return
    setDeleteLoreTarget(draft)
  }

  const confirmDeleteLoreTarget = async () => {
    if (!deleteLoreTarget) return
    setSaving(true)
    try {
      await flushLoreAutosave()
      loreAutosave.cancelPending()
      await deleteProjectLoreItem(projectId, deleteLoreTarget.id)
      await refreshItems()
      notifyLoreUpdated({ projectId, ids: [deleteLoreTarget.id] })
      setDeleteLoreTarget(null)
    } finally {
      setSaving(false)
    }
  }

  const flushLoreAutosave = useCallback(async (force = false) => {
    const pending = loreAutosave.flushPending()
    if (pending) return pending
    if (force || loreAutosave.status === 'error') return loreAutosave.saveNow(force ? 'manual' : 'auto')
    return null
  }, [loreAutosave.flushPending, loreAutosave.saveNow, loreAutosave.status])

  /**
   * 本局新建 reads the committed story, not the lore store. A person the story
   * invented, a place or faction it recorded, and the protagonist's own holdings
   * are all on the formal save path already, so this page only decides which of
   * them it lists. Copying any of them into a lore entry would be the second
   * store the design forbids.
   *
   * The read is the player-known projection, so a fact the character never
   * learned is already gone before this runs.
   */
  const localOverview = useLoreOverview({
    storyId: storyId || '',
    branchId: branchId || '',
    active: activeMode === 'lore' && libraryPage === 'local' && Boolean(storyId),
    withSnapshot: true,
  })
  const localEntities = useMemo(() => buildLocalEntities({
    snapshot: localOverview.data?.snapshot ?? null,
    characters: localOverview.data?.characters ?? null,
    journey: localOverview.data?.journey ?? null,
    quests: localOverview.data?.quests ?? null,
    events: localOverview.data?.events ?? null,
  }), [localOverview.data])
  const localEntryById = useMemo(() => {
    const index = new Map<string, LoreEntry>()
    for (const entries of Object.values(localEntities)) {
      for (const entry of entries) index.set(entry.id, entry)
    }
    return index
  }, [localEntities])
  const activeLocalEntry = localEntryById.get(activeId) ?? null

  /**
   * 本局新建's settings are the story's, not this component's, so they are read
   * from the story index rather than kept in a page-local draft. Reading them
   * lazily (only while that page is open) keeps the request off every other tab.
   */
  useEffect(() => {
    if (activeMode !== 'lore' || libraryPage !== 'local' || !storyId) return
    let cancelled = false
    void getInteractiveStories()
      .then((index) => {
        if (cancelled) return
        const story = index.stories.find((candidate) => candidate.id === storyId)
        setLocalEntitySettings(story?.local_entity_settings ?? {})
        setLocalEntitySettingsError('')
      })
      .catch((error: unknown) => {
        if (cancelled) return
        setLocalEntitySettingsError((error as Error).message || t('settingPanel.localSettings.loadFailed'))
      })
    return () => {
      cancelled = true
    }
  }, [activeMode, libraryPage, storyId, t])

  const saveLocalEntitySettings = useCallback(async (entityId: string, next: LocalEntitySettings) => {
    const merged = { ...localEntitySettings }
    // A record that decides nothing is removed rather than stored empty: the
    // file must not claim a decision the player never made.
    if ((next.tags?.length ?? 0) === 0 && !next.load_mode) {
      delete merged[entityId]
    } else {
      merged[entityId] = next
    }
    setLocalEntitySettingsSaving(true)
    try {
      await updateInteractiveStory(storyId || '', { local_entity_settings: merged })
      setLocalEntitySettings(merged)
      setLocalEntitySettingsError('')
    } catch (error) {
      setLocalEntitySettingsError((error as Error).message || t('settingPanel.localSettings.saveFailed'))
    } finally {
      setLocalEntitySettingsSaving(false)
    }
  }, [localEntitySettings, storyId, t])

  const changeLibraryPage = useCallback(async (page: 'native' | 'local') => {
    if (page === libraryPage) return
    try {
      if (activeId === CREATOR_ENTRY_ID) {
        await (creatorAutosave.flushPending() ?? creatorAutosave.saveNow('auto'))
      } else {
        await flushLoreAutosave()
      }
    } catch (error) {
      console.error('[lore-library] failed to flush autosave before switching pages', error)
      toast.error((error as Error).message || t('editor.saveFailed'))
      return
    }
    setLibraryPage(page)
    // The two pages list different things under different ids, so a selection
    // from the page just left would name nothing on the page now showing.
    setActiveId(page === 'native' ? firstVisibleLoreItemId(items) ?? '' : '')
  }, [activeId, creatorAutosave.flushPending, creatorAutosave.saveNow, flushLoreAutosave, items, libraryPage, t])

  const flushActiveAutosave = useCallback(async () => {
    try {
      if (activeMode === 'creator' || (activeMode === 'lore' && activeId === CREATOR_ENTRY_ID)) {
        await (creatorAutosave.flushPending() ?? creatorAutosave.saveNow('manual'))
        return true
      }
      if (activeMode === 'lore' && activeId === CREATOR_ENTRY_ID) {
        await (creatorAutosave.flushPending() ?? creatorAutosave.saveNow('manual'))
        return true
      }
      const item = await flushLoreAutosave()
      if (item) {
        notifyLoreUpdated({ projectId, ids: [item.id] })
      }
      return true
    } catch (err) {
      toast.error((err as Error).message || t('editor.saveFailed'))
      return false
    }
  }, [
    activeId,
    activeMode,
    creatorAutosave.flushPending,
    creatorAutosave.saveNow,
    flushLoreAutosave,
    projectId,
    t,
  ])

  const prepareLoreReviewSnapshot = useCallback(async (): Promise<DocumentReviewSnapshot> => {
    const itemID = loreDraftRef.current?.id
    if (!itemID || !(await flushActiveAutosave())) {
      throw new Error('The lore draft could not be saved')
    }
    const canonical = (await getProjectLoreItems(projectId)).find((item) => item.id === itemID)
    if (!canonical?.updated_at) {
      throw new Error('The canonical lore snapshot is unavailable')
    }
    setItems((current) => current.map((item) => item.id === canonical.id ? canonical : item))
    return { content: canonical.content || '', revision: canonical.updated_at }
  }, [flushActiveAutosave, projectId])

  useEffect(() => {
    onFlushHandlerChange?.(flushActiveAutosave)
    return () => onFlushHandlerChange?.(null)
  }, [flushActiveAutosave, onFlushHandlerChange])

  const handleSelectLore = useCallback(async (id: string) => {
    if (id === activeId) { closeMobilePanes(); return }
    try {
      if (activeId === CREATOR_ENTRY_ID) {
        await (creatorAutosave.flushPending() ?? creatorAutosave.saveNow('auto'))
      } else {
        await flushLoreAutosave()
      }
      setActiveId(id)
      closeMobilePanes()
    } catch (error) {
      console.error('[lore-editor] failed to flush autosave before switching resources', error)
      toast.error((error as Error).message || t('editor.saveFailed'))
    }
  }, [activeId, creatorAutosave.flushPending, creatorAutosave.saveNow, flushLoreAutosave, t])

  useEffect(() => {
    if (!documentReviewLoreID || documentReviewLoreID === activeId || !items.some((item) => item.id === documentReviewLoreID)) return
    void handleSelectLore(documentReviewLoreID)
  }, [activeId, documentReviewLoreID, handleSelectLore, items])

  useEffect(() => {
    const target = toolNavigationIntent?.target
    if (!target || target.kind !== 'lore_item') return
    const targetID = target.id || items.find((item) => item.name === target.name)?.id || ''
    if (!targetID || targetID === activeId || !items.some((item) => item.id === targetID)) return
    void handleSelectLore(targetID)
  }, [activeId, handleSelectLore, items, toolNavigationIntent?.nonce])

  const selectedLoreImagePresetId = () => activeImagePresetId || imagePresets.find((preset) => !preset.invalid)?.id || DEFAULT_IMAGE_PRESET_ID
  const handleGenerateLoreImage = async () => {
    if (!draft || loreImageBusy) return
    setLoreImageBusy({ itemId: draft.id, action: 'generate' })
    try {
      const saved = await flushLoreAutosave()
      const target = saved || loreDraftRef.current || draft
      const item = await generateLoreItemImage(projectId, target.id, {
        mode: loreImageGenerationMode,
        command_id: loreImageGenerationMode === 'agent' ? createAgentCommandID() : undefined,
        instruction: loreImageGenerationMode === 'agent' ? loreImageInstruction : undefined,
        prompt: loreImageGenerationMode === 'custom' ? loreImageInstruction : undefined,
        image_preset_id: loreImageGenerationMode === 'agent' ? selectedLoreImagePresetId() : undefined,
      })
      mergeSavedLoreItem(item)
      notifyLoreUpdated({ projectId, ids: [item.id] })
      toast.success(t('settingPanel.loreImage.generated'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.loreImage.failed'))
    } finally {
      setLoreImageBusy(null)
    }
  }

  const handleUploadLoreImage = async (file: File) => {
    if (!draft || loreImageBusy) return
    setLoreImageBusy({ itemId: draft.id, action: 'upload' })
    try {
      const saved = await flushLoreAutosave()
      const target = saved || loreDraftRef.current || draft
      const item = await uploadLoreItemImage(projectId, target.id, file)
      mergeSavedLoreItem(item)
      notifyLoreUpdated({ projectId, ids: [item.id] })
      toast.success(t('settingPanel.loreImage.uploaded'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.loreImage.uploadFailed'))
    } finally {
      setLoreImageBusy(null)
    }
  }

  const handleClearLoreImage = async () => {
    if (!draft || loreImageBusy) return
    setLoreImageBusy({ itemId: draft.id, action: 'clear' })
    try {
      const saved = await flushLoreAutosave()
      const target = saved || loreDraftRef.current || draft
      const item = await clearLoreItemImage(projectId, target.id)
      mergeSavedLoreItem(item)
      notifyLoreUpdated({ projectId, ids: [item.id] })
      toast.success(t('settingPanel.loreImage.cleared'))
    } catch (err) {
      toast.error((err as Error).message || t('settingPanel.loreImage.failed'))
    } finally {
      setLoreImageBusy(null)
    }
  }

  const handleOpenLoreImageBatch = () => {
    setLoreImageBatchSelectedIds([])
    setLoreImageBatchPresetId(selectedLoreImagePresetId())
    setLoreImageBatchOpen(true)
  }

  // The assistant lives in Game settings, so the entry hands it the item the
  // player is looking at instead of opening a second workbench beside the editor.
  // The story rides along so the assistant writes into this story's world rather
  // than whichever one the settings page happens to show.
  const openAssistantForDraft = (instruction = '') => {
    if (!onOpenAssistant) return
    onOpenAssistant({
      key: `lore-${instruction ? 'task' : 'entry'}-${Date.now()}`,
      instruction,
      resourceId: draft?.id || 'lore',
      storyId: storyId || undefined,
      context: {
        active_lore_id: draft?.id || '',
        active_lore_name: draft?.name || '',
        item_count: String(items.length),
      },
    })
  }

  // Collecting a story entity into the world it was played in is an author-side
  // write, so it goes through the same assistant as every other edit. The entry's
  // identity rides along as context rather than as a command: the assistant reads
  // the story's facts, decides what the world should hold, and asks before filing
  // anything the player did not ask for.
  const openAssistantForLocalEntry = (entry: LoreEntry) => {
    if (!onOpenAssistant) return
    const name = entry.titleKey ? t(entry.titleKey) : entry.title
    onOpenAssistant({
      key: `lore-collect-${entry.id}-${Date.now()}`,
      instruction: t('settingPanel.library.collectInstruction', { name }),
      resourceId: entry.id,
      storyId: storyId || undefined,
      context: {
        active_lore_id: entry.id,
        active_lore_name: name,
        item_count: String(localEntryById.size),
      },
    })
  }

  const handleRunLoreImageBatch = () => {
    if (loreImageBatchSelectedIds.length === 0) {
      toast.error(t('settingPanel.loreImage.noSelection'))
      return
    }
    openAssistantForDraft(buildLoreImageBatchAgentInstruction({
      itemIds: loreImageBatchSelectedIds,
      imagePresetId: loreImageBatchPresetId || selectedLoreImagePresetId(),
      instruction: loreImageBatchInstruction,
      overwriteExisting: loreImageBatchOverwrite,
    }))
    setLoreImageBatchOpen(false)
  }

  const activeAutosaveStatus = isCreatorActive
    ? creatorAutosave.status
    : loreAutosave.status
  const activeAutosaveError = isCreatorActive
    ? creatorAutosave.error
    : loreAutosave.error
  const editorHeaderIcon = libraryPage === 'local' ? UserRound : isCreatorActive ? BookMarked : Database
  const editorHeaderTitle = libraryPage === 'local'
    ? (activeLocalEntry?.titleKey ? t(activeLocalEntry.titleKey) : activeLocalEntry?.title || t('settingPanel.library.localPage'))
    : isCreatorActive
      ? t('settingPanel.workspaceNarrative.title')
      : editorTitle(activeMode, draft, t)
  const editorHeaderSubtitle = libraryPage === 'local'
    ? (activeLocalEntry?.subtitleKey ? t(activeLocalEntry.subtitleKey) : activeLocalEntry?.subtitle || t('settingPanel.library.localHint'))
    : isCreatorActive
      ? t('settingPanel.workspaceNarrative.subtitle')
      : editorSubtitle(draft, t)
  const loadModeFilterLabel = loadModeFilter === 'resident'
    ? t('settingPanel.lore.loadModeFilter.resident')
    : loadModeFilter === 'on_demand'
      ? t('settingPanel.lore.loadModeFilter.onDemand')
      : t('settingPanel.lore.loadModeFilter.all')
  const loadModeFilterAriaLabel = `${t('settingPanel.lore.loadModeFilter')}: ${loadModeFilterLabel}`
  /**
   * 世界包原生 lists this world's own book plus the shared entries every world
   * reads. Entries filed under another world's scope belong to a world the
   * author is not looking at, and listing them here would mix two settings.
   * With no world open there is nothing to narrow by, so everything shows.
   */
  const pageItems = useMemo(() => {
    if (libraryPage !== 'native') return []
    const scope = worldScope.trim()
    if (!scope) return items
    return items.filter((item) => {
      const itemScope = (item.scope || '').trim()
      return itemScope === '' || itemScope === scope
    })
  }, [items, libraryPage, worldScope])
  const loreDirectorySections: ResourceDirectorySection[] = libraryPage === 'local'
    ? LOCAL_LIBRARY_SECTIONS.map((section) => {
      const entries = filterLoreEntries(localEntities[section.id], query)
      return {
        id: section.id,
        label: t(section.labelKey),
        icon: section.icon,
        items: entries.map((entry) => localEntryToDirectoryItem(entry, projectId, t)),
      }
    })
    : KNOWLEDGE_SECTIONS.map((section) => ({
      id: section.id,
      label: t(section.labelKey),
      icon: section.icon,
      items: sectionItems(pageItems, section, query, loadModeFilter).map((item) => loreItemToDirectoryItem(item, projectId, t)),
    }))
  const loreLoadModeFilterControl = (
    <Select value={loadModeFilter} onValueChange={(value) => setLoadModeFilter(value as LoreLoadModeFilter)}>
      <SelectTrigger
        size="sm"
        className={cn(
          'size-7 justify-center border-0 p-0 shadow-none [&>svg:last-child]:hidden',
          loadModeFilter !== 'all' && 'bg-muted text-foreground',
        )}
        aria-label={loadModeFilterAriaLabel}
      >
        <SlidersHorizontal />
        <span className="sr-only">{loadModeFilterLabel}</span>
      </SelectTrigger>
      <SelectContent position="popper" align="end">
        <SelectGroup>
          <SelectItem value="all">{t('settingPanel.lore.loadModeFilter.all')}</SelectItem>
          <SelectItem value="resident">{t('settingPanel.lore.loadModeFilter.resident')}</SelectItem>
          <SelectItem value="on_demand">{t('settingPanel.lore.loadModeFilter.onDemand')}</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  )
  const loreDirectoryActions = (
    <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || items.length === 0} onClick={handleOpenLoreImageBatch} aria-label={t('settingPanel.loreImage.batchOpen')}>
      <Images data-icon="inline-start" />
    </Button>
  )
  // The two pages are one library with two sources, so the switch sits above the
  // search rather than in a menu: an author has to see which page is showing.
  const libraryPageSwitcher = (
    <div className="flex flex-col gap-1 pb-1">
      <ToggleGroup
        type="single"
        size="sm"
        variant="outline"
        value={libraryPage}
        onValueChange={(value) => {
          if (value === 'native' || value === 'local') void changeLibraryPage(value)
        }}
        aria-label={t('settingPanel.library.pageSwitcher')}
        className="w-full"
      >
        <ToggleGroupItem value="native" className="min-w-0 flex-1 text-[11px]">
          <span className="truncate">{t('settingPanel.library.nativePage')}</span>
        </ToggleGroupItem>
        <ToggleGroupItem value="local" className="min-w-0 flex-1 text-[11px]" disabled={!storyId}>
          <span className="truncate">{t('settingPanel.library.localPage')}</span>
        </ToggleGroupItem>
      </ToggleGroup>
      <span className="truncate text-[10px] text-[var(--nova-text-faint)]">
        {libraryPage === 'native'
          ? (worldName ? t('settingPanel.library.nativeHint', { world: worldName }) : t('settingPanel.library.nativeHintNoWorld'))
          : t('settingPanel.library.localHint')}
      </span>
    </div>
  )
  const localLibraryEmpty = (
    <EmptyState
      icon={Database}
      title={t('settingPanel.library.localEmptyTitle')}
      description={t('settingPanel.library.localEmptyDescription')}
      variant="panel"
    />
  )
  const directoryPanel = (
    <div className="nova-sidebar flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      {activeMode === 'lore' ? (
        libraryPage === 'native' && loading ? (
          <LoadingState label={t('common.loading')} variant="panel" className="h-full min-h-0" />
        ) : libraryPage === 'native' && loadError ? (
          <div className="flex flex-col gap-2 p-3">
            <InlineErrorNotice message={loadError} />
            <Button variant="outline" size="sm" onClick={() => void loadLoreItems()}>
              {t('common.retry')}
            </Button>
          </div>
        ) : (
          <ResourceDirectory
            sections={loreDirectorySections}
            activeId={activeId || null}
            onSelect={handleSelectLore}
            saving={saving}
            searchPlaceholder={libraryPage === 'native' ? t('settingPanel.searchLore') : t('settingPanel.library.searchLocal')}
            query={query}
            onQueryChange={setQuery}
            filterItem={() => true}
            searchAccessory={libraryPage === 'native' ? loreLoadModeFilterControl : undefined}
            headerActions={libraryPage === 'native' ? loreDirectoryActions : undefined}
            headerContent={libraryPageSwitcher}
            emptyContent={libraryPage === 'local' ? localLibraryEmpty : undefined}
            emptySectionsLast
            hideEmptySections
          />
        )
      ) : <CreatorDirectory />}
    </div>
  )

  if (embedded && activeMode === 'creator') {
    return <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b p-3">
        <p className="text-xs text-muted-foreground">{t('settingPanel.workspaceNarrative.subtitle')}</p>
        <AutosaveStatusIndicator status={activeAutosaveStatus} error={activeAutosaveError} onRetry={flushActiveAutosave} />
      </div>
      <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={flushActiveAutosave} />
    </div>
  }

  return (
    <section className="h-full min-h-0 bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
      <ResourceWorkspace
        title={panelTitle(activeMode, t)}
        embedded={embedded}
        left={{
          id: 'setting-directory',
          title: panelTitle(activeMode, t),
          side: 'left',
          icon: <ModeIcon mode={activeMode} />,
          content: directoryPanel,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: embedded ? 'w-[min(86vw,320px)]' : 'w-[min(90vw,360px)]',
        }}
        className="h-full"
        mainClassName="min-h-0 min-w-0"
        leftResize={{
          layoutKey: embedded ? 'nova-embedded-setting-directory-layout' : 'nova-setting-directory-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: embedded ? '224px' : '320px',
          minSize: embedded ? '180px' : '220px',
          maxSize: '42%',
        }}
      >
        {() => (
          <main className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <FeaturePageShell
              icon={editorHeaderIcon}
              title={editorHeaderTitle}
              subtitle={editorHeaderSubtitle}
              onSaveShortcut={flushActiveAutosave}
              actions={(
                <>
                  {libraryPage === 'local' ? null : isCreatorActive || draft ? (
                    <AutosaveStatusIndicator
                      status={activeAutosaveStatus}
                      error={activeAutosaveError}
                      onRetry={flushActiveAutosave}
                    />
                  ) : null}
                  {activeMode === 'lore' && libraryPage === 'native' && !isCreatorActive && draft && (
                    <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving} onClick={handleDelete} aria-label={t('settingPanel.deleteLore')}>
                      <Trash2 data-icon="inline-start" />
                    </Button>
                  )}
                  {libraryPage === 'local' ? null : (
                    <Button
                      className={cn(iconActionClassName, 'shrink-0')}
                      variant="outline"
                      size="sm"
                      disabled={!onOpenAssistant}
                      onClick={() => openAssistantForDraft()}
                      aria-label={t('settingPanel.loreAgent.title')}
                      title={t('settingPanel.loreAgent.title')}
                    >
                      <Bot data-icon="inline-start" />
                      <span className="hidden sm:inline">{t('settingPanel.loreAgent.title')}</span>
                    </Button>
                  )}
                </>
              )}
              className="bg-[var(--nova-surface-2)] text-[var(--nova-text)]"
              topbarClassName="min-h-12"
            >
              {activeMode === 'lore' ? (
                <>
                  {libraryPage === 'local' ? (
                    localOverview.loading ? (
                      <LoadingState label={t('common.loading')} className="h-full min-h-0" />
                    ) : localOverview.error ? (
                      <div className="flex flex-col gap-2 p-3">
                        <InlineErrorNotice message={localOverview.error} />
                        <Button variant="outline" size="sm" onClick={() => localOverview.reload()}>
                          {t('common.retry')}
                        </Button>
                      </div>
                    ) : activeLocalEntry ? (
                      <EntryDetail
                        entry={activeLocalEntry}
                        projectId={projectId}
                        onOpenLink={(entryId) => setActiveId(entryId)}
                        onBack={() => setActiveId('')}
                        onCollect={onOpenAssistant ? () => openAssistantForLocalEntry(activeLocalEntry) : undefined}
                        picture={storyId
                          ? <LocalEntryPicture storyId={storyId} branchId={branchId} entry={activeLocalEntry} />
                          : undefined}
                        settings={storyId
                          ? (
                            <LocalEntrySettings
                              entry={activeLocalEntry}
                              settings={localEntitySettings[activeLocalEntry.id] ?? {}}
                              saving={localEntitySettingsSaving}
                              error={localEntitySettingsError || undefined}
                              onSave={(next) => { void saveLocalEntitySettings(activeLocalEntry.id, next) }}
                            />
                          )
                          : undefined}
                      />
                    ) : (
                      <EmptyState
                        icon={Database}
                        title={storyId ? t('settingPanel.library.localPickTitle') : t('settingPanel.library.localNoStoryTitle')}
                        description={storyId ? t('settingPanel.library.localPickDescription') : t('settingPanel.library.localNoStoryDescription')}
                        variant="page"
                      />
                    )
                  ) : loading ? (
                    <LoadingState label={t('common.loading')} className="h-full min-h-0" />
                  ) : items.length === 0 && !loadError && !activeId ? (
                    <EmptyState
                      icon={Database}
                      title={t('settingPanel.lore.emptyTitle')}
                      description={t('settingPanel.lore.emptyDescription')}
                      action={onOpenAssistant
                        ? { label: t('settingPanel.lore.emptyAction'), onClick: () => openAssistantForDraft() }
                        : undefined}
                      variant="page"
                    />
                  ) : activeId === CREATOR_ENTRY_ID ? (
                    <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={flushActiveAutosave} />
                  ) : (
                    <LoreEditor
                      projectId={projectId}
                      draft={draft}
                      tagDraft={tagDraft}
                      residentTotalBytes={residentLoreBytes}
                      imagePresets={imagePresets}
                      imagePresetId={selectedLoreImagePresetId()}
                      imageInstruction={loreImageInstruction}
                      imageGenerationMode={loreImageGenerationMode}
                      imageBusyAction={loreImageBusy && loreImageBusy.itemId === draft?.id ? loreImageBusy.action : ''}
                      searchQuery={query}
                      setDraft={setDraft}
                      setTagDraft={setTagDraft}
                      onImagePresetChange={setActiveImagePresetId}
                      setImageInstruction={setLoreImageInstruction}
                      onImageGenerationModeChange={setLoreImageGenerationMode}
                      onGenerateImage={() => void handleGenerateLoreImage()}
                      onUploadImage={(file) => void handleUploadLoreImage(file)}
                      onClearImage={() => void handleClearLoreImage()}
                      onSave={flushActiveAutosave}
                      documentReview={documentReview}
                      documentReviewNavigationIntent={documentReviewNavigationIntent}
                      onPrepareReviewSnapshot={prepareLoreReviewSnapshot}
                    />
                  )}
                </>
              ) : (
                <CreatorEditor content={creatorContent} setContent={setCreatorContent} onSave={flushActiveAutosave} />
              )}
            </FeaturePageShell>
          </main>
        )}
      </ResourceWorkspace>
      <LoreImageBatchDialog
        open={loreImageBatchOpen}
        projectId={projectId}
        items={items}
        query={loreImageBatchQuery}
        type={loreImageBatchType}
        selectedIds={loreImageBatchSelectedIds}
        imagePresets={imagePresets.filter((preset) => !preset.invalid)}
        imagePresetId={loreImageBatchPresetId || selectedLoreImagePresetId()}
        instruction={loreImageBatchInstruction}
        overwriteExisting={loreImageBatchOverwrite}
        onOpenChange={setLoreImageBatchOpen}
        onQueryChange={setLoreImageBatchQuery}
        onTypeChange={setLoreImageBatchType}
        onSelectedIdsChange={setLoreImageBatchSelectedIds}
        onImagePresetChange={setLoreImageBatchPresetId}
        onInstructionChange={setLoreImageBatchInstruction}
        onOverwriteExistingChange={setLoreImageBatchOverwrite}
        onRun={handleRunLoreImageBatch}
      />
      <ConfirmDialog
        open={Boolean(deleteLoreTarget)}
        onOpenChange={(open) => {
          if (!open && !saving) setDeleteLoreTarget(null)
        }}
        title={t('settingPanel.deleteLore')}
        description={t('settingPanel.confirmDeleteLore', { name: deleteLoreTarget?.name || '' })}
        confirmLabel={t('common.delete')}
        tone="danger"
        onConfirm={confirmDeleteLoreTarget}
      />
    </section>
  )
}

interface LoreImageBatchDialogProps {
  open: boolean
  projectId: string
  items: LoreItem[]
  query: string
  type: LoreType | 'all'
  selectedIds: string[]
  imagePresets: ImagePreset[]
  imagePresetId: string
  instruction: string
  overwriteExisting: boolean
  onOpenChange: (open: boolean) => void
  onQueryChange: (value: string) => void
  onTypeChange: (value: LoreType | 'all') => void
  onSelectedIdsChange: (ids: string[]) => void
  onImagePresetChange: (id: string) => void
  onInstructionChange: (value: string) => void
  onOverwriteExistingChange: (value: boolean) => void
  onRun: () => void
}

function LoreImageBatchDialog({
  open,
  projectId,
  items,
  query,
  type,
  selectedIds,
  imagePresets,
  imagePresetId,
  instruction,
  overwriteExisting,
  onOpenChange,
  onQueryChange,
  onTypeChange,
  onSelectedIdsChange,
  onImagePresetChange,
  onInstructionChange,
  onOverwriteExistingChange,
  onRun,
}: LoreImageBatchDialogProps) {
  const { t } = useTranslation()
  const selectedSet = new Set(selectedIds)
  const filteredItems = filterLoreImageBatchItems(items, query, type)

  const toggleSelected = (id: string) => {
    onSelectedIdsChange(selectedSet.has(id) ? selectedIds.filter((entry) => entry !== id) : [...selectedIds, id])
  }

  const selectVisible = () => {
    const next = new Set(selectedIds)
    filteredItems.forEach((item) => next.add(item.id))
    onSelectedIdsChange(Array.from(next))
  }

  const clearSelection = () => {
    onSelectedIdsChange([])
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      onOpenChange(nextOpen)
    }}>
      <DialogContent className="max-w-[min(calc(100vw-2rem),760px)] gap-3 border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle>{t('settingPanel.loreImage.batchTitle')}</DialogTitle>
          <DialogDescription>{t('settingPanel.loreImage.batchDesc')}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px]">
          <div className="nova-field flex h-8 items-center gap-2 rounded-[var(--nova-radius)] px-2 text-xs text-[var(--nova-text-faint)]">
            <Search className="h-3.5 w-3.5" />
            <input
              className="min-w-0 flex-1 bg-transparent text-[var(--nova-text-muted)] outline-none placeholder:text-[var(--nova-text-faint)]"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder={t('settingPanel.loreImage.search')}
            />
          </div>
          <Select value={type} onValueChange={(value) => onTypeChange(value as LoreType | 'all')}>
            <SelectTrigger size="sm" className="nova-field h-8 text-xs focus:ring-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                <SelectItem value="all">{t('settingPanel.loreImage.typeAll')}</SelectItem>
                {LORE_TYPE_FILTER_OPTIONS.map((option) => (
                  <SelectItem key={option} value={option}>{loreTypeLabel(option, t)}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="text-xs text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.selectedCount', { count: selectedIds.length })}</div>
          <div className="flex items-center gap-2">
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={filteredItems.length === 0} onClick={selectVisible}>
              {t('settingPanel.loreImage.selectVisible')}
            </Button>
            <Button className={actionButtonClassName} variant="outline" size="sm" disabled={selectedIds.length === 0} onClick={clearSelection}>
              {t('settingPanel.loreImage.clearSelection')}
            </Button>
          </div>
        </div>

        <ScrollArea className="h-[min(42vh,360px)] rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <div className="divide-y divide-[var(--nova-border)]">
            {filteredItems.length === 0 ? (
              <div className="px-3 py-8 text-center text-xs text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.noItems')}</div>
            ) : filteredItems.map((item) => {
              return (
                <label key={item.id} className="flex min-h-16 cursor-pointer items-center gap-3 px-3 py-2 text-xs hover:bg-[var(--nova-hover)]">
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-[var(--nova-accent)]"
                    checked={selectedSet.has(item.id)}
                    onChange={() => toggleSelected(item.id)}
                    aria-label={item.name}
                  />
                  <LoreImageBatchThumb projectId={projectId} item={item} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-[var(--nova-text)]">{item.name}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{loreTypeLabel(item.type, t)} · {item.brief_description || t('settingPanel.loreImage.missingImage')}</span>
                  </span>
                  <span className={`shrink-0 rounded-full border px-2 py-1 text-[10px] ${loreImageStatusClassName(item)}`}>
                    {item.image?.image_path ? t('settingPanel.loreImage.hasImage') : t('settingPanel.loreImage.missingImage')}
                  </span>
                </label>
              )
            })}
          </div>
        </ScrollArea>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
          <label className="grid gap-1.5">
            <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.instruction')}</span>
            <Textarea
              className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
              value={instruction}
              onChange={(event) => onInstructionChange(event.target.value)}
              placeholder={t('settingPanel.loreImage.instructionPlaceholder')}
            />
          </label>
          <div className="grid content-start gap-3">
            <label className="grid gap-1.5">
              <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.preset')}</span>
              <Select value={imagePresetId} onValueChange={onImagePresetChange}>
                <SelectTrigger size="sm" className="nova-field h-8 text-xs focus:ring-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectGroup>
                    {imagePresets.length > 0 ? imagePresets.map((preset) => (
                      <SelectItem key={preset.id} value={preset.id}>{preset.name}</SelectItem>
                    )) : (
                      <SelectItem value={DEFAULT_IMAGE_PRESET_ID}>{t('settingPanel.editor.defaultImagePreset')}</SelectItem>
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </label>
            <div className="flex items-center justify-between gap-3 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2">
              <span className="min-w-0 text-xs text-[var(--nova-text-muted)]">{t('settingPanel.loreImage.overwriteExisting')}</span>
              <Switch checked={overwriteExisting} onCheckedChange={onOverwriteExistingChange} />
            </div>
          </div>
        </div>

        <DialogFooter className="border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={selectedIds.length === 0} onClick={onRun}>
            <Sparkles data-icon="inline-start" />
            {t('settingPanel.loreImage.startBatch')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function LoreImageBatchThumb({ projectId, item }: { projectId: string; item: LoreItem }) {
  const imagePath = item.image?.image_path || ''
  // A shipped default is not this entry's picture, so it does not change what
  // the batch does with the row: the row still counts as missing an image and is
  // still generated. It only gives the author something to recognize the person
  // by while choosing, which is why the dashed border stays.
  const src = imagePath ? projectFileAssetURL(projectId, imagePath) : item.portrait_url || ''
  if (!src) {
    return (
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]">
        <ImageIcon className="h-4 w-4" />
      </span>
    )
  }
  return (
    <span className={cn('h-10 w-10 shrink-0 overflow-hidden rounded-md border bg-[var(--nova-surface)]', imagePath ? 'border-[var(--nova-border)]' : 'border-dashed border-[var(--nova-border)]')}>
      <img src={src} alt="" className="h-full w-full object-cover" />
    </span>
  )
}

function filterLoreImageBatchItems(items: LoreItem[], query: string, type: LoreType | 'all') {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (type !== 'all' && item.type !== type) return false
    if (!normalizedQuery) return true
    const haystack = [item.name, item.brief_description || '', item.content || '', (item.tags || []).join('\n')].join('\n').toLowerCase()
    return haystack.includes(normalizedQuery)
  })
}

function buildLoreImageBatchAgentInstruction(input: {
  itemIds: string[]
  imagePresetId: string
  instruction: string
  overwriteExisting: boolean
}) {
  const userInstruction = input.instruction.trim() || 'No additional user requirements.'
  return [
    'Generate images for the selected lore items as one managed task.',
    `Exact lore item IDs: ${JSON.stringify(input.itemIds)}`,
    `Image preset ID: ${JSON.stringify(input.imagePresetId)}`,
    `Overwrite existing images: ${input.overwriteExisting ? 'yes' : 'no'}.`,
    `Additional user requirements: ${userInstruction}`,
    '',
    'Read the exact lore items with read_lore_items. Read the selected image_preset with config_read when available.',
    'For each eligible item, author a complete final model-native prompt from its lore content, the image preset, the additional requirements, and the prompt guide in generate_image. Then call generate_image once with purpose=lore_item and that exact lore_item_id.',
    'When overwrite is no, skip items that already have an image. Continue after an individual failure and report generated, skipped, and failed item IDs at the end. Do not create or edit lore text. Do not add a negative prompt.',
  ].join('\n')
}

function loreImageStatusClassName(item: LoreItem) {
  if (item.image?.image_path) return 'border-[var(--nova-accent-green)]/35 bg-[var(--nova-accent-green)]/10 text-[var(--nova-text-muted)]'
  return 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]'
}

function ModeIcon({ mode }: { mode: SettingPanelMode }) {
  if (mode === 'creator') return <BookMarked className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
  return <Database className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
}

/**
 * 本局新建's directory rows. The page lists what the story registered, so a row
 * carries the same facts the overview shows and nothing the author could edit
 * here: an entity the story created is not a lore entry, and letting the library
 * rewrite it would be a second store for the same fact.
 */
function localEntryToDirectoryItem(entry: LoreEntry, projectId: string, t: (key: string) => string): ResourceDirectoryItem {
  const badges: ResourceDirectoryBadge[] = [{
    label: t('settingPanel.library.localBadge'),
    tone: 'outline',
  }]
  return {
    id: entry.id,
    title: entry.titleKey ? t(entry.titleKey) : entry.title,
    summary: entry.subtitleKey ? t(entry.subtitleKey) : entry.subtitle,
    thumbnailUrl: entry.tab === 'people'
      ? personAvatarSource(entry.avatar, entry.avatarLorePath, projectId) || null
      : null,
    searchText: [entry.summary || '', ...(entry.keywords || [])].join('\n'),
    badges,
  }
}

function loreItemToDirectoryItem(item: LoreItem, projectId: string, t: (key: string) => string): ResourceDirectoryItem {
  const imagePath = item.image?.image_path || ''
  const badges: ResourceDirectoryBadge[] = [{
    label: item.load_mode === 'resident' ? t('settingPanel.lore.loadModeBadge.resident') : t('settingPanel.lore.loadModeBadge.onDemand'),
    title: loreLoadModeLabel(item.load_mode, t),
    tone: item.load_mode === 'resident' ? 'default' : 'outline',
  }]
  if (item.type === 'character' && hasLoreProtagonistTag(item.tags || [])) {
    badges.unshift({ label: t('loreWorkspace.protagonistTag'), tone: 'warning' })
  }
  if (item.enabled === false) {
    badges.push({ label: t('settingPanel.disabled'), tone: 'muted' })
  }
  return {
    id: item.id,
    title: item.name,
    thumbnailUrl: imagePath ? projectFileAssetURL(projectId, imagePath) : item.portrait_url || null,
    badges,
    disabled: item.enabled === false,
  }
}

function panelTitle(mode: SettingPanelMode, t: (key: string) => string) {
  if (mode === 'creator') return t('settingPanel.mode.creator')
  return t('settingPanel.mode.lore')
}

function editorTitle(mode: SettingPanelMode, draft: LoreItem | null, t: (key: string) => string) {
  if (mode === 'creator') return t('settingPanel.workspaceNarrative.title')
  return draft?.name || t('settingPanel.mode.lore')
}

function editorSubtitle(draft: LoreItem | null, t: (key: string) => string) {
  if (!draft) return t('settingPanel.editor.loreSubtitle')
  return `${draft.enabled === false ? t('settingPanel.disabled') : t('settingPanel.enabled')} · ${loreTypeLabel(draft.type, t)} · ${loreImportanceLabel(draft.importance, t)} · ${loreLoadModeLabel(draft.load_mode, t)} · ${(draft.tags || []).join('，') || t('settingPanel.editor.noTags')}`
}
