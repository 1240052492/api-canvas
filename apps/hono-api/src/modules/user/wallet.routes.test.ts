import { describe, it, expect, beforeEach, vi } from 'vitest'
import { Hono } from 'hono'
import { walletRouter } from './wallet.routes'
import type { AppEnv } from '../../types'

vi.mock('../../utils/new-api-client', () => ({
  newApiClient: {
    getWallet: vi.fn()
  }
}))

vi.mock('../../middleware/session', () => ({
  sessionMiddleware: () => async (c: any, next: any) => {
    c.set('user', {
      user_id: 'test-user-123',
      username: 'testuser',
      email: 'test@example.com',
      avatar_url: 'https://example.com/avatar.png'
    })
    await next()
  }
}))

describe('Wallet Routes', () => {
  let app: Hono<AppEnv>

  beforeEach(() => {
    vi.clearAllMocks()
    app = new Hono<AppEnv>()
    app.route('/wallet', walletRouter)
  })

  it('should return wallet balance for authenticated user', async () => {
    const { newApiClient } = await import('../../utils/new-api-client')
    vi.mocked(newApiClient.getWallet).mockResolvedValue({
      user_id: 'test-user-123',
      balance: 1000000,
      used_quota: 250000,
      display_balance: '$10.00'
    })

    const res = await app.request('/wallet')

    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body).toEqual({
      user_id: 'test-user-123',
      balance: 1000000,
      used_quota: 250000,
      display_balance: '$10.00'
    })
    expect(newApiClient.getWallet).toHaveBeenCalledWith('test-user-123')
  })

  it('should return 500 if wallet fetch fails', async () => {
    const { newApiClient } = await import('../../utils/new-api-client')
    vi.mocked(newApiClient.getWallet).mockRejectedValue(new Error('API unavailable'))

    const res = await app.request('/wallet')

    expect(res.status).toBe(500)
    const body = await res.json()
    expect(body).toHaveProperty('error')
    expect(body.error).toContain('Failed to fetch wallet')
  })
})
