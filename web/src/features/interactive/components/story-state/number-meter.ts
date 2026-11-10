import type { ActorStateField } from '../../types'

export interface NumberMeterGeometry {
  min: number
  max: number
  startPercent: number
  widthPercent: number
  zeroPercent?: number
  tone: 'standard' | 'negative' | 'positive' | 'neutral'
}

/**
 * Resolves ordinary and zero-centered bounded numbers for every state view.
 *
 * A field may declare its bound either as a fixed `max` or as `max_field`, the
 * name of a sibling whose current value is the bound. The sibling form exists
 * because the cultivation pools' maximum is a settled number of its own: 生命上限
 * moves with the character's stage, so 生命 cannot carry a fixed max without
 * clipping a late-stage character. Passing `resolveSibling` lets the caller
 * supply that sibling's current value; without it a `max_field` field simply has
 * no resolvable bound and falls back to a plain number.
 */
export function resolveNumberMeter(
  field: ActorStateField,
  value: unknown,
  resolveSibling?: (fieldName: string) => unknown,
): NumberMeterGeometry | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) return null
  const min = typeof field.min === 'number' && Number.isFinite(field.min) ? field.min : 0
  const max = resolveFieldMax(field, resolveSibling)
  if (max === null || !Number.isFinite(max) || max <= min) return null

  const position = numberPosition(value, min, max)
  if (min < 0 && max > 0) {
    const zero = numberPosition(0, min, max)
    if (value < 0) {
      return { min, max, startPercent: position, widthPercent: zero - position, zeroPercent: zero, tone: 'negative' }
    }
    if (value > 0) {
      return { min, max, startPercent: zero, widthPercent: position - zero, zeroPercent: zero, tone: 'positive' }
    }
    return { min, max, startPercent: zero, widthPercent: 0, zeroPercent: zero, tone: 'neutral' }
  }
  return { min, max, startPercent: 0, widthPercent: position, tone: 'standard' }
}

/**
 * Resolves a field's maximum, from its fixed `max` or from its declared sibling.
 *
 * A declared sibling whose value is absent, non-numeric or non-finite leaves the
 * field unbounded rather than defaulting to some scale: showing "40 / 100" for a
 * character whose real maximum is 200 would be a worse lie than showing 40.
 */
export function resolveFieldMax(
  field: ActorStateField,
  resolveSibling?: (fieldName: string) => unknown,
): number | null {
  if (typeof field.max === 'number' && Number.isFinite(field.max)) return field.max
  const siblingName = typeof field.max_field === 'string' ? field.max_field.trim() : ''
  if (!siblingName || !resolveSibling) return null
  const sibling = resolveSibling(siblingName)
  return typeof sibling === 'number' && Number.isFinite(sibling) ? sibling : null
}

function numberPosition(value: number, min: number, max: number) {
  return Math.min(100, Math.max(0, ((value - min) / (max - min)) * 100))
}
