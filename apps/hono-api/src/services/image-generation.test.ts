import { describe, it, expect, beforeEach, vi } from 'vitest'
import { generateImage } from './image-generation'
import { newApiClient } from '../utils/new-api-client'

vi.mock('../utils/new-api-client', () => ({
  newApiClient: {
    preDeduct: vi.fn(),
    settle: vi.fn(),
    refund: vi.fn(),
    getWallet: vi.fn()
  }
}))

global.fetch = vi.fn()

describe('generateImage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should pre-deduct, generate, and settle', async () => {
    // Mock pre-deduct
    vi.mocked(newApiClient.preDeduct).mockResolvedValue({
      deduction_id: 'deduct_123',
      amount: 10000,
      balance_after: 90000
    })

    // Mock AI generation
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [{ url: 'https://example.com/image.png' }],
        usage: { total_cost: 8500 }
      })
    } as Response)

    // Mock settle
    vi.mocked(newApiClient.settle).mockResolvedValue({ settled: true })

    const result = await generateImage({
      user_id: '123',
      task_id: 'task_abc',
      model: 'flux-pro',
      prompt: 'a cat',
      estimated_cost: 10000
    })

    expect(result).toEqual({
      image_url: 'https://example.com/image.png',
      actual_cost: 8500
    })

    // Verify pre-deduct was called
    expect(newApiClient.preDeduct).toHaveBeenCalledWith({
      user_id: '123',
      amount: 10000,
      idempotency_key: '123_task_abc_prededuct',
      metadata: { task_id: 'task_abc', model: 'flux-pro', prompt: 'a cat' }
    })

    // Verify settle was called with actual cost
    expect(newApiClient.settle).toHaveBeenCalledWith({
      deduction_id: 'deduct_123',
      actual_cost: 8500,
      idempotency_key: '123_task_abc_settle'
    })

    // Verify refund was NOT called
    expect(newApiClient.refund).not.toHaveBeenCalled()
  })

  it('should refund on generation failure', async () => {
    // Mock pre-deduct
    vi.mocked(newApiClient.preDeduct).mockResolvedValue({
      deduction_id: 'deduct_456',
      amount: 10000,
      balance_after: 90000
    })

    // Mock AI generation failure
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      statusText: 'Bad Request'
    } as Response)

    // Mock refund
    vi.mocked(newApiClient.refund).mockResolvedValue({ refunded: true })

    await expect(generateImage({
      user_id: '123',
      task_id: 'task_fail',
      model: 'flux-pro',
      prompt: 'invalid',
      estimated_cost: 10000
    })).rejects.toThrow('Generation failed: Bad Request')

    // Verify refund was called
    expect(newApiClient.refund).toHaveBeenCalledWith({
      deduction_id: 'deduct_456',
      idempotency_key: '123_task_fail_refund',
      reason: 'generation_failed'
    })

    // Verify settle was NOT called
    expect(newApiClient.settle).not.toHaveBeenCalled()
  })

  it('should throw error on insufficient quota', async () => {
    // Mock pre-deduct failure
    vi.mocked(newApiClient.preDeduct).mockRejectedValue(
      new Error('INSUFFICIENT_QUOTA: Not enough balance')
    )

    await expect(generateImage({
      user_id: '123',
      task_id: 'task_noquota',
      model: 'flux-pro',
      prompt: 'test',
      estimated_cost: 10000
    })).rejects.toThrow('余额不足，请前往充值')

    // Verify no settle or refund was called
    expect(newApiClient.settle).not.toHaveBeenCalled()
    expect(newApiClient.refund).not.toHaveBeenCalled()
  })

  it('should use estimated cost if actual cost not provided', async () => {
    // Mock pre-deduct
    vi.mocked(newApiClient.preDeduct).mockResolvedValue({
      deduction_id: 'deduct_789',
      amount: 10000,
      balance_after: 90000
    })

    // Mock AI generation without usage info
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [{ url: 'https://example.com/image.png' }]
        // No usage field
      })
    } as Response)

    // Mock settle
    vi.mocked(newApiClient.settle).mockResolvedValue({ settled: true })

    const result = await generateImage({
      user_id: '123',
      task_id: 'task_noactual',
      model: 'flux-pro',
      prompt: 'a dog',
      estimated_cost: 10000
    })

    expect(result.actual_cost).toBe(10000)

    // Verify settle was called with estimated cost
    expect(newApiClient.settle).toHaveBeenCalledWith({
      deduction_id: 'deduct_789',
      actual_cost: 10000,
      idempotency_key: '123_task_noactual_settle'
    })
  })
})
