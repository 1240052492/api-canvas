import { describe, it, expect, beforeEach, vi } from 'vitest'
import { Hono } from 'hono'

const prisma = {
  sessions: {
    findUnique: vi.fn(),
    update: vi.fn(),
  },
}

vi.mock('../platform/node/prisma', () => ({
  getPrismaClient: () => prisma,
}))

vi.mock('hono/cookie', async (importOriginal) => {
  const actual = await importOriginal() as object
  return {
    ...actual,
    getCookie: vi.fn(),
  }
})

import { sessionMiddleware } from './session'
import { getCookie } from 'hono/cookie'

describe('sessionMiddleware', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should reject request without session cookie', async () => {
    vi.mocked(getCookie).mockReturnValue(undefined)

    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => c.json({ ok: true }))

    const res = await app.request('/test')
    expect(res.status).toBe(401)
    const body = await res.json()
    expect(body).toMatchObject({
      error: 'SESSION_EXPIRED',
      message: 'No session found',
    })
  })

  it('should reject invalid session token', async () => {
    vi.mocked(getCookie).mockReturnValue('invalid_token_123')
    prisma.sessions.findUnique.mockResolvedValue(null)

    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => c.json({ ok: true }))

    const res = await app.request('/test', {
      headers: { Cookie: 'canvas_session=invalid_token_123' },
    })
    expect(res.status).toBe(401)
    const body = await res.json()
    expect(body).toMatchObject({
      error: 'SESSION_EXPIRED',
      message: 'Invalid session',
    })
  })

  it('should reject expired session', async () => {
    const expiredToken = 'expired_token_123'
    vi.mocked(getCookie).mockReturnValue(expiredToken)
    prisma.sessions.findUnique.mockResolvedValue({
      id: 'session-1',
      user_id: '123',
      username: 'alice',
      email: 'alice@example.com',
      avatar_url: 'https://avatar.com/alice.png',
      token: expiredToken,
      expires_at: new Date(Date.now() - 1000).toISOString(),
      created_at: new Date(Date.now() - 86400000).toISOString(),
      last_accessed_at: new Date(Date.now() - 3600000).toISOString(),
    })

    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => c.json({ ok: true }))

    const res = await app.request('/test', {
      headers: { Cookie: `canvas_session=${expiredToken}` },
    })
    expect(res.status).toBe(401)
    const body = await res.json()
    expect(body).toMatchObject({
      error: 'SESSION_EXPIRED',
      message: 'Session expired',
    })
  })

  it('should attach user to context for valid session', async () => {
    const validToken = 'valid_token_456'
    vi.mocked(getCookie).mockReturnValue(validToken)
    prisma.sessions.findUnique.mockResolvedValue({
      id: 'session-2',
      user_id: '123',
      username: 'alice',
      email: 'alice@example.com',
      avatar_url: 'https://avatar.com/alice.png',
      token: validToken,
      expires_at: new Date(Date.now() + 86400000).toISOString(),
      created_at: new Date(Date.now() - 86400000).toISOString(),
      last_accessed_at: new Date(Date.now() - 3600000).toISOString(),
    })
    prisma.sessions.update.mockResolvedValue({})

    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => {
      const user = c.get('user')
      return c.json({ user_id: user.user_id, username: user.username })
    })

    const res = await app.request('/test', {
      headers: { Cookie: `canvas_session=${validToken}` },
    })
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.user_id).toBe('123')
    expect(body.username).toBe('alice')
    expect(prisma.sessions.update).toHaveBeenCalledWith({
      where: { id: 'session-2' },
      data: { last_accessed_at: expect.any(String) },
    })
  })

  it('should update last_accessed_at on valid session', async () => {
    const validToken = 'valid_token_789'
    vi.mocked(getCookie).mockReturnValue(validToken)
    prisma.sessions.findUnique.mockResolvedValue({
      id: 'session-3',
      user_id: '456',
      username: 'bob',
      email: 'bob@example.com',
      avatar_url: null,
      token: validToken,
      expires_at: new Date(Date.now() + 86400000).toISOString(),
      created_at: new Date(Date.now() - 86400000).toISOString(),
      last_accessed_at: new Date(Date.now() - 7200000).toISOString(),
    })
    prisma.sessions.update.mockResolvedValue({})

    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => c.json({ ok: true }))

    await app.request('/test', {
      headers: { Cookie: `canvas_session=${validToken}` },
    })

    expect(prisma.sessions.update).toHaveBeenCalledWith({
      where: { id: 'session-3' },
      data: { last_accessed_at: expect.any(String) },
    })
  })
})
