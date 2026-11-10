import { createContext, useContext, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { resolveWorldPresentation, type WorldPresentationSource } from './world-presentation'

const WorldPresentationContext = createContext<WorldPresentationSource | null>(null)

export function WorldPresentationProvider({ story, snapshot, children }: WorldPresentationSource & { children: ReactNode }) {
  return <WorldPresentationContext.Provider value={{ story, snapshot }}>{children}</WorldPresentationContext.Provider>
}

/** Standalone detail panels can use their own frozen snapshot or native family as a fallback. */
export function useWorldPresentation(fallback: WorldPresentationSource = {}) {
  const context = useContext(WorldPresentationContext)
  const { t, i18n } = useTranslation()
  const source = context || fallback
  const locale = i18n.resolvedLanguage || i18n.language || 'zh-CN'
  return useMemo(() => resolveWorldPresentation(source, locale, t), [source.story, source.snapshot, source.family, locale, t])
}
