import { Hono } from 'hono'
import { setCookie } from 'hono/cookie'
import { nanoid } from 'nanoid'
import { getPrismaClient } from '../../platform/node/prisma'
import { newApiClient } from '../../utils/new-api-client'
import type { AppEnv } from '../../types'

export const oauthRoutes = new Hono<AppEnv>()

oauthRoutes.get('/callback', async (c) => {
  const code = c.req.query('code')
  const state = c.req.query('state')

  if (!code) {
    return c.json({ error: 'missing_code' }, 400)
  }

  // TODO: Verify state against stored value (CSRF protection)

  try {
    // Exchange code for user info
    const redirectUri = c.env?.CANVAS_REDIRECT_URI || process.env.CANVAS_REDIRECT_URI || 'http://localhost:5173/oauth/callback'
    const userInfo = await newApiClient.exchangeCode(code, redirectUri)

    // Create session
    const sessionToken = nanoid(64)
    const expiresAt = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000) // 30 days

    const prisma = getPrismaClient()
    await prisma.sessions.create({
      data: {
        id: nanoid(),
        user_id: userInfo.user_id,
        username: userInfo.username,
        email: userInfo.email || null,
        avatar_url: userInfo.avatar_url || null,
        token: sessionToken,
        expires_at: expiresAt.toISOString(),
        created_at: new Date().toISOString(),
        last_accessed_at: new Date().toISOString(),
      },
    })

    // Set session cookie
    setCookie(c, 'canvas_session', sessionToken, {
      maxAge: 2592000, // 30 days
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'Lax',
      path: '/',
    })

    // Redirect to home
    return c.redirect('/')
  } catch (error) {
    console.error('OAuth callback error:', error)
    const message = error instanceof Error ? error.message : 'Unknown error'
    return c.json({ error: 'exchange_failed', message }, 400)
  }
})
