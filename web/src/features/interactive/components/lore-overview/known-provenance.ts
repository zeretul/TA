/** Provenance is read from a player-known fact, never guessed from save times. */
export interface KnownProvenance {
  learnedAt?: string
  source?: string
  sourceKey?: string
  nature?: 'experienced' | 'told' | 'hearsay' | 'conjecture' | 'formal'
  confidence?: 'confirmed' | 'uncertain'
  evidence?: string[]
  references?: { labelKey: string; id: string }[]
}

export function knownProvenance(value: Record<string, unknown>): KnownProvenance {
  const nature = natureNames[text(value.nature) || text(value['获知方式'])]
  const confidence = confidenceNames[text(value.confidence) || text(value['可信度']) || text(value['确定性'])]
  const references = value.evidence_refs
  const source = text(value.source) || text(value['获知来源']) || text(value['来源'])
  return {
    learnedAt: text(value.learned_at) || text(value['获知时刻']) || undefined,
    sourceKey: sourceNames[source] ? `loreOverview.provenance.source.${sourceNames[source]}` : undefined,
    nature,
    // A rumor cannot be upgraded by a contradictory "confirmed" label.
    confidence: nature === 'hearsay' || nature === 'conjecture' ? 'uncertain' : confidence,
    references: (Array.isArray(references) ? references : []).map(text).filter(Boolean).map(id => ({ labelKey: 'loreOverview.provenance.exchange', id })),
  }
}

const sourceNames: Record<string, string> = { observation: 'observation', conversation: 'conversation', public_record: 'public_record', 亲眼所见: 'observation', 本人介绍: 'conversation', 公开记录: 'public_record' }
const natureNames: Record<string, KnownProvenance['nature']> = {
  experienced: 'experienced', observed: 'experienced', 亲历: 'experienced', 亲眼所见: 'experienced',
  told: 'told', 告知: 'told', 本人告知: 'told', hearsay: 'hearsay', 传闻: 'hearsay',
  conjecture: 'conjecture', 推测: 'conjecture',
}
const confidenceNames: Record<string, KnownProvenance['confidence']> = {
  confirmed: 'confirmed', certain: 'confirmed', 已确认: 'confirmed', 确认: 'confirmed',
  uncertain: 'uncertain', 未确认: 'uncertain', 待确认: 'uncertain', 存疑: 'uncertain',
}
export const provenanceKeys = new Set(['learned_at', '获知时刻', 'source', '来源', '获知来源', 'nature', '获知方式', 'confidence', '可信度', '确定性', 'evidence', '证据', 'evidence_refs'])
function text(value: unknown): string { return typeof value === 'string' ? value.trim() : '' }
