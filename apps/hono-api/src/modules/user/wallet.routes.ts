import { Hono } from 'hono'
import type { AppEnv } from '../../types'
import { sessionMiddleware } from '../../middleware/session'
import { newApiClient } from '../../utils/new-api-client'

export const walletRouter = new Hono<AppEnv>()

walletRouter.use('/*', sessionMiddleware())

walletRouter.get('/', async (c) => {
  try {
    const user = c.get('user')

    if (!user?.user_id) {
      return c.json({ error: 'User not found in session' }, 401)
    }

    const wallet = await newApiClient.getWallet(user.user_id)

    return c.json(wallet)
  } catch (error) {
    console.error('Failed to fetch wallet balance:', error)
    return c.json({
      error: 'Failed to fetch wallet balance',
      message: error instanceof Error ? error.message : 'Unknown error'
    }, 500)
  }
})
