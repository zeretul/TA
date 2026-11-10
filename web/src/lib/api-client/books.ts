import { requestJSON } from './client'
import type { BookshelfResult } from './types'

export async function getBookshelf(): Promise<BookshelfResult> {
  const data = await requestJSON<Partial<BookshelfResult>>('/api/books')
  return {
    books: data.books || [],
    sort_mode: data.sort_mode === 'manual' ? 'manual' : 'recent',
  }
}

export function bookCoverURL(path: string, version?: string): string {
  const params = new URLSearchParams({ path })
  if (version) params.set('v', version)
  return `/api/books/cover?${params.toString()}`
}
