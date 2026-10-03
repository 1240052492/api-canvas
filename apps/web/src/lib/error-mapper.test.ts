import { describe, it, expect } from 'vitest'
import { mapError } from './error-mapper'

describe('mapError', () => {
  it('should map INSUFFICIENT_QUOTA to user-friendly message', () => {
    const result = mapError({ code: 'INSUFFICIENT_QUOTA', message: 'Not enough credits' })
    expect(result.message).toBe('积分余额不足，请前往充值')
    expect(result.action).toBe('redirect:/wallet/recharge')
  })

  it('should map MODEL_NOT_AVAILABLE', () => {
    const result = mapError({ code: 'MODEL_NOT_AVAILABLE', message: '' })
    expect(result.message).toBe('该模型暂时不可用，请选择其他模型')
    expect(result.action).toBe('refresh_model_list')
  })

  it('should handle unknown error codes', () => {
    const result = mapError({ code: 'UNKNOWN_ERROR', message: 'Something broke' })
    expect(result.message).toBe('系统异常，请稍后重试')
  })
})
