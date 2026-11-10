import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {apiRoute, installApiMock, jsonResponse} from '@/test/api-mock'
import {getProjectBookTree, getProjectBookSummary} from './project-book'

describe('game workspace reads the retained project snapshot', () => {
  let apiMock: ReturnType<typeof installApiMock>
  beforeEach(() => { apiMock = installApiMock() })
  afterEach(() => vi.unstubAllGlobals())

  it('loads tree and summary through the registered snapshot endpoint', async () => {
    const tree = [{name: 'interactive', type: 'dir' as const}]
    const summary = {chapters: [], chapter_plans: []}
    apiMock.use(apiRoute.get('/api/projects/project-one/book', () => jsonResponse({project_id: 'project-one', tree, summary})))
    expect(await getProjectBookTree('project-one')).toEqual(tree)
    expect(await getProjectBookSummary('project-one')).toEqual(summary)
  })

  it('still rejects another project snapshot', async () => {
    apiMock.use(apiRoute.get('/api/projects/project-one/book', () => jsonResponse({project_id: 'project-other', tree: [], summary: {chapters: [], chapter_plans: []}})))
    await expect(getProjectBookTree('project-one')).rejects.toThrow('scope mismatch')
    await expect(getProjectBookSummary('project-one')).rejects.toThrow('scope mismatch')
  })
})
