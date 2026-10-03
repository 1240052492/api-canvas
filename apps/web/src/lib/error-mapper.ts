export interface ApiError {
  code: string
  message: string
}

export interface UserFacingError {
  message: string
  action?: string
}

const ERROR_MESSAGES: Record<string, { zh: string; en: string; action?: string }> = {
  'INSUFFICIENT_QUOTA': {
    zh: '积分余额不足，请前往充值',
    en: 'Insufficient credits, please top up',
    action: 'redirect:/wallet/recharge'
  },
  'MODEL_NOT_AVAILABLE': {
    zh: '该模型暂时不可用，请选择其他模型',
    en: 'Model temporarily unavailable',
    action: 'refresh_model_list'
  },
  'UPSTREAM_TIMEOUT': {
    zh: '生成超时，已退回积分，请重试',
    en: 'Generation timeout, credits refunded',
    action: 'retry'
  },
  'UPSTREAM_ERROR': {
    zh: '服务异常，已退回积分，请稍后重试',
    en: 'Service error, credits refunded',
  },
  'RATE_LIMIT_EXCEEDED': {
    zh: '请求过于频繁，请稍后再试',
    en: 'Rate limit exceeded',
    action: 'cooldown:60s'
  },
  'SESSION_EXPIRED': {
    zh: '会话已过期，正在跳转登录...',
    en: 'Session expired',
    action: 'redirect:/sso/authorize'
  }
}

export function mapError(apiError: ApiError, lang: string = 'zh'): UserFacingError {
  const template = ERROR_MESSAGES[apiError.code]

  if (!template) {
    return {
      message: lang === 'zh' ? '系统异常，请稍后重试' : 'System error, please try again'
    }
  }

  return {
    message: template[lang as 'zh' | 'en'],
    action: template.action
  }
}
