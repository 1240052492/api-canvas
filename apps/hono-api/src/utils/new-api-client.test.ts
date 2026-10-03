import { describe, it, expect, beforeEach, vi } from 'vitest'
import { NewAPIClient } from './new-api-client'

describe('NewAPIClient', () => {
  let client: NewAPIClient
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    global.fetch = fetchMock
    client = new NewAPIClient('http://localhost:8080', 'sk-test-token')
  })

  it('should exchange auth code for user info', async () => {
    const mockResponse = {
      user_id: '123',
      username: 'testuser',
      email: 'test@example.com',
      expires_in: 3600
    }

    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => mockResponse
    })

    const result = await client.exchangeCode('test_code', 'http://localhost:5173/callback')
    expect(result).toHaveProperty('user_id')
    expect(result).toHaveProperty('username')
    expect(result.user_id).toBe('123')

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/sso/token',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Authorization': 'Bearer sk-test-token'
        })
      })
    )
  })

  it('should get wallet balance', async () => {
    const mockWallet = {
      user_id: '123',
      balance: 100000,
      used_quota: 50000,
      display_balance: '100.00'
    }

    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => mockWallet
    })

    const wallet = await client.getWallet('123')
    expect(wallet).toHaveProperty('balance')
    expect(wallet.user_id).toBe('123')

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/internal/v1/users/123/wallet',
      expect.objectContaining({
        headers: expect.objectContaining({
          'Authorization': 'Bearer sk-test-token'
        })
      })
    )
  })

  it('should pre-deduct credits', async () => {
    const mockResponse = {
      deduction_id: 'ded_123',
      amount: 10000,
      balance_after: 90000
    }

    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => mockResponse
    })

    const result = await client.preDeduct({
      user_id: '123',
      amount: 10000,
      idempotency_key: '123_task_test_prededuct'
    })
    expect(result).toHaveProperty('deduction_id')
    expect(result.deduction_id).toBe('ded_123')

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/internal/v1/wallet/pre-deduct',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Authorization': 'Bearer sk-test-token'
        })
      })
    )
  })

  it('should settle pre-deducted credits', async () => {
    const mockResponse = {
      settled: true
    }

    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => mockResponse
    })

    const result = await client.settle({
      deduction_id: 'ded_123',
      actual_cost: 8000,
      idempotency_key: 'settle_123'
    })
    expect(result.settled).toBe(true)

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/internal/v1/wallet/settle',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Authorization': 'Bearer sk-test-token'
        })
      })
    )
  })

  it('should refund pre-deducted credits', async () => {
    const mockResponse = {
      refunded: true
    }

    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => mockResponse
    })

    const result = await client.refund({
      deduction_id: 'ded_123',
      idempotency_key: 'refund_123',
      reason: 'Task cancelled'
    })
    expect(result.refunded).toBe(true)

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/internal/v1/wallet/refund',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Authorization': 'Bearer sk-test-token'
        })
      })
    )
  })

  it('should get models list', async () => {
    const mockResponse = {
      models: [
        { id: 'gpt-4', name: 'GPT-4', type: 'chat', enabled: true },
        { id: 'claude-3', name: 'Claude 3', type: 'chat', enabled: true }
      ]
    }

    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => mockResponse
    })

    const result = await client.getModels(['chat'])
    expect(result.models).toHaveLength(2)
    expect(result.models[0].id).toBe('gpt-4')

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/api/models?type=chat',
      expect.objectContaining({
        headers: expect.objectContaining({
          'Authorization': 'Bearer sk-test-token'
        })
      })
    )
  })

  it('should handle non-JSON error responses', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: () => Promise.reject(new Error('Not JSON'))
    })

    await expect(client.getWallet('123')).rejects.toThrow('API request failed: 500 Internal Server Error')
  })
})
