import { Context, Next } from 'hono'
import { getCookie } from 'hono/cookie'
import { getPrismaClient } from '../platform/node/prisma'

export function sessionMiddleware() {
  return async (c: Context, next: Next) => {
    const sessionToken = getCookie(c, 'canvas_session')

    if (!sessionToken) {
      return c.json({ error: 'SESSION_EXPIRED', message: 'No session found' }, 401)
    }

    const prisma = getPrismaClient()
    const session = await prisma.sessions.findUnique({
      where: { token: sessionToken }
    })

    if (!session) {
      return c.json({ error: 'SESSION_EXPIRED', message: 'Invalid session' }, 401)
    }

    const expiresAt = new Date(session.expires_at)
    if (expiresAt < new Date()) {
      return c.json({ error: 'SESSION_EXPIRED', message: 'Session expired' }, 401)
    }

    // Update last accessed time
    await prisma.sessions.update({
      where: { id: session.id },
      data: { last_accessed_at: new Date().toISOString() }
    })

    // Attach user to context
    c.set('user', {
      user_id: session.user_id,
      username: session.username,
      email: session.email,
      avatar_url: session.avatar_url
    })

    await next()
  }
}
