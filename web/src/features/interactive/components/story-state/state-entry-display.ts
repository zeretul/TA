import type { TFunction } from 'i18next'

/**
 * A nested state record carries two different kinds of fact.
 *
 * The character's own part — a technique's mastery, an item's count, where it
 * came from — is what the player reads. The rule layer's part — the definition
 * ID a cast resolves, the revision it was learned at, the template it was built
 * from — exists so the rules can price an action, and reading it out tells the
 * player nothing about the world.
 *
 * This module decides which keys belong to the player's view. It never changes
 * the stored record: the ledger drops these keys while rendering, and the
 * journal, the settlement and the cast path keep reading the full entry.
 */
const RULE_METADATA_KEYS = new Set(['定义', '版本', '模板', '主要属性预算'])

export function isRuleMetadataKey(key: string): boolean {
  return RULE_METADATA_KEYS.has(key.trim())
}

/**
 * Drops the rule layer's bookkeeping from one nested record.
 *
 * The outer entry still names the held item or learned skill when its details
 * contain only rule metadata. An absent description must not expose those IDs.
 */
export function playerVisibleEntries(value: Record<string, unknown>): Array<[string, unknown]> {
  const entries = Object.entries(value).filter(([, item]) => item !== undefined)
  return entries.filter(([key]) => !isRuleMetadataKey(key))
}

/**
 * The wording a nested record's key deserves.
 *
 * The pack writes these keys in the story's own language — "名称即 ID" — so most
 * read as they are. Two keys do not: a generated technique is keyed by its
 * registry ID (`gen.<template>.<name>`), and an entry the pack never named is
 * keyed by its definition ID. Both would show the rule layer's own bookkeeping,
 * so the record's own 名称 wins first, then the name the story gave the
 * technique, then the interface's catalogue for that definition.
 */
export function stateEntryLabel(key: string, entry: unknown, t: TFunction): string {
  const text = key.trim()
  if (!text) return text
  const generated = text.startsWith('gen.')
  if (!generated && containsCJK(text)) return text
  if (isRecord(entry)) {
    const name = stringValue(entry['名称'])
    if (name) return name
  }
  if (generated) {
    // `gen.<template>.<name>` — the name is the story's own wording for the art.
    const parts = text.split('.')
    if (parts.length >= 3) {
      const named = parts.slice(2).join('.').trim()
      if (named) return named
    }
  }
  return catalogName(text, t) || text
}

/**
 * The catalogue's wording for a definition ID, or '' when it holds none.
 *
 * Technique definitions are keyed `skill.<id>.name`. Item definitions are keyed
 * `item.<group>.<rest joined by _>.name`, so `pill.restore.health.lower` reads
 * as `item.pill.restore_health_lower.name`. Both spellings are tried, because a
 * definition the catalogue does not word must fall back rather than invent a
 * name for itself.
 */
export function catalogName(id: string, t: TFunction): string {
  const key = id.trim()
  if (!key) return ''
  for (const candidate of [`skill.${key}.name`, ...itemNameKeys(key)]) {
    const translated = t(candidate, { defaultValue: '' })
    if (translated && translated !== candidate) return translated
  }
  return ''
}

function itemNameKeys(id: string): string[] {
  const parts = id.split('.').filter(Boolean)
  if (parts.length < 2) return [`item.${id.replace(/\./g, '_')}.name`]
  return [
    `item.${parts[0]}.${parts.slice(1).join('_')}.name`,
    `item.${parts.join('_')}.name`,
  ]
}

function containsCJK(text: string): boolean {
  return /[㐀-鿿]/.test(text)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
