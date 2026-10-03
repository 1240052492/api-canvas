export interface UserInfo {
  user_id: string
  username: string
  email?: string
  avatar_url?: string
  expires_in: number
}

export interface WalletBalance {
  user_id: string
  balance: number
  used_quota: number
  display_balance: string
}

export interface PreDeductRequest {
  user_id: string
  amount: number
  idempotency_key: string
  metadata?: Record<string, any>
}

export interface PreDeductResponse {
  deduction_id: string
  amount: number
  balance_after: number
}

export interface SettleRequest {
  deduction_id: string
  actual_cost: number
  idempotency_key: string
}

export interface RefundRequest {
  deduction_id: string
  idempotency_key: string
  reason?: string
}

export class NewAPIClient {
  private baseUrl: string
  private serviceToken: string

  constructor(baseUrl: string, serviceToken: string) {
    this.baseUrl = baseUrl
    this.serviceToken = serviceToken
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${path}`
    const headers = {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${this.serviceToken}`,
      ...options.headers
    }

    const response = await fetch(url, { ...options, headers })

    if (!response.ok) {
      try {
        const error = await response.json()
        throw new Error(error.error || error.message || 'API request failed')
      } catch (e) {
        // If error response is not JSON, use status text
        throw new Error(`API request failed: ${response.status} ${response.statusText}`)
      }
    }

    return response.json()
  }

  async exchangeCode(code: string, redirectUri: string): Promise<UserInfo> {
    return this.request<UserInfo>('/sso/token', {
      method: 'POST',
      body: JSON.stringify({ code, redirect_uri: redirectUri })
    })
  }

  async getWallet(userId: string): Promise<WalletBalance> {
    return this.request<WalletBalance>(`/internal/v1/users/${userId}/wallet`)
  }

  async preDeduct(req: PreDeductRequest): Promise<PreDeductResponse> {
    return this.request<PreDeductResponse>('/internal/v1/wallet/pre-deduct', {
      method: 'POST',
      body: JSON.stringify(req)
    })
  }

  async settle(req: SettleRequest): Promise<{ settled: boolean }> {
    return this.request('/internal/v1/wallet/settle', {
      method: 'POST',
      body: JSON.stringify(req)
    })
  }

  async refund(req: RefundRequest): Promise<{ refunded: boolean }> {
    return this.request('/internal/v1/wallet/refund', {
      method: 'POST',
      body: JSON.stringify(req)
    })
  }

  async getModels(types?: string[]): Promise<any> {
    const query = types ? `?${types.map(t => `type=${t}`).join('&')}` : ''
    return this.request(`/api/models${query}`)
  }
}

// Singleton instance
export const newApiClient = new NewAPIClient(
  process.env.NEW_API_BASE_URL || 'http://localhost:8080',
  process.env.NEW_API_SERVICE_TOKEN || ''
)
