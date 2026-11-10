import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { ProvenanceDetails } from './ProvenanceDetails'
import { AffiliationList } from './AffiliationList'
import { knownProvenance } from './known-provenance'

it('labels hearsay, preserves the actual learning time and shows only references', () => {
  render(<ProvenanceDetails value={knownProvenance({ learned_at: '昨日午时', source: 'conversation', nature: 'hearsay', confidence: 'confirmed', evidence_refs: ['public-exchange-7'], evidence: 'secret-text' })} />)
  expect(screen.getByText('昨日午时')).toBeInTheDocument()
  expect(screen.getByText('交流获知')).toBeInTheDocument()
  expect(screen.getByText('传闻')).toBeInTheDocument()
  expect(screen.getByText('尚未确认')).toBeInTheDocument()
  expect(screen.getByText('对话记录：public-exchange-7')).toBeInTheDocument()
  expect(screen.queryByText('secret-text')).not.toBeInTheDocument()
})

it('marks missing provenance as unrecorded, not as confirmed', () => {
  render(<ProvenanceDetails />)
  expect(screen.getAllByText('未记录')).toHaveLength(4)
  expect(screen.queryByText('已确认')).not.toBeInTheDocument()
})

it('distinguishes an ended application from a current membership and retains its reason', () => {
  render(<AffiliationList affiliations={[{ factionId: 'faction-east', name: '同名宗门', status: 'left', statement: '申请已被拒绝' }, { name: '同名宗门', status: 'unresolved' }]} onOpen={vi.fn()} />)
  expect(screen.getByText('已结束／不在籍')).toBeInTheDocument()
  expect(screen.getByText('申请已被拒绝')).toBeInTheDocument()
  expect(screen.getByText('势力身份未核实')).toBeInTheDocument()
  expect(screen.getAllByRole('button')).toHaveLength(1)
  expect(screen.getByText('势力ID：faction-east')).toBeInTheDocument()
})
