import { Context, Next } from 'hono'

let lastHealthCheck = Date.now()
let healthStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy'
let consecutiveFailures = 0

export async function checkNewAPIHealth() {
  try {
    const baseUrl = process.env.NEW_API_BASE_URL || 'http://localhost:8080'
    const controller = new AbortController()
    const timeoutId = setTimeout(() => controller.abort(), 3000)

    const response = await fetch(`${baseUrl}/health`, {
      signal: controller.signal
    })

    clearTimeout(timeoutId)

    if (response.ok) {
      const data = await response.json()
      healthStatus = data.status || 'healthy'
      consecutiveFailures = 0
    } else {
      consecutiveFailures++
      if (consecutiveFailures >= 3) {
        healthStatus = 'unhealthy'
      }
    }
  } catch (error) {
    consecutiveFailures++
    if (consecutiveFailures >= 3) {
      healthStatus = 'unhealthy'
    }
  }

  lastHealthCheck = Date.now()
}

export function healthCheckMiddleware() {
  return async (c: Context, next: Next) => {
    // Refresh health status every 30 seconds
    if (Date.now() - lastHealthCheck > 30000) {
      checkNewAPIHealth() // Non-blocking
    }

    c.set('new_api_health', healthStatus)
    await next()
  }
}

export function getHealthStatus(): 'healthy' | 'degraded' | 'unhealthy' {
  return healthStatus
}

// Start periodic health checks
setInterval(checkNewAPIHealth, 30000)
