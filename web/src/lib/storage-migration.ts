// Browser storage keys carry the product prefix. Keys written under either
// earlier prefix are copied once, at startup and before any module reads them,
// so a returning browser keeps its layout, tabs and preferences instead of
// falling back to defaults. The old entries stay in place: they are what a
// rollback to the previous build reads, and copying them costs a single pass.
const LEGACY_STORAGE_PREFIXES = [['denova', '.'].join(''), ['nova', '.'].join('')]
const STORAGE_PREFIX = 'ta.'
const MIGRATION_MARKER = 'ta.migration.storagePrefix.v2'

function migrateStore(store: Storage | undefined) {
  if (!store) return
  try {
    if (store.getItem(MIGRATION_MARKER) === 'done') return
    for (const legacy of LEGACY_STORAGE_PREFIXES) {
      const keys: string[] = []
      for (let index = 0; index < store.length; index += 1) {
        const key = store.key(index)
        if (key?.startsWith(legacy)) keys.push(key)
      }
      for (const key of keys) {
        const current = STORAGE_PREFIX + key.slice(legacy.length)
        // A value already under the current prefix is newer than the old one.
        if (store.getItem(current) !== null) continue
        const value = store.getItem(key)
        if (value !== null) store.setItem(current, value)
      }
    }
    store.setItem(MIGRATION_MARKER, 'done')
  } catch {
    // Private-mode and quota failures leave the defaults in place; nothing here
    // is worth blocking startup over.
  }
}

export function migrateLegacyStorage() {
  if (typeof window === 'undefined') return
  migrateStore(window.localStorage)
  migrateStore(window.sessionStorage)
}
