import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { RelationshipView } from './RelationshipView'
import type { KnownPerson, KnownRelation } from './known-people'

it('pages a large graph, keeps every relation in the list and opens the exact identity', () => {
  const people: KnownPerson[] = Array.from({ length: 10 }, (_, index) => ({ id: index ? `npc-${index}` : 'protagonist', name: index ? `人物${index}` : '玩家', affiliations: [], seenNow: false }))
  const relations: KnownRelation[] = people.slice(1).map(person => ({ id: person.id, sourceId: 'protagonist', targetId: person.id, label: '同伴', mutual: false }))
  const open = vi.fn()
  render(<RelationshipView people={people} relations={relations} onOpenPerson={open} />)
  expect(screen.getByRole('group', { name: '玩家的已知关系图' })).toBeInTheDocument()
  expect(screen.getByRole('group').querySelector('line')).toHaveAttribute('stroke-dasharray', '5 4')
  expect(screen.getByRole('button', { name: '上一组' })).toBeDisabled()
  expect(screen.queryByRole('button', { name: '查看人物7' })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '下一组' }))
  expect(screen.getByRole('button', { name: '下一组' })).toBeDisabled()
  fireEvent.keyDown(screen.getByRole('button', { name: '查看人物7' }), { key: 'Enter' })
  expect(open).toHaveBeenCalledWith('npc-7')
  fireEvent.click(screen.getByRole('button', { name: '关系列表' }))
  expect(screen.getAllByText('同伴')).toHaveLength(9)
  fireEvent.click(screen.getByRole('button', { name: '关系图' }))
  fireEvent.change(screen.getByRole('combobox', { name: '以谁为中心' }), { target: { value: 'npc-2' } })
  expect(screen.getByRole('group', { name: '人物2的已知关系图' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '下一组' })).not.toBeInTheDocument()
})

it('keeps empty and disconnected people honest', () => {
  const { rerender } = render(<RelationshipView people={[]} relations={[]} onOpenPerson={() => {}} />)
  expect(screen.getByText('尚无已知的人物关系。')).toBeInTheDocument()
  rerender(<RelationshipView people={[{ id: 'a', name: '独行者', affiliations: [], seenNow: false }]} relations={[]} onOpenPerson={() => {}} />)
  expect(screen.queryByRole('group')).not.toBeInTheDocument()
  expect(screen.getByRole('combobox')).toHaveValue('a')
})
