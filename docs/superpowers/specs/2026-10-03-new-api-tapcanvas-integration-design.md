# New API + TapCanvas 集成设计文档

**文档版本**: v1.0  
**创建日期**: 2026-10-03  
**作者**: Claude (Opus 5)  
**状态**: 待审核

---

## 目录

1. [项目背景](#1-项目背景)
2. [需求概述](#2-需求概述)
3. [架构设计](#3-架构设计)
4. [核心业务流程](#4-核心业务流程)
5. [数据库设计](#5-数据库设计)
6. [API 接口规范](#6-api-接口规范)
7. [安全设计](#7-安全设计)
8. [错误处理](#8-错误处理)
9. [监控和运维](#9-监控和运维)
10. [环境配置](#10-环境配置)
11. [开发计划](#11-开发计划)
12. [验收标准](#12-验收标准)

---

## 1. 项目背景

### 1.1 项目目标

整合 **New API**（AI 模型网关）和 **TapCanvas**（AI 视频创作平台），实现：
- 统一的用户账号体系
- 共享的积分钱包系统
- 统一的模型管理和调用
- 独立部署但紧密协作的两个系统

### 1.2 集成策略

采用**轻量级集成方案（Plan A）**：
- New API 作为主入口（`https://www.aiplantform.site`）
- TapCanvas 作为子域名（`https://canvas.aiplantform.site`）
- 通过 SSO（单点登录）实现账号统一
- 通过内部 API 调用实现积分共享
- 各自独立部署和扩展

### 1.3 核心原则

- **最小侵入**：减少对现有代码的修改
- **安全优先**：Service Token 机制保护内部 API
- **可维护性**：清晰的职责边界和接口契约
- **可扩展性**：预留未来添加更多子系统的能力

---

## 2. 需求概述

### 2.1 功能需求

| 功能模块 | 需求描述 | 优先级 |
|---------|---------|--------|
| **SSO 单点登录** | TapCanvas 用户通过 New API 登录，返回 Canvas 会话 | P0 |
| **积分钱包共享** | TapCanvas 实时查询用户在 New API 的余额 | P0 |
| **模型目录同步** | TapCanvas 从 New API 获取可用模型列表 | P0 |
| **AI 任务调用** | TapCanvas 通过 Service Token 调用 New API 的 AI 接口 | P0 |
| **积分扣费结算** | 预扣 → 执行 → 结算的完整流程，支持幂等性 | P0 |
| **服务降级** | New API 不可用时，TapCanvas 优雅降级 | P1 |
| **监控告警** | 关键指标监控和异常告警 | P1 |

### 2.2 非功能需求

| 需求类型 | 指标 |
|---------|------|
| **性能** | SSO 登录 < 2s，余额查询 < 500ms，模型列表查询 < 1s |
| **可用性** | 服务可用性 99.5%，支持优雅降级 |
| **安全性** | Service Token 加密存储，内部 API 仅限白名单 IP |
| **可扩展性** | 支持水平扩展，无状态设计 |

---

## 3. 架构设计

### 3.1 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                         用户浏览器                            │
└────────────┬────────────────────────────────┬────────────────┘
             │                                │
             │ ① 登录/管理账号                │ ② 使用画布创作
             ▼                                ▼
┌────────────────────────┐      ┌────────────────────────────┐
│   New API (主站)        │      │   TapCanvas (子域名)        │
│  www.aiplantform.site  │◄─────│ canvas.aiplantform.site    │
│                        │ SSO  │                            │
│ ┌──────────────────┐  │      │ ┌──────────────────────┐  │
│ │ 用户管理         │  │      │ │ 画布编辑器            │  │
│ │ 钱包充值         │  │      │ │ 视频生成              │  │
│ │ 模型配置         │  │      │ │ 项目管理              │  │
│ │ API Key 管理     │  │      │ │ 会话管理 (本地)       │  │
│ └──────────────────┘  │      │ └──────────────────────┘  │
│                        │      │                            │
│ ┌──────────────────┐  │      │ ┌──────────────────────┐  │
│ │ AI 模型网关       │◄─┼──────┤ │ New API Client       │  │
│ │ - 模型路由        │  │      │ │ - Service Token Auth │  │
│ │ - 积分扣费        │  │      │ │ - 余额查询           │  │
│ │ - 日志记录        │  │      │ │ - AI 任务调用        │  │
│ └──────────────────┘  │      │ └──────────────────────┘  │
└───────────┬────────────┘      └──────────┬─────────────────┘
            │                              │
            │ ③ 内部 API 调用               │
            │ (Service Token)              │
            └──────────────────────────────┘
            
┌────────────────────────┐      ┌────────────────────────────┐
│  PostgreSQL (new_api)  │      │  PostgreSQL (tapcanvas)    │
│  - users               │      │  - projects, flows         │
│  - tokens              │      │  - sessions                │
│  - channels, models    │      │  - 95+ Canvas 业务表        │
│  - logs                │      │                            │
└────────────────────────┘      └────────────────────────────┘
```

### 3.2 系统职责划分

#### New API 职责
- ✅ 用户账号管理（注册、登录、OAuth）
- ✅ 钱包管理（充值、扣费、余额查询）
- ✅ 模型配置管理（Channel、Model、Vendor）
- ✅ AI 模型网关（请求路由、重试、日志）
- ✅ API Key 管理（用户 Key + Service Key）
- ✅ 消费日志和审计

#### TapCanvas 职责
- ✅ 画布编辑器和视频创作工具
- ✅ 项目管理（projects, flows, chapters）
- ✅ 会话管理（SSO 后的本地会话）
- ✅ AI 任务编排（调用 New API 生成图像/视频）
- ✅ 用户生成偏好（本地存储）
- ✅ 社区和内容管理

#### 共享职责
- 🔗 **用户身份**：New API 管理，TapCanvas 通过 SSO 获取
- 🔗 **模型列表**：New API 提供，TapCanvas 实时查询
- 🔗 **积分余额**：New API 存储，TapCanvas 实时查询

---

## 4. 核心业务流程

### 4.1 SSO 单点登录流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Canvas as TapCanvas
    participant NewAPI as New API
    participant DB as Database

    User->>Canvas: 访问 canvas.aiplantform.site
    Canvas->>Canvas: 检查本地会话
    alt 无会话或已过期
        Canvas->>NewAPI: 重定向到 /sso/authorize
        NewAPI->>User: 展示登录页面
        User->>NewAPI: 输入账号密码
        NewAPI->>DB: 验证用户凭证
        DB-->>NewAPI: 返回用户信息
        NewAPI->>NewAPI: 生成授权码 (auth_code)
        NewAPI->>Canvas: 重定向到 /oauth/callback?code=xxx
        Canvas->>NewAPI: POST /sso/token (exchange code)
        NewAPI->>DB: 验证 auth_code
        NewAPI-->>Canvas: 返回 user_id, username, email
        Canvas->>Canvas: 创建本地会话 (30天)
        Canvas-->>User: 登录成功，进入画布
    else 已有会话
        Canvas-->>User: 直接进入画布
    end
```

**关键步骤**：
1. TapCanvas 检测到未登录，重定向到 `https://www.aiplantform.site/sso/authorize?redirect_uri=https://canvas.aiplantform.site/oauth/callback`
2. 用户在 New API 登录（支持密码/OAuth/微信等）
3. New API 验证成功后，重定向回 TapCanvas 并携带授权码 `code`
4. TapCanvas 后端用 `code` 换取用户信息（`user_id`, `username`, `email`）
5. TapCanvas 创建本地会话（30天有效期）

### 4.2 AI 任务调用流程（完整链路）

```mermaid
sequenceDiagram
    participant User as 用户
    participant Canvas as TapCanvas
    participant NewAPI as New API
    participant Upstream as 上游 AI 服务

    User->>Canvas: 点击"生成图像"
    Canvas->>Canvas: 生成幂等键 task_abc_prededuct
    Canvas->>NewAPI: POST /internal/v1/wallet/pre-deduct<br/>{user_id, amount, idempotency_key}
    NewAPI->>NewAPI: 检查余额 & 幂等性
    alt 余额不足
        NewAPI-->>Canvas: 400 INSUFFICIENT_QUOTA
        Canvas-->>User: 提示"余额不足，请充值"
    else 余额充足
        NewAPI->>NewAPI: 冻结积分（预扣）
        NewAPI-->>Canvas: 200 OK {deduction_id}
        
        Canvas->>NewAPI: POST /v1/images/generations<br/>Authorization: Bearer sk-canvas-service-xxx<br/>X-Canvas-Task-Id: task_abc
        NewAPI->>NewAPI: 验证 Service Token
        NewAPI->>Upstream: 转发请求到上游
        
        alt 上游成功
            Upstream-->>NewAPI: 返回图像 URL
            NewAPI->>NewAPI: 记录消费日志
            NewAPI-->>Canvas: 200 OK {image_url, actual_cost}
            
            Canvas->>NewAPI: POST /internal/v1/wallet/settle<br/>{deduction_id, actual_cost, task_abc_settle}
            NewAPI->>NewAPI: 结算：扣除实际费用，退回差额
            NewAPI-->>Canvas: 200 OK
            Canvas-->>User: 显示生成的图像
            
        else 上游失败（已重试3次）
            Upstream-->>NewAPI: 500 ERROR
            NewAPI-->>Canvas: 500 UPSTREAM_ERROR
            
            Canvas->>NewAPI: POST /internal/v1/wallet/refund<br/>{deduction_id, task_abc_refund}
            NewAPI->>NewAPI: 全额退款
            NewAPI-->>Canvas: 200 OK
            Canvas-->>User: 提示"生成失败，积分已退回"
        end
    end
```

**关键要点**：
- **幂等性保证**：每个操作使用唯一幂等键 `{user_id}_{task_id}_{operation}`
- **三步扣费**：预扣 → 执行 → 结算/退款
- **自动重试**：New API 内部已实现跨渠道重试（最多3次）
- **错误处理**：失败后自动退款，用户无感知

### 4.3 模型列表同步流程

```mermaid
sequenceDiagram
    participant Canvas as TapCanvas
    participant Cache as 本地缓存
    participant NewAPI as New API

    Canvas->>Cache: 检查模型列表缓存
    alt 缓存有效（5分钟内）
        Cache-->>Canvas: 返回缓存的模型列表
    else 缓存过期或不存在
        Canvas->>NewAPI: GET /api/models?type=image&type=video
        NewAPI->>NewAPI: 查询可用模型
        NewAPI-->>Canvas: 返回模型列表 JSON
        Canvas->>Cache: 更新缓存（TTL=5分钟）
        Canvas-->>Canvas: 渲染模型选择器
    end
```

**缓存策略**：
- 前端缓存：5分钟（用户切换页面时不重复请求）
- 后端缓存：10分钟（减少数据库查询）
- 手动刷新：提供"刷新模型列表"按钮

---

## 5. 数据库设计

### 5.1 数据库架构

采用**共享 PostgreSQL 实例，独立数据库**的方案：

```
PostgreSQL 实例 (localhost:5432 或远程服务器)
├── new_api          # New API 数据库（25 张表）
│   ├── users        # 用户账号
│   ├── tokens       # API Keys (包括 Service Token)
│   ├── channels     # 模型渠道配置
│   ├── models       # 模型目录
│   ├── logs         # AI 调用日志
│   └── ...
│
└── tapcanvas        # TapCanvas 数据库（95+ 张表）
    ├── sessions     # 本地会话（SSO 后）
    ├── projects     # 项目
    ├── flows        # 画布流程
    ├── chapters     # 章节
    └── ...          # 其他 Canvas 业务表
```

### 5.2 New API 数据库改动

#### 5.2.1 tokens 表新增字段

```sql
ALTER TABLE tokens ADD COLUMN service_name VARCHAR(64) DEFAULT NULL;
ALTER TABLE tokens ADD INDEX idx_tokens_service_name (service_name);

-- 注释：
-- service_name = NULL: 普通用户创建的 API Key
-- service_name = 'tapcanvas': TapCanvas 的 Service Token
-- service_name = 'future_service': 预留给未来其他子系统
```

**字段说明**：
- `service_name`: 标识该 Token 是哪个服务自动创建的
- 前端展示逻辑：
  - `service_name IS NULL`: 显示"编辑"、"删除"按钮
  - `service_name IS NOT NULL`: 按钮置灰，显示"（服务专用）"

#### 5.2.2 新增 wallet_transactions 表（可选）

用于记录预扣/结算/退款的详细流水：

```sql
CREATE TABLE wallet_transactions (
    id VARCHAR(64) PRIMARY KEY,
    user_id INT NOT NULL,
    type VARCHAR(20) NOT NULL,  -- 'pre_deduct', 'settle', 'refund'
    amount BIGINT NOT NULL,     -- 积分数（单位：0.001 USD）
    idempotency_key VARCHAR(128) UNIQUE NOT NULL,
    deduction_id VARCHAR(64),   -- 关联的预扣记录 ID
    metadata TEXT,              -- JSON，存储 task_id 等信息
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_wallet_tx_user_created (user_id, created_at DESC),
    INDEX idx_wallet_tx_idempotency (idempotency_key)
);
```

### 5.3 TapCanvas 数据库改动

#### 无需新增表，仅调整业务逻辑

- **禁用表**：不删除，但不再读写
  - 原 Luban API 的 `users`, `tokens`, `channels`, `models`, `logs` 等
  - 通过环境变量 `DISABLE_EMBEDDED_LUBAN=true` 控制

- **保留表**：继续使用
  - 所有 Canvas 业务表（projects, flows, chapters, assets, etc.）
  - `sessions` 表（存储 SSO 登录后的会话）

#### sessions 表结构（如果不存在则创建）

```sql
CREATE TABLE sessions (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,      -- 从 New API 获取的 user_id
    username VARCHAR(255),
    email VARCHAR(255),
    avatar_url TEXT,
    token VARCHAR(255) UNIQUE NOT NULL, -- 会话 Token（Cookie 中存储）
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_sessions_user_id (user_id),
    INDEX idx_sessions_token (token),
    INDEX idx_sessions_expires_at (expires_at)
);
```

---

## 6. API 接口规范

### 6.1 New API 新增接口

#### 6.1.1 SSO 授权接口

**发起授权**

```http
GET /sso/authorize?redirect_uri={canvas_callback_url}&state={random_state}

响应：重定向到登录页面或直接返回授权码
Location: {redirect_uri}?code={auth_code}&state={random_state}
```

**兑换 Token**

```http
POST /sso/token
Content-Type: application/json

{
  "code": "auth_code_xxxxxxx",
  "redirect_uri": "https://canvas.aiplantform.site/oauth/callback"
}

响应：
{
  "user_id": "123",
  "username": "alice",
  "email": "alice@example.com",
  "avatar_url": "https://...",
  "expires_in": 2592000  // 30 天
}
```

#### 6.1.2 内部钱包 API

**查询余额**

```http
GET /internal/v1/users/{user_id}/wallet
Authorization: Bearer sk-canvas-service-xxxxxx

响应：
{
  "user_id": "123",
  "balance": 1500000,  // 积分（单位：0.001 USD）
  "used_quota": 500000,
  "display_balance": "$1.50"  // 格式化后的余额
}
```

**预扣积分**

```http
POST /internal/v1/wallet/pre-deduct
Authorization: Bearer sk-canvas-service-xxxxxx
Content-Type: application/json

{
  "user_id": "123",
  "amount": 10000,  // 预估费用 $0.01
  "idempotency_key": "123_task_abc_prededuct",
  "metadata": {
    "task_id": "task_abc",
    "model": "flux-pro",
    "service": "tapcanvas"
  }
}

响应：
{
  "deduction_id": "deduct_xyz789",
  "amount": 10000,
  "balance_after": 1490000
}
```

**结算扣费**

```http
POST /internal/v1/wallet/settle
Authorization: Bearer sk-canvas-service-xxxxxx
Content-Type: application/json

{
  "deduction_id": "deduct_xyz789",
  "actual_cost": 8500,  // 实际费用 $0.0085
  "idempotency_key": "123_task_abc_settle"
}

响应：
{
  "settled": true,
  "refunded_amount": 1500,  // 退回差额
  "balance_after": 1491500
}
```

**退款**

```http
POST /internal/v1/wallet/refund
Authorization: Bearer sk-canvas-service-xxxxxx
Content-Type: application/json

{
  "deduction_id": "deduct_xyz789",
  "idempotency_key": "123_task_abc_refund",
  "reason": "upstream_error"
}

响应：
{
  "refunded": true,
  "amount": 10000,
  "balance_after": 1500000
}
```

#### 6.1.3 模型列表接口（已有，文档补充）

```http
GET /api/models?type=image&type=video
Authorization: Bearer sk-canvas-service-xxxxxx

响应：
{
  "data": [
    {
      "id": "flux-pro",
      "name": "Flux Pro",
      "type": "image",
      "enabled": true,
      "pricing": {
        "per_image": 5000  // $0.005/张
      }
    },
    {
      "id": "minimax-video-01",
      "name": "MiniMax Video",
      "type": "video",
      "enabled": true,
      "pricing": {
        "per_second": 1000  // $0.001/秒
      }
    }
  ]
}
```

### 6.2 TapCanvas 新增接口

#### 6.2.1 OAuth 回调处理

```http
GET /oauth/callback?code={auth_code}&state={random_state}

逻辑：
1. 验证 state 防止 CSRF
2. 用 code 调用 New API /sso/token 换取用户信息
3. 创建本地会话（写入 sessions 表）
4. 设置 Cookie: canvas_session={token}
5. 重定向到画布首页
```

---

## 7. 安全设计

### 7.1 Service Token 管理

**生成方式**：
1. 管理员在 New API 后台手动创建
2. 格式：`sk-canvas-service-{random_32_chars}`
3. 写入 `tokens` 表，设置 `service_name='tapcanvas'`

**权限控制**：
- Service Token 只能调用 `/internal/*` 路径
- IP 白名单限制（可选）：仅允许 TapCanvas 服务器 IP

**存储方式**：
- TapCanvas 服务器：环境变量 `NEW_API_SERVICE_TOKEN`
- 不记录在代码仓库中（`.env` 加入 `.gitignore`）

### 7.2 SSO 安全措施

**防止 CSRF**：
- 发起授权时生成随机 `state`，存储在 session 中
- 回调时验证 `state` 是否匹配

**授权码单次有效**：
- `auth_code` 使用后立即失效
- 有效期 5 分钟，过期自动删除

**HTTPS 强制**：
- 生产环境强制 HTTPS（Let's Encrypt 证书）
- Cookie 设置 `Secure` 和 `HttpOnly` 标记

### 7.3 幂等性保证

**幂等键格式**：
```
{user_id}_{task_id}_{operation_type}
例如：123_task_abc_prededuct
```

**实现方式**：
- 数据库唯一索引：`UNIQUE INDEX idx_wallet_tx_idempotency (idempotency_key)`
- 重复请求返回相同结果（不重复扣费）

---

## 8. 错误处理

### 8.1 错误码规范

| 错误码 | HTTP 状态 | 说明 | 用户提示 |
|--------|----------|------|---------|
| `INSUFFICIENT_QUOTA` | 400 | 积分余额不足 | "余额不足，请前往充值" |
| `MODEL_NOT_AVAILABLE` | 400 | 模型暂时不可用 | "该模型暂时不可用，请选择其他模型" |
| `UPSTREAM_TIMEOUT` | 504 | 上游服务超时 | "生成超时，已退回积分，请重试" |
| `UPSTREAM_ERROR` | 500 | 上游服务错误 | "服务异常，已退回积分，请稍后重试" |
| `RATE_LIMIT_EXCEEDED` | 429 | 请求过于频繁 | "请求过于频繁，请稍后再试" |
| `INVALID_SERVICE_TOKEN` | 401 | Service Token 无效 | "系统错误，请联系管理员" |
| `SESSION_EXPIRED` | 401 | 会话过期 | 自动重定向到登录页 |
| `IDEMPOTENCY_CONFLICT` | 409 | 幂等键冲突 | 返回已有结果（静默处理） |

### 8.2 TapCanvas 错误映射

```typescript
// src/lib/error-mapper.ts
const ERROR_MESSAGES = {
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
  }
}

export function mapError(apiError: ApiError): UserFacingError {
  const template = ERROR_MESSAGES[apiError.code]
  return {
    message: template?.zh || '系统异常，请稍后重试',
    action: template?.action
  }
}
```

### 8.3 服务降级策略

| 故障级别 | 检测方式 | 降级行为 |
|---------|----------|---------|
| **L1 部分功能不可用** | `/health` 返回 `degraded` | 禁用生成按钮，显示"AI 服务维护中" |
| **L2 完全不可达** | 连续 3 次请求超时（5s） | 全局横幅提示，允许浏览历史项目 |
| **L3 紧急维护** | 手动设置 `EMERGENCY_MODE=true` | 重定向到维护页面 |

---

## 9. 监控和运维

### 9.1 关键指标

| 指标 | 监控工具 | 告警阈值 | 说明 |
|------|---------|---------|------|
| **SSO 成功率** | Prometheus | < 95% | SSO 登录成功率 |
| **New API 调用 P99 延迟** | Grafana | > 10s | 内部 API 响应时间 |
| **余额查询失败率** | CloudWatch | > 1% | 钱包查询可用性 |
| **Service Token 401 错误** | Sentry | > 0 | 立即告警（Token 泄露或过期） |
| **幂等键冲突次数** | 日志统计 | > 10/hour | 可能的重试风暴 |
| **预扣未结算数量** | 数据库查询 | > 100 | 可能的资金冻结 |

### 9.2 日志规范

**统一 Request ID**：

```http
# TapCanvas 发起请求
POST https://api.aiplantform.site/v1/images/generations
X-Request-ID: canvas_20261003_a1b2c3d4
X-Canvas-Task-Id: task_xyz123
Authorization: Bearer sk-canvas-service-xxxxx

# New API 日志
[INFO] [canvas_20261003_a1b2c3d4] Relay to channel=12 model=flux-pro
[INFO] [canvas_20261003_a1b2c3d4] Upstream response status=200 duration=3.2s

# TapCanvas 日志
[INFO] [canvas_20261003_a1b2c3d4] Image generation succeeded task=task_xyz123
```

**日志聚合**：
- 使用 ELK（Elasticsearch + Logstash + Kibana）或 Loki
- 按 `request_id` 关联跨服务日志
- 保留 30 天

### 9.3 健康检查

**New API 健康检查接口**：

```http
GET /health

响应：
{
  "status": "healthy",  // healthy | degraded | unhealthy
  "services": {
    "database": "healthy",
    "redis": "healthy",
    "upstream_channels": "degraded"  // 部分上游失效
  },
  "timestamp": "2026-10-03T10:30:00Z"
}
```

**TapCanvas 探测逻辑**：
- 每 30 秒探测一次
- 连续失败 3 次触发降级
- 恢复后自动取消降级

---

## 10. 环境配置

### 10.1 环境对比

| 环境 | TapCanvas URL | New API URL | 数据库 | 需要域名 |
|------|--------------|-------------|--------|---------|
| **开发** | `http://localhost:5173` | `http://localhost:8080` | SQLite 或 PostgreSQL 本地 | ❌ 否 |
| **测试** | `http://localhost:5174` | `http://localhost:8081` | PostgreSQL 本地 | ❌ 否 |
| **生产** | `https://canvas.aiplantform.site` | `https://www.aiplantform.site` | PostgreSQL 服务器 | ✅ 是 |

### 10.2 开发环境配置

**TapCanvas `.env.development`**:
```bash
# New API 本地接口
NEW_API_BASE_URL=http://localhost:8080
NEW_API_AUTHORIZE_URL=http://localhost:8080/sso/authorize
NEW_API_SERVICE_TOKEN=sk-dev-canvas-local-test-xxxxx

# 本地回调地址
CANVAS_REDIRECT_URI=http://localhost:5173/oauth/callback

# 本地数据库
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/tapcanvas_dev

# 开发配置
NODE_ENV=development
```

**New API 开发配置**:
```bash
# 数据库
SQL_DSN=postgresql://postgres:postgres@localhost:5432/new_api_dev

# SSO 白名单
CANVAS_REDIRECT_URI_WHITELIST=http://localhost:5173/oauth/callback,http://localhost:5174/oauth/callback

# 端口
PORT=8080
```

### 10.3 生产环境配置

**TapCanvas `.env.production`**:
```bash
# 生产域名（HTTPS）
NEW_API_BASE_URL=https://www.aiplantform.site
NEW_API_AUTHORIZE_URL=https://www.aiplantform.site/sso/authorize
NEW_API_SERVICE_TOKEN=sk-prod-canvas-xxxxxxxxxxxxxxxx

# 生产回调地址
CANVAS_REDIRECT_URI=https://canvas.aiplantform.site/oauth/callback

# 生产数据库
DATABASE_URL=postgresql://prod_user:strong_password@localhost:5432/tapcanvas

# 生产配置
NODE_ENV=production
SESSION_SECRET=prod-random-secret-xxxxx
```

**New API 生产配置**:
```bash
# 生产数据库
SQL_DSN=postgresql://prod_user:strong_password@localhost:5432/new_api

# SSO 白名单（只允许生产域名）
CANVAS_REDIRECT_URI_WHITELIST=https://canvas.aiplantform.site/oauth/callback

# 端口（Nginx 反向代理）
PORT=8080
TRUSTED_PROXIES=127.0.0.1
```

### 10.4 跨域处理

**开发环境（Vite Proxy）**:
```typescript
// vite.config.ts
export default defineConfig({
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/sso': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      }
    }
  }
})
```

**生产环境（Nginx）**:
```nginx
# /etc/nginx/conf.d/canvas.conf
server {
    listen 443 ssl;
    server_name canvas.aiplantform.site;
    
    ssl_certificate /etc/letsencrypt/live/canvas.aiplantform.site/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/canvas.aiplantform.site/privkey.pem;
    
    location / {
        proxy_pass http://localhost:5173;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## 11. 开发计划

### 11.1 任务分解

#### 阶段 1：New API 改造（2 周）

| 任务 | 工作量 | 依赖 | 负责人 |
|------|-------|------|--------|
| 数据库字段添加（tokens.service_name） | 2 小时 | - | 后端 |
| SSO 授权接口开发 | 1 天 | - | 后端 |
| 内部钱包 API（4个接口） | 3 天 | 数据库改动 | 后端 |
| Service Token 验证中间件 | 1 天 | - | 后端 |
| 前端 UI 调整（按钮置灰） | 4 小时 | 数据库改动 | 前端 |
| 单元测试 | 2 天 | 所有接口 | 后端 |
| 集成测试 | 1 天 | 所有接口 | 测试 |
| 文档编写 | 1 天 | - | 后端 |

#### 阶段 2：TapCanvas 改造（1 周）

| 任务 | 工作量 | 依赖 | 负责人 |
|------|-------|------|--------|
| SSO 客户端实现 | 1 天 | New API SSO 接口 | 前端+后端 |
| New API Client 封装 | 1 天 | - | 后端 |
| 余额查询集成 | 4 小时 | Client 封装 | 前端+后端 |
| 模型列表集成 | 4 小时 | Client 封装 | 前端+后端 |
| AI 任务调用改造 | 2 天 | Client 封装 | 后端 |
| 错误处理和降级 | 1 天 | - | 前端+后端 |
| 单元测试 | 1 天 | 所有功能 | 前端+后端 |

#### 阶段 3：联调和验收（3 天）

| 任务 | 工作量 | 依赖 | 负责人 |
|------|-------|------|--------|
| 本地环境联调 | 1 天 | 阶段1+2 | 全员 |
| 测试环境部署 | 4 小时 | 联调完成 | 运维 |
| 端到端测试 | 1 天 | 测试环境 | 测试 |
| 性能测试 | 4 小时 | 测试环境 | 测试 |
| 验收确认 | 4 小时 | 所有测试 | 产品 |

### 11.2 时间线

```
Week 1-2: New API 改造
  Day 1-2:   数据库改动 + SSO 接口
  Day 3-5:   内部钱包 API
  Day 6-7:   Service Token 中间件
  Day 8-9:   前端 UI + 测试
  Day 10:    文档

Week 3: TapCanvas 改造
  Day 1:     SSO 客户端
  Day 2:     New API Client
  Day 3:     余额+模型集成
  Day 4-5:   AI 任务改造
  Day 6:     错误处理
  Day 7:     测试

Week 4: 联调和验收
  Day 1:     本地联调
  Day 2:     测试环境部署
  Day 3:     端到端测试
```

**总工期**：3-4 周（包含缓冲时间）

---

## 12. 验收标准

### 12.1 功能验收

| 测试场景 | 验收标准 | 优先级 |
|---------|---------|--------|
| **SSO 登录** | 用户从 TapCanvas 跳转到 New API，登录后自动返回 Canvas，无需二次登录 | P0 |
| **会话保持** | 30 天内无需重新登录，到期后自动跳转 SSO | P0 |
| **余额查询** | 页面加载时显示正确的积分余额（与 New API 一致） | P0 |
| **余额不足拦截** | 余额不足时禁用生成按钮，点击时提示充值 | P0 |
| **模型列表同步** | TapCanvas 显示的模型与 New API 后台配置一致 | P0 |
| **AI 任务成功** | 生成图像/视频成功，扣除正确积分，记录日志 | P0 |
| **AI 任务失败** | 失败后自动退款，用户看到友好错误提示 | P0 |
| **幂等性** | 重复提交相同任务不会重复扣费 | P0 |
| **服务降级** | New API 不可用时，TapCanvas 显示维护提示，不崩溃 | P1 |
| **跨环境隔离** | 开发/测试/生产环境数据完全隔离 | P0 |

### 12.2 性能验收

| 指标 | 目标值 | 测量方式 |
|------|--------|---------|
| **SSO 登录耗时** | < 2s（P95） | Chrome DevTools |
| **余额查询耗时** | < 500ms（P95） | API 日志 |
| **模型列表查询耗时** | < 1s（P95） | API 日志 |
| **AI 任务调用延迟** | < 5s（不含上游生成时间） | API 日志 |
| **并发支持** | 100 QPS（AI 任务） | 压力测试 |

### 12.3 安全验收

| 检查项 | 验收标准 |
|--------|---------|
| **Service Token 保密** | Token 不在代码仓库、日志、前端代码中暴露 |
| **HTTPS 强制** | 生产环境所有请求使用 HTTPS |
| **Cookie 安全** | Cookie 设置 `HttpOnly`, `Secure`, `SameSite=Lax` |
| **CSRF 防护** | SSO 流程使用 `state` 参数验证 |
| **SQL 注入防护** | 所有数据库查询使用参数化查询 |
| **幂等键唯一** | 幂等键格式正确，数据库唯一索引生效 |

### 12.4 运维验收

| 检查项 | 验收标准 |
|--------|---------|
| **健康检查** | `/health` 接口返回正确的服务状态 |
| **日志完整性** | 所有 API 调用记录日志，包含 `request_id` |
| **监控配置** | 关键指标接入 Prometheus/Grafana |
| **告警规则** | SSO 失败率、余额查询失败配置告警 |
| **回滚计划** | 有明确的回滚步骤和数据恢复方案 |

---

## 附录

### A. 术语表

| 术语 | 说明 |
|------|------|
| **SSO** | Single Sign-On，单点登录 |
| **Service Token** | 服务级别的 API Token，用于子系统调用主系统的内部 API |
| **幂等性** | 相同请求多次执行与一次执行效果相同 |
| **预扣-结算** | 先冻结费用，任务完成后扣除实际费用，退回差额 |
| **熔断器** | 当服务故障率过高时，暂时停止调用，避免雪崩 |

### B. 参考文档

- New API 项目：https://github.com/1240052492/new-api
- TapCanvas 项目：https://github.com/1240052492/TapCanvas
- OAuth 2.0 规范：https://oauth.net/2/
- 幂等性设计最佳实践：https://stripe.com/docs/api/idempotent_requests

### C. 变更记录

| 版本 | 日期 | 变更内容 | 作者 |
|------|------|---------|------|
| v1.0 | 2026-10-03 | 初始版本 | Claude |

---

**文档结束**
