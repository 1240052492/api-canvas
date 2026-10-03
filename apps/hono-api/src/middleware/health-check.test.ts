import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { checkNewAPIHealth, healthCheckMiddleware, getHealthStatus } from './health-check'
import type { Context } from 'hono'

describe('health-check middleware', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    global.fetch = vi.fn()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('should set healthy status when New API responds ok', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'healthy' })
    })
    global.fetch = mockFetch

    await checkNewAPIHealth()

    expect(getHealthStatus()).toBe('healthy')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/health'),
      expect.any(Object)
    )
  })

  it('should set unhealthy after 3 consecutive failures', async () => {
    const mockFetch = vi.fn().mockRejectedValue(new Error('Connection failed'))
    global.fetch = mockFetch

    await checkNewAPIHealth()
    expect(getHealthStatus()).toBe('healthy') // First failure

    await checkNewAPIHealth()
    expect(getHealthStatus()).toBe('healthy') // Second failure

    await checkNewAPIHealth()
    expect(getHealthStatus()).toBe('unhealthy') // Third failure triggers unhealthy
  })

  it('should reset consecutive failures on successful health check', async () => {
    const mockFetch = vi.fn()
      .mockRejectedValueOnce(new Error('Fail'))
      .mockRejectedValueOnce(new Error('Fail'))
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ status: 'healthy' })
      })

    global.fetch = mockFetch

    await checkNewAPIHealth()
    await checkNewAPIHealth()
    await checkNewAPIHealth()

    expect(getHealthStatus()).toBe('healthy')
  })

  it('should attach health status to context', async () => {
    const mockContext = {
      set: vi.fn()
    } as unknown as Context

    const next = vi.fn()
    const middleware = healthCheckMiddleware()

    await middleware(mockContext, next)

    expect(mockContext.set).toHaveBeenCalledWith('new_api_health', expect.any(String))
    expect(next).toHaveBeenCalled()
  })
})
