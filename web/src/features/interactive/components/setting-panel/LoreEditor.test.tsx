import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { setConfiguredLocale } from '@/i18n'
import type { LoreItem } from '@/lib/api'
import { LoreEditor } from './LoreEditor'

// The body editor is a full Markdown surface with its own suite; this file is
// about the metadata strip above it, so the editor is replaced by a marker.
vi.mock('@/features/lore/LoreContentEditor', () => ({
  LoreContentEditor: () => <div data-testid="lore-content-editor" />,
}))

function draft(overrides: Partial<LoreItem> = {}): LoreItem {
  return {
    id: 'liuqing',
    project_id: 'project-1',
    type: 'character',
    name: '柳青',
    enabled: true,
    importance: 'important',
    load_mode: 'auto',
    tags: [],
    brief_description: '药铺学徒',
    content: '',
    ...overrides,
  } as LoreItem
}

function renderEditor(item: LoreItem = draft(), overrides: { onClearImage?: () => void } = {}) {
  return render(
    <TooltipProvider>
      <LoreEditor
        projectId="project-1"
        draft={item}
        tagDraft=""
        residentTotalBytes={0}
        imagePresets={[]}
        imagePresetId=""
        imageInstruction=""
        imageGenerationMode="agent"
        imageBusyAction=""
        setDraft={vi.fn()}
        setTagDraft={vi.fn()}
        onImagePresetChange={vi.fn()}
        setImageInstruction={vi.fn()}
        onImageGenerationModeChange={vi.fn()}
        onGenerateImage={vi.fn()}
        onUploadImage={vi.fn()}
        onClearImage={overrides.onClearImage ?? vi.fn()}
        onSave={vi.fn()}
      />
    </TooltipProvider>,
  )
}

describe('LoreEditor reading mode', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
  })

  it('explains the current reading mode in plain words without jargon', () => {
    renderEditor()

    // The sentence is visible before any control is opened, so an author never
    // has to interpret the mode name to know what it does.
    expect(screen.getByText(/随人物、地点和眼前的事情，翻阅相关资料/)).toBeInTheDocument()
    expect(screen.queryByText(/system prompt/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Agent/)).not.toBeInTheDocument()
    expect(screen.queryByText(/索引/)).not.toBeInTheDocument()
  })

  it('keeps the mode control folded until someone asks for it', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(screen.queryByRole('combobox', { name: '取用方式' })).not.toBeInTheDocument()

    const trigger = screen.getByRole('button', { name: /取用方式：自动取用/ })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')

    await user.click(trigger)

    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('combobox', { name: '取用方式' })).toBeInTheDocument()
    // The hint ties appearances to the current story, not the reading mode.
    expect(screen.getByText(/人物会随所在之地、各自牵挂与眼前的事情走入故事/)).toBeInTheDocument()
  })

  it('says a resident entry is read every turn without promising appearances', () => {
    renderEditor(draft({ load_mode: 'resident' }))

    expect(screen.getByText(/每回合参考这段背景/)).toBeInTheDocument()
    expect(screen.queryByText(/每回合.*(?:出现|说话)/)).not.toBeInTheDocument()
  })

  it('describes a manual entry as read only when the story names it', () => {
    renderEditor(draft({ load_mode: 'manual' }))

    expect(screen.getByText(/在故事提及这条资料时阅读正文/)).toBeInTheDocument()
  })
})

// The shipped world book carries no image field: 101 people and not one picture.
// The author's library still has to show their faces, which the server reports
// on every read from the world's own catalog.
describe('LoreEditor portraits', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
  })

  it("shows the person's shipped face when the entry has no picture of its own", () => {
    renderEditor(draft({ portrait_url: '/api/portraits/canon_chen_qiaoqian.png', portrait_display_url: '/api/portraits/display/canon_chen_qiaoqian.png?size=display' }))

    const face = screen.getByRole('img', { name: '柳青' })
    expect(face).toHaveAttribute('src', '/api/portraits/display/canon_chen_qiaoqian.png?size=display')
    // It is a shipped default, not a chosen picture, and the page says so rather
    // than presenting it as the author's own work.
    expect(screen.getByText('世界自带的默认头像')).toBeInTheDocument()
    expect(screen.queryByText('暂无图片')).not.toBeInTheDocument()
  })

  it('keeps the placeholder for a person the world never authored', () => {
    renderEditor(draft({ name: '我编的人' }))

    expect(screen.queryByRole('img', { name: '我编的人' })).not.toBeInTheDocument()
    expect(screen.getByText('暂无图片')).toBeInTheDocument()
  })

  it("lets a chosen picture outrank the shipped face and stays clearable", async () => {
    const user = userEvent.setup()
    const onClearImage = vi.fn()
    renderEditor(
      draft({
        image: { schema: 'lore_item_image.v1', image_path: 'assets/lore/liuqing.png', meta_path: 'assets/lore/liuqing.json' },
        portrait_url: '/api/portraits/canon_chen_qiaoqian.png',
      }),
      { onClearImage },
    )

    // The chosen picture is what the page draws; the shipped default does not
    // replace it, and no caption claims it is a default.
    const face = screen.getByRole('img', { name: '柳青' })
    expect(face.getAttribute('src')).toContain('path=assets%2Flore%2Fliuqing.png')
    expect(screen.queryByText('世界自带的默认头像')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '打开图片重新生成' }))
    const clear = screen.getByRole('button', { name: '清除图片' })
    expect(clear).toBeEnabled()
    await user.click(clear)
    expect(onClearImage).toHaveBeenCalled()
  })

  it('does not offer to clear a face the author never chose', async () => {
    const user = userEvent.setup()
    renderEditor(draft({ portrait_url: '/api/portraits/canon_chen_qiaoqian.png' }))

    await user.click(screen.getByRole('button', { name: '打开图片生成' }))

    // Clearing a shipped default is not a change this page can make, so the
    // control does not promise it.
    expect(screen.getByRole('button', { name: '清除图片' })).toBeDisabled()
  })
})
