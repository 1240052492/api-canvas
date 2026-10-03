import { useEffect, useState } from 'react'
import { getWalletBalance } from '../api/server'
import { useAuth } from '../auth/store'

export function WalletBalance() {
  const auth = useAuth()
  const [balance, setBalance] = useState<string>('--')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!auth.user || auth.user.guest) {
      setLoading(false)
      return
    }

    const fetchBalance = async () => {
      try {
        const wallet = await getWalletBalance()
        setBalance(wallet.display_balance)
      } catch (error) {
        console.error('Failed to fetch wallet balance:', error)
        setBalance('Error')
      } finally {
        setLoading(false)
      }
    }

    fetchBalance()

    // Refresh every 30 seconds
    const interval = setInterval(fetchBalance, 30000)
    return () => clearInterval(interval)
  }, [auth.user])

  if (!auth.user || auth.user.guest) {
    return null
  }

  if (loading) {
    return <span className="wallet-balance-loading" style={{ fontSize: '0.875rem', color: '#94a3b8' }}>Loading...</span>
  }

  return (
    <div className="wallet-balance-container" style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
      <span className="wallet-balance-label" style={{ fontSize: '0.875rem', color: '#64748b' }}>余额:</span>
      <span className="wallet-balance-value" style={{ fontSize: '0.875rem', fontWeight: 500 }}>{balance}</span>
      <a
        href={`${import.meta.env.VITE_NEW_API_BASE_URL || 'http://localhost:8080'}/wallet`}
        className="wallet-balance-recharge"
        style={{ fontSize: '0.75rem', color: '#3b82f6', textDecoration: 'none' }}
        onMouseEnter={(e) => e.currentTarget.style.textDecoration = 'underline'}
        onMouseLeave={(e) => e.currentTarget.style.textDecoration = 'none'}
      >
        充值
      </a>
    </div>
  )
}
