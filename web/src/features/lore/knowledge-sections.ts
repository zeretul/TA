import {
  Building2,
  Globe2,
  Library,
  MapPin,
  UserRound,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { LoreItem } from '@/lib/api'

export type LoreType = LoreItem['type']
export type LoreLoadModeFilter = 'all' | 'resident' | 'on_demand'

export interface KnowledgeSection {
  id: string
  labelKey: string
  icon: LucideIcon
  types: LoreType[]
  createType: LoreType
  /** 新建条目默认名称的 i18n key */
  createNameKey: string
  tag?: string
  excludeTag?: string
}

/**
 * The catalogue's fixed groups, shared by SettingPanel and the directory.
 *
 * Rules are not one of them. The world pack's own world book and the bound
 * gameplay configuration remain the single source for rules, and the model
 * still reads them; the library is an authoring view of concrete entities, so
 * it shows people, places, factions, background and items instead of a rule
 * list with editable bodies. `world` stays because a world entry is background
 * prose, not a rule, and dropping it would hide real content.
 */
export const KNOWLEDGE_SECTIONS: KnowledgeSection[] = [
  {
    id: 'characters',
    labelKey: 'lore.type.character',
    icon: UserRound,
    types: ['character'],
    createType: 'character',
    createNameKey: 'settingPanel.lore.newCharacter',
  },
  {
    id: 'locations',
    labelKey: 'lore.type.location',
    icon: MapPin,
    types: ['location'],
    createType: 'location',
    createNameKey: 'settingPanel.lore.newLocation',
  },
  {
    id: 'factions',
    labelKey: 'lore.type.faction',
    icon: Building2,
    types: ['faction'],
    createType: 'faction',
    createNameKey: 'settingPanel.lore.newFaction',
  },
  {
    id: 'world',
    labelKey: 'settingPanel.section.world',
    icon: Globe2,
    types: ['world'],
    createType: 'world',
    createNameKey: 'settingPanel.lore.newWorld',
  },
  {
    id: 'items',
    labelKey: 'settingPanel.section.items',
    icon: Library,
    types: ['item', 'other'],
    createType: 'item',
    createNameKey: 'settingPanel.lore.newItem',
  },
]

/** 按分组归属 + 加载策略 + 搜索词过滤资料条目。 */
export function sectionItems(
  items: LoreItem[],
  section: KnowledgeSection,
  query = '',
  loadModeFilter: LoreLoadModeFilter = 'all',
) {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (!section.types.includes(item.type)) return false
    if (loadModeFilter === 'resident' && item.load_mode !== 'resident')
      return false
    if (loadModeFilter === 'on_demand' && item.load_mode === 'resident')
      return false
    const tags = item.tags || []
    if (section.tag && !tags.includes(section.tag)) return false
    if (section.excludeTag && tags.includes(section.excludeTag)) return false
    if (normalizedQuery) {
      const haystack = [
        item.name,
        item.brief_description || '',
        item.content || '',
        tags.join('\n'),
      ]
        .join('\n')
        .toLowerCase()
      if (!haystack.includes(normalizedQuery)) return false
    }
    return true
  })
}

/** 目录中第一个可见条目的 id（按分组顺序），用于加载后的默认选中。 */
export function firstVisibleLoreItemId(items: LoreItem[]): string | null {
  for (const section of KNOWLEDGE_SECTIONS) {
    const first = sectionItems(items, section)[0]
    if (first) return first.id
  }
  return null
}
