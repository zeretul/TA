import { describe, expect, it } from 'vitest'
import { knownAffiliations } from './known-affiliations'

describe('formal membership evidence copy', () => {
  it('localizes fixed creation evidence while retaining narrative evidence', () => {
    const creation = knownAffiliations([], {}, [{ id: 'm1', faction_id: 'sect', faction_name: 'Known faction', status: 'formal', source_kind: 'identity', statement: '建角时确认的所属身份', world_time: '开局' }])[0]
    expect(creation.statementKey).toBe('loreOverview.membership.creationConfirmed')
    expect(creation.statement).toBeUndefined()
    expect(creation.provenance?.learnedAt).toBeUndefined()
    const scene = knownAffiliations([], {}, [{ id: 'm2', faction_id: 'sect', faction_name: 'Known faction', status: 'left', source_kind: 'player', statement: 'I withdrew my application.', world_time: 'Evening' }])[0]
    expect(scene.statement).toBe('I withdrew my application.')
    expect(scene.statementKey).toBeUndefined()
    expect(scene.provenance?.learnedAt).toBe('Evening')
  })
})
