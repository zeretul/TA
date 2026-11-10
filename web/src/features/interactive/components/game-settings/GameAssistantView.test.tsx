import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { GameAssistantView } from './GameAssistantView'
import { useInteractiveStore } from '../../stores/interactive-store'
import type { StorySummary } from '../../types'
import { getInteractiveStories } from '../../api'
vi.mock('../../api',()=>({getInteractiveStories:vi.fn()}))
vi.mock('@/features/settings/SettingsPageFrame',()=>({SettingsPageFrame:({children}: {children:React.ReactNode})=><div>{children}</div>}))
vi.mock('@/components/Chat/ConfigManagerChat',()=>({ConfigManagerChat:({storyId,branchId,onMutated}: {storyId?:string;branchId?:string;onMutated:()=>void})=><button data-testid="assistant-scope" data-story={storyId} data-branch={branchId} onClick={onMutated}>Refresh</button>}))
beforeEach(()=>{
 vi.mocked(getInteractiveStories).mockReset()
 useInteractiveStore.setState({currentStoryId:'playing',currentBranchId:'main',stories:[{id:'playing',title:'Before'} as StorySummary]})
})
it('keeps a lore handoff on its source story and refreshes mutations without switching play',async()=>{
 vi.mocked(getInteractiveStories).mockResolvedValue({current_story_id:'source',stories:[{id:'playing',title:'Updated'} as StorySummary]})
 render(<GameAssistantView projectId="project" visible assistantHandoff={{key:'handoff',storyId:'source',resourceId:'lore-entry',context:{},instruction:'Review this entry'}} onAssistantHandoffConsumed={vi.fn()} />)
 await waitFor(()=>expect(screen.getByTestId('assistant-scope')).toHaveAttribute('data-story','source'))
 expect(screen.getByTestId('assistant-scope')).not.toHaveAttribute('data-branch')
 fireEvent.click(screen.getByTestId('assistant-scope'))
 await waitFor(()=>expect(useInteractiveStore.getState().stories[0].title).toBe('Updated'))
 expect(useInteractiveStore.getState().currentStoryId).toBe('playing')
})
it('ignores an old project mutation response after switching projects',async()=>{
 let finish!: (value: Awaited<ReturnType<typeof getInteractiveStories>>) => void
 vi.mocked(getInteractiveStories).mockReturnValue(new Promise(resolve=>{finish=resolve}))
 const props={visible:true,onAssistantHandoffConsumed:vi.fn()}
 const view=render(<GameAssistantView {...props} projectId="first" />)
 fireEvent.click(screen.getByTestId('assistant-scope'))
 view.rerender(<GameAssistantView {...props} projectId="second" />)
 await act(async()=>finish({current_story_id:'other',stories:[{id:'other',title:'Wrong project'} as StorySummary]}))
 expect(useInteractiveStore.getState().stories[0].id).toBe('playing')
})
