export interface ContentFontScale {
  sourceEditor: number
}

type ContentFontScaleListener = () => void

const listeners = new Set<ContentFontScaleListener>()
let currentScale: ContentFontScale = {
  sourceEditor: 14,
}

export function getContentFontScale(): ContentFontScale {
  return currentScale
}

export function subscribeContentFontScale(listener: ContentFontScaleListener) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Publishes the content ramp used by canvas-backed editors. */
export function setContentFontScale(readingFontSize: number): ContentFontScale {
  const ratio = readingFontSize / 18
  const nextScale: ContentFontScale = {
    sourceEditor: roundPixels(14 * ratio),
  }
  if (nextScale.sourceEditor === currentScale.sourceEditor) return currentScale

  currentScale = nextScale
  listeners.forEach((listener) => listener())
  return currentScale
}

function roundPixels(value: number) {
  return Math.round(value * 10) / 10
}
