import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { BookSwitcher } from './BookSwitcher'

vi.mock('@/components/Home/BookCoverThumbnail', () => ({ BookCoverThumbnail: () => null }))
const books = ['one', 'two'].map(name => ({ project_id: name, name, path: `/${name}`, author: '', last_opened_at: '' }))

function MobileSwitcher({ switchBook }: { switchBook: () => Promise<boolean> }) {
  const [open, setOpen] = useState(true)
  return <>
    <button onClick={() => setOpen(true)}>Open navigation</button>
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent aria-describedby={undefined}>
        <SheetTitle>Navigation test</SheetTitle>
        <BookSwitcher books={books} currentBookName="one" workspace="/one" modal={false} onManageBooks={() => setOpen(false)}
          onSwitchBook={async () => { const ok = await switchBook(); if (ok) setOpen(false); return ok }} />
      </SheetContent>
    </Sheet>
  </>
}

describe('BookSwitcher within mobile navigation', () => {
  it('releases pointer blocking when a successful asynchronous switch closes both overlays', async () => {
    let resolve!: (ok: boolean) => void
    const switchBook = vi.fn(() => new Promise<boolean>(r => { resolve = r }))
    render(<MobileSwitcher switchBook={switchBook} />)
    fireEvent.keyDown(screen.getByRole('button', { name: '切换世界，当前：one' }), { key: 'Enter' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /^two/ }))
    expect(switchBook).toHaveBeenCalledOnce()
    expect(screen.getByRole('menuitem', { name: /^two/ })).toHaveAttribute('aria-disabled', 'true')
    expect(document.body.style.pointerEvents).toBe('none')
    await act(async () => resolve(true))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(document.body.style.pointerEvents).not.toBe('none')
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
  })
  it('keeps the navigation and menu available after a rejected switch, then releases the sheet lock on dismissal', async () => {
    const switchBook = vi.fn(async () => false)
    render(<MobileSwitcher switchBook={switchBook} />)
    fireEvent.keyDown(screen.getByRole('button', { name: '切换世界，当前：one' }), { key: 'Enter' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /^two/ }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /^two/ })).not.toHaveAttribute('aria-disabled'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(document.body.style.pointerEvents).toBe('none')
    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(document.body.style.pointerEvents).toBe('none')
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(document.body.style.pointerEvents).not.toBe('none')
  })

  it('retains standalone desktop modal behavior and releases its own lock on close', async () => {
    render(<BookSwitcher books={books} currentBookName="one" workspace="/one" onSwitchBook={vi.fn(async () => false)} onManageBooks={vi.fn()} />)
    fireEvent.keyDown(screen.getByRole('button', { name: '切换世界，当前：one' }), { key: 'Enter' })
    const menu = await screen.findByRole('menu')
    expect(document.body.style.pointerEvents).toBe('none')
    fireEvent.keyDown(menu, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(document.body.style.pointerEvents).not.toBe('none')
  })
})
