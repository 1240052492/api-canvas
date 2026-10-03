import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Hono } from 'hono'
import { oauthRoutes } from './oauth.routes'

const mockPrisma = {
  sessions: {
    create: vi.fn(),
  },
}

const { newApiClient, getPrismaClient, nanoid } = vi.hoisted(() => ({
  newApiClient: {
    exchangeCode: vi.fn(),
  },
  getPrismaClient: vi.fn(() => mockPrisma),
  nanoid: vi.fn(() => 'mock-session-token-12345'),
}))

vi.mock('../../utils/new-api-client', () => ({
  newApiClient,
}))

vi.mock('../../platform/node/prisma', () => ({
  getPrismaClient,
}))

vi.mock('nanoid', () => ({
  nanoid,
}))

describe('OAuth callback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    nanoid.mockReturnValue('mock-session-token-12345')
  })

  it('should exchange code and create session', async () => {
    const app = new Hono()
    app.route('/oauth', oauthRoutes)

    newApiClient.exchangeCode.mockResolvedValue({
      user_id: '123',
      username: 'alice',
      email: 'alice@example.com',
      avatar_url: 'https://example.com/avatar.png',
      expires_in: 2592000,
    })

    mockPrisma.sessions.create.mockResolvedValue({
      id: 'session-1',
      user_id: '123',
      username: 'alice',
      email: 'alice@example.com',
      avatar_url: 'https://example.com/avatar.png',
      token: 'mock-session-token-12345',
      expires_at: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
      created_at: new Date().toISOString(),
      last_accessed_at: new Date().toISOString(),
    })

    const res = await app.request('/oauth/callback?code=test_code_xyz&state=abc')

    expect(res.status).toBe(302)
    expect(res.headers.get('Location')).toBe('/')

    const cookies = res.headers.get('Set-Cookie')
    expect(cookies).toContain('canvas_session=')
    expect(cookies).toContain('HttpOnly')
    expect(cookies).toContain('Max-Age=2592000')

    expect(newApiClient.exchangeCode).toHaveBeenCalledWith(
      'test_code_xyz',
      expect.any(String)
    )
    expect(mockPrisma.sessions.create).toHaveBeenCalledWith({
      data: expect.objectContaining({
        user_id: '123',
        username: 'alice',
        email: 'alice@example.com',
        token: 'mock-session-token-12345',
      }),
    })
  })

  it('should reject missing code', async () => {
    const app = new Hono()
    app.route('/oauth', oauthRoutes)

    const res = await app.request('/oauth/callback')
    expect(res.status).toBe(400)

    const body = await res.json()
    expect(body).toMatchObject({ error: 'missing_code' })
  })

  it('should reject invalid code', async () => {
    const app = new Hono()
    app.route('/oauth', oauthRoutes)

    newApiClient.exchangeCode.mockRejectedValue(new Error('invalid_grant'))

    const res = await app.request('/oauth/callback?code=invalid_code')
    expect(res.status).toBe(400)

    const body = await res.json()
    expect(body.error).toBe('exchange_failed')
  })
})
