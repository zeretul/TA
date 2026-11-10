import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { GameSaveActions } from './GameSaveActions'
import { getGameSaves, type GameSave } from '../game-saves'

vi.mock('../game-saves', () => ({ getGameSaves: vi.fn() }))
vi.mock('@/components/Home/HomeView', () => ({ HomeView: ({ onStartStory }: { onStartStory: (pack: string) => void }) => <button onClick={() => onStartStory('world-pack')}>创建新角色</button> }))
const saves = [
  { project_id: 'world-a', world_name: '凡人世界', workspace: '/world-a', story: { id: 'story-a', title: '韩立的旅程', protagonist: { name: '韩立' }, turn_count: 12 } },
  { project_id: 'world-b', world_name: '星海世界', workspace: '/world-b', story: { id: 'story-b', title: '远航', protagonist: { name: '舰长' }, turn_count: 8 } },
] as GameSave[]

beforeEach(() => { vi.mocked(getGameSaves).mockResolvedValue({ saves, unavailable: [] }) })
describe('GameSaveActions', () => {
  it('opens a cross-world save and filters by character without creating a new game', async () => {
    const user = userEvent.setup(); const open = vi.fn().mockResolvedValue(undefined); const start = vi.fn()
    render(<GameSaveActions projectId="world-a" onStartStory={start} onOpenSave={open} />)
    await user.click(screen.getByRole('button', { name: '切换存档' }))
    await screen.findByText('远航')
    await user.type(screen.getByRole('textbox', { name: '搜索角色、世界或故事' }), '舰长')
    expect(screen.queryByText('韩立的旅程')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '继续' }))
    await waitFor(() => expect(open).toHaveBeenCalledWith(saves[1]))
    expect(start).not.toHaveBeenCalled()
  })
  it('requires confirmation for the selected save, and cancel preserves it', async () => {
    const user = userEvent.setup(); const remove = vi.fn().mockResolvedValue(undefined)
    render(<GameSaveActions projectId="world-a" onStartStory={vi.fn()} onDeleteSave={remove} />)
    await user.click(screen.getByRole('button', { name: '删除存档' }))
    const row = (await screen.findByText('远航')).closest('li')!
    await user.click(within(row).getByRole('button', { name: '删除' }))
    expect(screen.getByRole('alertdialog')).toHaveTextContent('星海世界')
    expect(screen.getByRole('alertdialog')).toHaveTextContent('舰长')
    expect(remove).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: '取消' }))
    expect(remove).not.toHaveBeenCalled()
    await user.click(within(row).getByRole('button', { name: '删除' }))
    await user.click(screen.getByRole('button', { name: '确认删除' }))
    await waitFor(() => expect(remove).toHaveBeenCalledWith(saves[1]))
    await waitFor(() => expect(screen.queryByText('远航')).not.toBeInTheDocument())
    expect(screen.getByText('韩立的旅程')).toBeInTheDocument()
  })
  it('starts a new game through the existing creation flow', async () => {
    const user = userEvent.setup(); const start = vi.fn().mockResolvedValue(undefined)
    render(<GameSaveActions projectId="world-a" onStartStory={start} />)
    await user.click(screen.getByRole('button', { name: '新开一局' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('原有存档会保留')
    await user.click(await screen.findByRole('button', { name: '创建新角色' }))
    await waitFor(() => expect(start).toHaveBeenCalledWith('world-pack'))
  })
})

