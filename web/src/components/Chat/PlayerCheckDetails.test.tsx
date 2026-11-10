import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { PlayerCheckDetails } from './PlayerCheckDetails'

it('opens only the saved result and offers no reroll or other mutation', () => {
  render(<PlayerCheckDetails evidence={{ label: '身手检定', dice: '1d20', rolls: [14], total: 16, outcome: 'success', bonus_total: 2 }} />)
  const panel = screen.getByTestId('player-check-details')
  expect(panel).not.toHaveAttribute('open')
  fireEvent.click(screen.getByText('查看依据'))
  expect(panel).toHaveAttribute('open')
  expect(screen.getByText('已结算总值：16')).toBeInTheDocument()
  expect(screen.getByText('本故事公开修正：+2')).toBeInTheDocument()
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})
