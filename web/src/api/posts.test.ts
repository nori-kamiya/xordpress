import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getPost, ifMatchHeader } from './posts'
import type { Post } from '../types'

// Locks the actual backend contract (internal/adminapi/posts.go postResponse):
// id/author_id are int64, created_at/updated_at/published_at are Unix seconds
// (time.Now().Unix(), no custom MarshalJSON) — all JSON numbers, not strings.
const RAW_POST_RESPONSE = {
  id: 1,
  slug: 'hello-xordpress',
  title: 'Hello XordPress',
  body: 'body text',
  status: 'published',
  version: 3,
  author_id: 1,
  created_at: 1785993522,
  updated_at: 1785993600,
  published_at: 1785993522,
}

function mockJsonResponse(body: unknown): Response {
  const headers = new Headers({ 'content-type': 'application/json' })
  return {
    ok: true,
    status: 200,
    statusText: '',
    headers,
    text: () => Promise.resolve(JSON.stringify(body)),
    json: () => Promise.resolve(body),
  } as unknown as Response
}

describe('posts contract', () => {
  let fetchSpy: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchSpy = vi.fn().mockResolvedValue(mockJsonResponse(RAW_POST_RESPONSE))
    vi.stubGlobal('fetch', fetchSpy)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('decodes a real postResponse JSON shape with numeric id/timestamps', async () => {
    const post: Post = await getPost('1')
    expect(post.id).toBe(1)
    expect(typeof post.id).toBe('number')
    expect(post.author_id).toBe(1)
    expect(typeof post.author_id).toBe('number')
    expect(post.created_at).toBe(1785993522)
    expect(post.updated_at).toBe(1785993600)
    expect(post.published_at).toBe(1785993522)
  })
})

describe('ifMatchHeader', () => {
  it('wraps the version in double quotes', () => {
    expect(ifMatchHeader(3)).toBe('"3"')
  })
})
