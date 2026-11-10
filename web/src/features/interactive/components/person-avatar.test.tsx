import { cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { PersonAvatar, PersonPortrait } from './person-avatar'
afterEach(cleanup)
describe.each([PersonAvatar, PersonPortrait])('portrait fallback', Component => {
  it.each(['png', 'webp'])('accepts the server supplied %s URL without rewriting it', extension => {
    const url = `/api/catalog-art/${'ab'.repeat(32)}.${extension}`
    const view = render(<Component avatar={url} name="Native image" />)
    expect(view.container.querySelector('img')).toHaveAttribute('src', url)
  })

  it('keeps a neutral initial on failure and recovers when the image identity changes', () => {
    const first = `/api/catalog-art/${'ab'.repeat(32)}.png`
    const second = `/api/catalog-art/${'cd'.repeat(32)}.png`
    const view = render(<Component avatar={first} name="宋老橹" />)
    fireEvent.error(view.container.querySelector('img')!)
    expect(view.container.querySelector('img')).toBeNull()
    expect(view.container.textContent).toBe('宋')
    view.rerender(<Component avatar={second} name="宋老橹" />)
    expect(view.container.querySelector('img')).toHaveAttribute('src', second)
    view.rerender(<Component name="未知人物" />)
    expect(view.container.querySelector('img')).toBeNull()
    expect(view.container.textContent).toBe('未')
  })
})
