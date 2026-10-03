# New API + TapCanvas Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate New API (AI model gateway) with TapCanvas (video creation platform) through SSO authentication and internal API calls for shared user accounts, wallet, and model catalog.

**Architecture:** Two independent services with SSO authentication flow. TapCanvas uses Service Token to call New API's internal wallet APIs for balance queries and credit management. New API remains authoritative for user accounts, credits, and model routing.

**Tech Stack:** 
- New API: Go (Gin framework), GORM, PostgreSQL/MySQL/SQLite
- TapCanvas: TypeScript (Hono framework), Prisma, PostgreSQL

**Spec:** `E:\ZSXQ\API_Canvas\docs\superpowers\specs\2026-10-03-new-api-tapcanvas-integration-design.md`

## Global Constraints

- PostgreSQL 9.6+ (production), SQLite 3.35+ (development)
- Go 1.19+, Node.js 18+
- Service Token format: `sk-canvas-service-{32_random_chars}`
- Idempotency key format: `{user_id}_{task_id}_{operation_type}`
- Session duration: 30 days (2592000 seconds)
- Authorization code TTL: 5 minutes (300 seconds)
- HTTPS required in production (Let's Encrypt certificates)
- Credential environment variables must not be committed to git

## Review Focus

1. **Idempotency key collision**: When two TapCanvas instances submit the same task_id for different users, the key `{user_id}_{task_id}_{operation}` must remain unique. Tests verify that `user_id` prefix prevents cross-user collisions.

2. **Authorization code replay**: After `/sso/token` exchanges a code once, a second exchange attempt with the same code must fail with 400. Tests verify codes are single-use.

3. **Service Token leakage in logs**: When logging API requests, the `Authorization: Bearer sk-canvas-service-xxx` header must be redacted to `Authorization: Bearer sk-***`. Tests grep application logs for the full token string.

4. **Stale session detection**: When a Canvas session token's `expires_at` is in the past, middleware must reject the request with 401 and redirect to SSO, not serve stale user data. Tests verify middleware checks `expires_at < NOW()`.

5. **Partial credit refund**: When pre-deduct reserves 10000 credits but actual_cost is 8500, settle must refund exactly 1500 (not 0, not 10000). Tests verify `balance_after = balance_before - actual_cost`, not `balance_before - pre_deduct_amount`.

---

## Part 1: New API - Database Schema Changes

### Task 1: Add service_name Column to tokens Table

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\model\token.go`
- Create: `E:\ZSXQ\API_Canvas\apps\new-api\model\migrations\add_service_name_to_tokens.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\model\token_test.go`

**Interfaces:**
- Consumes: Existing `Token` struct with `Key`, `UserId`, `Status` fields
- Produces: `Token.ServiceName string` field (nullable), migration function `MigrateServiceNameColumn() error`

- [ ] **Step 1: Write failing test for service_name field**

```go
// token_test.go
func TestTokenServiceNameField(t *testing.T) {
    token := &Token{
        Key:         "sk-canvas-service-test123",
        UserId:      1,
        Status:      1,
        ServiceName: "tapcanvas",
    }
    err := DB.Create(token).Error
    assert.NoError(t, err)
    
    var loaded Token
    DB.First(&loaded, token.Id)
    assert.Equal(t, "tapcanvas", loaded.ServiceName)
}

func TestTokenServiceNameNullable(t *testing.T) {
    token := &Token{
        Key:    "sk-user-key-abc",
        UserId: 1,
        Status: 1,
        // ServiceName omitted
    }
    err := DB.Create(token).Error
    assert.NoError(t, err)
    
    var loaded Token
    DB.First(&loaded, token.Id)
    assert.Empty(t, loaded.ServiceName)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd E:\ZSXQ\API_Canvas\apps\new-api && go test ./model -run TestTokenServiceName -v`
Expected: FAIL with "unknown column: service_name"

- [ ] **Step 3: Add ServiceName field to Token struct in `model/token.go`**

Add after existing fields:
```go
ServiceName string `gorm:"column:service_name;type:varchar(64);default:null" json:"service_name"`
```

- [ ] **Step 4: Create migration in `model/migrations/add_service_name_to_tokens.go`**

```go
package model

func MigrateServiceNameColumn() error {
    if DB.Migrator().HasColumn(&Token{}, "service_name") {
        return nil
    }
    return DB.Migrator().AddColumn(&Token{}, "service_name")
}
```

- [ ] **Step 5: Register migration in `model/main.go` migrateDB() function**

Add before `return nil`:
```go
if err := MigrateServiceNameColumn(); err != nil {
    return fmt.Errorf("migrate service_name column: %w", err)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./model -run TestTokenServiceName -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add model/token.go model/migrations/add_service_name_to_tokens.go model/token_test.go model/main.go
git commit -m "feat(db): add service_name column to tokens table"
```

---

### Task 2: Create wallet_transactions Table

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\new-api\model\wallet_transaction.go`
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\model\main.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\model\wallet_transaction_test.go`

**Interfaces:**
- Consumes: GORM DB connection, existing migration pattern from Task 1
- Produces: `WalletTransaction` struct, `CreateWalletTransaction(tx *WalletTransaction) error`, `GetTransactionByIdempotencyKey(key string) (*WalletTransaction, error)`

- [ ] **Step 1: Write failing test for wallet_transactions table**

```go
// wallet_transaction_test.go
func TestCreateWalletTransaction(t *testing.T) {
    tx := &WalletTransaction{
        Id:             "tx_abc123",
        UserId:         1,
        Type:           "pre_deduct",
        Amount:         10000,
        IdempotencyKey: "1_task_abc_prededuct",
        DeductionId:    "",
        Metadata:       `{"task_id":"task_abc"}`,
    }
    err := CreateWalletTransaction(tx)
    assert.NoError(t, err)
}

func TestIdempotencyKeyUnique(t *testing.T) {
    tx1 := &WalletTransaction{
        Id:             "tx_1",
        UserId:         1,
        Type:           "pre_deduct",
        Amount:         10000,
        IdempotencyKey: "1_task_duplicate_prededuct",
    }
    CreateWalletTransaction(tx1)
    
    tx2 := &WalletTransaction{
        Id:             "tx_2",
        UserId:         1,
        Type:           "pre_deduct",
        Amount:         10000,
        IdempotencyKey: "1_task_duplicate_prededuct",
    }
    err := CreateWalletTransaction(tx2)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "duplicate")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./model -run TestWalletTransaction -v`
Expected: FAIL with "undefined: WalletTransaction"

- [ ] **Step 3: Create WalletTransaction struct in `model/wallet_transaction.go`**

```go
package model

import (
    "time"
    "gorm.io/gorm"
)

type WalletTransaction struct {
    Id             string    `gorm:"column:id;primary_key" json:"id"`
    UserId         int       `gorm:"column:user_id;not null;index:idx_wallet_tx_user_created" json:"user_id"`
    Type           string    `gorm:"column:type;type:varchar(20);not null" json:"type"`
    Amount         int64     `gorm:"column:amount;not null" json:"amount"`
    IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(128);uniqueIndex:idx_wallet_tx_idempotency;not null" json:"idempotency_key"`
    DeductionId    string    `gorm:"column:deduction_id;type:varchar(64)" json:"deduction_id,omitempty"`
    Metadata       string    `gorm:"column:metadata;type:text" json:"metadata,omitempty"`
    CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (WalletTransaction) TableName() string {
    return "wallet_transactions"
}

func CreateWalletTransaction(tx *WalletTransaction) error {
    return DB.Create(tx).Error
}

func GetTransactionByIdempotencyKey(key string) (*WalletTransaction, error) {
    var tx WalletTransaction
    err := DB.Where("idempotency_key = ?", key).First(&tx).Error
    if err == gorm.ErrRecordNotFound {
        return nil, nil
    }
    return &tx, err
}
```

- [ ] **Step 4: Register table in `model/main.go` migrateDB() autoMigrateModels call**

Add `&WalletTransaction{}` to the list of models.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./model -run TestWalletTransaction -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add model/wallet_transaction.go model/wallet_transaction_test.go model/main.go
git commit -m "feat(db): add wallet_transactions table for credit flow tracking"
```

---

## Part 2: New API - SSO Authorization

### Task 3: SSO Authorization Endpoint (GET /sso/authorize)

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\new-api\controller\sso.go`
- Create: `E:\ZSXQ\API_Canvas\apps\new-api\model\auth_code.go`
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\router\main.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\controller\sso_test.go`

**Interfaces:**
- Consumes: User session middleware (existing), redirect_uri whitelist from env `CANVAS_REDIRECT_URI_WHITELIST`
- Produces: `GET /sso/authorize` handler, `AuthCode` model with `Code string`, `UserId int`, `RedirectUri string`, `ExpiresAt time.Time`

- [ ] **Step 1: Write failing test for SSO authorize redirect**

```go
// sso_test.go
func TestSSOAuthorizeRedirectsToLogin(t *testing.T) {
    router := setupTestRouter()
    req := httptest.NewRequest("GET", "/sso/authorize?redirect_uri=http://localhost:5173/oauth/callback&state=xyz", nil)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusFound, w.Code)
    location := w.Header().Get("Location")
    assert.Contains(t, location, "/login")
}

func TestSSOAuthorizeGeneratesCode(t *testing.T) {
    router := setupTestRouter()
    req := httptest.NewRequest("GET", "/sso/authorize?redirect_uri=http://localhost:5173/oauth/callback&state=xyz", nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: testUserSession})
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusFound, w.Code)
    location := w.Header().Get("Location")
    assert.Contains(t, location, "http://localhost:5173/oauth/callback?code=")
    assert.Contains(t, location, "&state=xyz")
}

func TestSSOAuthorizeRejectsInvalidRedirectUri(t *testing.T) {
    router := setupTestRouter()
    req := httptest.NewRequest("GET", "/sso/authorize?redirect_uri=http://evil.com/callback", nil)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controller -run TestSSOAuthorize -v`
Expected: FAIL with "404 page not found"

- [ ] **Step 3: Create AuthCode model in `model/auth_code.go`**

```go
package model

import (
    "crypto/rand"
    "encoding/base64"
    "time"
    "gorm.io/gorm"
)

type AuthCode struct {
    Code        string    `gorm:"column:code;primary_key" json:"code"`
    UserId      int       `gorm:"column:user_id;not null" json:"user_id"`
    RedirectUri string    `gorm:"column:redirect_uri;not null" json:"redirect_uri"`
    State       string    `gorm:"column:state" json:"state"`
    Used        bool      `gorm:"column:used;default:false" json:"used"`
    ExpiresAt   time.Time `gorm:"column:expires_at;not null" json:"expires_at"`
    CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (AuthCode) TableName() string {
    return "auth_codes"
}

func GenerateAuthCode(userId int, redirectUri, state string) (*AuthCode, error) {
    bytes := make([]byte, 24)
    if _, err := rand.Read(bytes); err != nil {
        return nil, err
    }
    code := base64.URLEncoding.EncodeToString(bytes)
    
    authCode := &AuthCode{
        Code:        code,
        UserId:      userId,
        RedirectUri: redirectUri,
        State:       state,
        Used:        false,
        ExpiresAt:   time.Now().Add(5 * time.Minute),
    }
    err := DB.Create(authCode).Error
    return authCode, err
}

func ConsumeAuthCode(code, redirectUri string) (*AuthCode, error) {
    var authCode AuthCode
    err := DB.Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("code = ? AND redirect_uri = ?", code, redirectUri).First(&authCode).Error; err != nil {
            return err
        }
        if authCode.Used {
            return errors.New("auth code already used")
        }
        if time.Now().After(authCode.ExpiresAt) {
            return errors.New("auth code expired")
        }
        return tx.Model(&authCode).Update("used", true).Error
    })
    return &authCode, err
}
```

- [ ] **Step 4: Register AuthCode table in `model/main.go` migrateDB()**

Add `&AuthCode{}` to autoMigrateModels.

- [ ] **Step 5: Implement SSOAuthorize handler in `controller/sso.go`**

```go
package controller

import (
    "net/http"
    "strings"
    "github.com/gin-gonic/gin"
    "github.com/QuantumNous/new-api/model"
    "github.com/QuantumNous/new-api/common"
)

func SSOAuthorize(c *gin.Context) {
    redirectUri := c.Query("redirect_uri")
    state := c.Query("state")
    
    // Validate redirect_uri against whitelist
    whitelist := strings.Split(common.GetEnvOrDefault("CANVAS_REDIRECT_URI_WHITELIST", ""), ",")
    valid := false
    for _, allowed := range whitelist {
        if strings.TrimSpace(allowed) == redirectUri {
            valid = true
            break
        }
    }
    if !valid {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_redirect_uri"})
        return
    }
    
    // Check if user is logged in
    userId := c.GetInt("user_id")
    if userId == 0 {
        // Redirect to login with return_url
        c.Redirect(http.StatusFound, "/login?return_url="+c.Request.URL.String())
        return
    }
    
    // Generate auth code
    authCode, err := model.GenerateAuthCode(userId, redirectUri, state)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate code"})
        return
    }
    
    // Redirect back with code
    separator := "?"
    if strings.Contains(redirectUri, "?") {
        separator = "&"
    }
    location := redirectUri + separator + "code=" + authCode.Code
    if state != "" {
        location += "&state=" + state
    }
    c.Redirect(http.StatusFound, location)
}
```

- [ ] **Step 6: Register route in `router/main.go`**

Add: `router.GET("/sso/authorize", controller.SSOAuthorize)`

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./controller -run TestSSOAuthorize -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add controller/sso.go model/auth_code.go controller/sso_test.go router/main.go model/main.go
git commit -m "feat(sso): add /sso/authorize endpoint"
```

---

### Task 4: SSO Token Exchange Endpoint (POST /sso/token)

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\controller\sso.go`
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\router\main.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\controller\sso_test.go`

**Interfaces:**
- Consumes: `ConsumeAuthCode(code, redirectUri string) (*AuthCode, error)` from Task 3, `User` model with `Id`, `Username`, `Email`, `AvatarUrl`
- Produces: `POST /sso/token` handler returning JSON with `user_id`, `username`, `email`, `avatar_url`, `expires_in`

- [ ] **Step 1: Write failing test for token exchange**

```go
// sso_test.go (append to file)
func TestSSOTokenExchangeSuccess(t *testing.T) {
    // Setup: create auth code
    authCode, _ := model.GenerateAuthCode(testUserId, "http://localhost:5173/oauth/callback", "xyz")
    
    router := setupTestRouter()
    body := `{"code":"` + authCode.Code + `","redirect_uri":"http://localhost:5173/oauth/callback"}`
    req := httptest.NewRequest("POST", "/sso/token", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.Equal(t, float64(testUserId), resp["user_id"])
    assert.NotEmpty(t, resp["username"])
    assert.Equal(t, float64(2592000), resp["expires_in"])
}

func TestSSOTokenExchangeRejectsUsedCode(t *testing.T) {
    authCode, _ := model.GenerateAuthCode(testUserId, "http://localhost:5173/oauth/callback", "")
    model.ConsumeAuthCode(authCode.Code, "http://localhost:5173/oauth/callback")
    
    router := setupTestRouter()
    body := `{"code":"` + authCode.Code + `","redirect_uri":"http://localhost:5173/oauth/callback"}`
    req := httptest.NewRequest("POST", "/sso/token", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controller -run TestSSOTokenExchange -v`
Expected: FAIL with "404 page not found"

- [ ] **Step 3: Implement SSOTokenExchange handler in `controller/sso.go`**

```go
type SSOTokenRequest struct {
    Code        string `json:"code" binding:"required"`
    RedirectUri string `json:"redirect_uri" binding:"required"`
}

type SSOTokenResponse struct {
    UserId     int    `json:"user_id"`
    Username   string `json:"username"`
    Email      string `json:"email"`
    AvatarUrl  string `json:"avatar_url"`
    ExpiresIn  int    `json:"expires_in"`
}

func SSOTokenExchange(c *gin.Context) {
    var req SSOTokenRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
        return
    }
    
    authCode, err := model.ConsumeAuthCode(req.Code, req.RedirectUri)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    user, err := model.GetUserById(authCode.UserId, false)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
        return
    }
    
    c.JSON(http.StatusOK, SSOTokenResponse{
        UserId:    user.Id,
        Username:  user.Username,
        Email:     user.Email,
        AvatarUrl: user.AvatarUrl,
        ExpiresIn: 2592000, // 30 days
    })
}
```

- [ ] **Step 4: Register route in `router/main.go`**

Add: `router.POST("/sso/token", controller.SSOTokenExchange)`

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controller -run TestSSOTokenExchange -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add controller/sso.go controller/sso_test.go router/main.go
git commit -m "feat(sso): add /sso/token exchange endpoint"
```

---

## Part 3: New API - Internal Wallet APIs

### Task 5: Service Token Validation Middleware

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\new-api\middleware\service_auth.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\middleware\service_auth_test.go`

**Interfaces:**
- Consumes: `Token` model with `ServiceName` field from Task 1
- Produces: `ServiceTokenAuth() gin.HandlerFunc` middleware that validates `Authorization: Bearer sk-canvas-service-xxx` header

- [ ] **Step 1: Write failing test for service token validation**

```go
// service_auth_test.go
func TestServiceTokenAuthSuccess(t *testing.T) {
    // Setup: create service token
    token := &model.Token{
        Key:         "sk-canvas-service-test123",
        UserId:      1,
        Status:      1,
        ServiceName: "tapcanvas",
    }
    model.DB.Create(token)
    
    router := gin.New()
    router.Use(middleware.ServiceTokenAuth())
    router.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"ok": true})
    })
    
    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("Authorization", "Bearer sk-canvas-service-test123")
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
}

func TestServiceTokenAuthRejectsUserToken(t *testing.T) {
    token := &model.Token{
        Key:         "sk-user-key-abc",
        UserId:      1,
        Status:      1,
        ServiceName: "", // User token, not service token
    }
    model.DB.Create(token)
    
    router := gin.New()
    router.Use(middleware.ServiceTokenAuth())
    router.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"ok": true})
    })
    
    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("Authorization", "Bearer sk-user-key-abc")
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 401, w.Code)
}

func TestServiceTokenAuthRedactsTokenInLogs(t *testing.T) {
    // Capture log output
    var logBuf bytes.Buffer
    log.SetOutput(&logBuf)
    
    router := gin.New()
    router.Use(middleware.ServiceTokenAuth())
    router.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"ok": true})
    })
    
    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("Authorization", "Bearer sk-canvas-service-secret123")
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    logOutput := logBuf.String()
    assert.NotContains(t, logOutput, "sk-canvas-service-secret123")
    assert.Contains(t, logOutput, "sk-***")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./middleware -run TestServiceTokenAuth -v`
Expected: FAIL with "undefined: ServiceTokenAuth"

- [ ] **Step 3: Implement ServiceTokenAuth middleware in `middleware/service_auth.go`**

```go
package middleware

import (
    "net/http"
    "strings"
    "github.com/gin-gonic/gin"
    "github.com/QuantumNous/new-api/model"
    "github.com/QuantumNous/new-api/logger"
)

func ServiceTokenAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
            c.Abort()
            return
        }
        
        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) != 2 || parts[0] != "Bearer" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
            c.Abort()
            return
        }
        
        tokenKey := parts[1]
        
        // Redact token in request context for logging
        c.Set("_redacted_token", "sk-***")
        
        token, err := model.ValidateUserToken(tokenKey)
        if err != nil || token == nil {
            logger.LogError(c, "invalid service token")
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
            c.Abort()
            return
        }
        
        // Verify this is a service token
        if token.ServiceName == "" {
            logger.LogError(c, "user token used for service endpoint")
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token_type"})
            c.Abort()
            return
        }
        
        c.Set("service_name", token.ServiceName)
        c.Set("token_id", token.Id)
        c.Next()
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./middleware -run TestServiceTokenAuth -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add middleware/service_auth.go middleware/service_auth_test.go
git commit -m "feat(auth): add service token validation middleware"
```

---

### Task 6: Wallet Balance Query API (GET /internal/v1/users/:id/wallet)

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet.go`
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\router\main.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet_test.go`

**Interfaces:**
- Consumes: `ServiceTokenAuth()` middleware from Task 5, `User` model with `Quota`, `UsedQuota` fields
- Produces: `GET /internal/v1/users/:id/wallet` handler returning `user_id`, `balance`, `used_quota`, `display_balance`

- [ ] **Step 1: Write failing test for wallet query**

```go
// internal_wallet_test.go
func TestGetWalletBalance(t *testing.T) {
    user := &model.User{
        Username:  "alice",
        Quota:     2000000, // $2.00
        UsedQuota: 500000,  // $0.50
    }
    model.DB.Create(user)
    
    router := setupTestRouterWithServiceAuth()
    req := httptest.NewRequest("GET", "/internal/v1/users/"+strconv.Itoa(user.Id)+"/wallet", nil)
    req.Header.Set("Authorization", "Bearer "+testServiceToken)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.Equal(t, float64(user.Id), resp["user_id"])
    assert.Equal(t, float64(1500000), resp["balance"]) // 2000000 - 500000
    assert.Equal(t, "$1.50", resp["display_balance"])
}

func TestGetWalletBalanceNotFound(t *testing.T) {
    router := setupTestRouterWithServiceAuth()
    req := httptest.NewRequest("GET", "/internal/v1/users/99999/wallet", nil)
    req.Header.Set("Authorization", "Bearer "+testServiceToken)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 404, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controller -run TestGetWalletBalance -v`
Expected: FAIL with "404 page not found"

- [ ] **Step 3: Implement GetWalletBalance handler in `controller/internal_wallet.go`**

```go
package controller

import (
    "fmt"
    "net/http"
    "strconv"
    "github.com/gin-gonic/gin"
    "github.com/QuantumNous/new-api/model"
)

type WalletBalanceResponse struct {
    UserId         int    `json:"user_id"`
    Balance        int64  `json:"balance"`
    UsedQuota      int64  `json:"used_quota"`
    DisplayBalance string `json:"display_balance"`
}

func GetWalletBalance(c *gin.Context) {
    userIdStr := c.Param("id")
    userId, err := strconv.Atoi(userIdStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id"})
        return
    }
    
    user, err := model.GetUserById(userId, false)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
        return
    }
    
    balance := user.Quota - user.UsedQuota
    displayBalance := fmt.Sprintf("$%.2f", float64(balance)/500000.0)
    
    c.JSON(http.StatusOK, WalletBalanceResponse{
        UserId:         user.Id,
        Balance:        balance,
        UsedQuota:      user.UsedQuota,
        DisplayBalance: displayBalance,
    })
}
```

- [ ] **Step 4: Register route in `router/main.go`**

```go
internal := router.Group("/internal/v1")
internal.Use(middleware.ServiceTokenAuth())
{
    internal.GET("/users/:id/wallet", controller.GetWalletBalance)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controller -run TestGetWalletBalance -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add controller/internal_wallet.go controller/internal_wallet_test.go router/main.go
git commit -m "feat(wallet): add balance query endpoint"
```

---

### Task 7: Pre-Deduct Credits API (POST /internal/v1/wallet/pre-deduct)

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet_test.go`

**Interfaces:**
- Consumes: `ServiceTokenAuth()` from Task 5, `WalletTransaction` and `CreateWalletTransaction()` from Task 2
- Produces: `POST /internal/v1/wallet/pre-deduct` handler, idempotent credit freezing

- [ ] **Step 1: Write failing test for pre-deduct**

```go
// internal_wallet_test.go (append)
func TestPreDeductSuccess(t *testing.T) {
    user := createTestUserWithQuota(2000000)
    
    router := setupTestRouterWithServiceAuth()
    body := `{
        "user_id": ` + strconv.Itoa(user.Id) + `,
        "amount": 10000,
        "idempotency_key": "` + strconv.Itoa(user.Id) + `_task_abc_prededuct",
        "metadata": {"task_id": "task_abc"}
    }`
    req := httptest.NewRequest("POST", "/internal/v1/wallet/pre-deduct", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+testServiceToken)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.NotEmpty(t, resp["deduction_id"])
    assert.Equal(t, float64(1990000), resp["balance_after"])
    
    // Verify user quota decreased
    user, _ = model.GetUserById(user.Id, false)
    assert.Equal(t, int64(10000), user.UsedQuota)
}

func TestPreDeductIdempotency(t *testing.T) {
    user := createTestUserWithQuota(2000000)
    idempotencyKey := strconv.Itoa(user.Id) + "_task_idem_prededuct"
    
    router := setupTestRouterWithServiceAuth()
    body := `{
        "user_id": ` + strconv.Itoa(user.Id) + `,
        "amount": 10000,
        "idempotency_key": "` + idempotencyKey + `"
    }`
    
    // First request
    req1 := httptest.NewRequest("POST", "/internal/v1/wallet/pre-deduct", strings.NewReader(body))
    req1.Header.Set("Content-Type", "application/json")
    req1.Header.Set("Authorization", "Bearer "+testServiceToken)
    w1 := httptest.NewRecorder()
    router.ServeHTTP(w1, req1)
    
    var resp1 map[string]interface{}
    json.Unmarshal(w1.Body.Bytes(), &resp1)
    deductionId1 := resp1["deduction_id"]
    
    // Second request with same idempotency key
    req2 := httptest.NewRequest("POST", "/internal/v1/wallet/pre-deduct", strings.NewReader(body))
    req2.Header.Set("Content-Type", "application/json")
    req2.Header.Set("Authorization", "Bearer "+testServiceToken)
    w2 := httptest.NewRecorder()
    router.ServeHTTP(w2, req2)
    
    var resp2 map[string]interface{}
    json.Unmarshal(w2.Body.Bytes(), &resp2)
    
    // Should return same deduction_id, not deduct twice
    assert.Equal(t, deductionId1, resp2["deduction_id"])
    
    user, _ = model.GetUserById(user.Id, false)
    assert.Equal(t, int64(10000), user.UsedQuota) // Only deducted once
}

func TestPreDeductInsufficientBalance(t *testing.T) {
    user := createTestUserWithQuota(5000) // Only $0.005
    
    router := setupTestRouterWithServiceAuth()
    body := `{
        "user_id": ` + strconv.Itoa(user.Id) + `,
        "amount": 10000,
        "idempotency_key": "` + strconv.Itoa(user.Id) + `_task_fail_prededuct"
    }`
    req := httptest.NewRequest("POST", "/internal/v1/wallet/pre-deduct", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+testServiceToken)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 400, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.Equal(t, "INSUFFICIENT_QUOTA", resp["code"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controller -run TestPreDeduct -v`
Expected: FAIL with "404 page not found"

- [ ] **Step 3: Implement PreDeductCredits handler in `controller/internal_wallet.go`**

```go
type PreDeductRequest struct {
    UserId         int               `json:"user_id" binding:"required"`
    Amount         int64             `json:"amount" binding:"required"`
    IdempotencyKey string            `json:"idempotency_key" binding:"required"`
    Metadata       map[string]string `json:"metadata"`
}

type PreDeductResponse struct {
    DeductionId  string `json:"deduction_id"`
    Amount       int64  `json:"amount"`
    BalanceAfter int64  `json:"balance_after"`
}

func PreDeductCredits(c *gin.Context) {
    var req PreDeductRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
        return
    }
    
    // Check idempotency
    existing, err := model.GetTransactionByIdempotencyKey(req.IdempotencyKey)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
        return
    }
    if existing != nil {
        // Return existing result
        user, _ := model.GetUserById(req.UserId, false)
        balance := user.Quota - user.UsedQuota
        c.JSON(http.StatusOK, PreDeductResponse{
            DeductionId:  existing.Id,
            Amount:       existing.Amount,
            BalanceAfter: balance,
        })
        return
    }
    
    // Transaction: check balance and deduct
    var balanceAfter int64
    deductionId := "deduct_" + generateRandomString(16)
    
    err = model.DB.Transaction(func(tx *gorm.DB) error {
        var user model.User
        if err := tx.Where("id = ?", req.UserId).First(&user).Error; err != nil {
            return err
        }
        
        balance := user.Quota - user.UsedQuota
        if balance < req.Amount {
            return errors.New("insufficient quota")
        }
        
        // Increase used_quota (freeze credits)
        if err := tx.Model(&user).Update("used_quota", gorm.Expr("used_quota + ?", req.Amount)).Error; err != nil {
            return err
        }
        
        balanceAfter = balance - req.Amount
        
        // Record transaction
        metadataJSON, _ := json.Marshal(req.Metadata)
        walletTx := &model.WalletTransaction{
            Id:             deductionId,
            UserId:         req.UserId,
            Type:           "pre_deduct",
            Amount:         req.Amount,
            IdempotencyKey: req.IdempotencyKey,
            DeductionId:    "",
            Metadata:       string(metadataJSON),
        }
        return tx.Create(walletTx).Error
    })
    
    if err != nil {
        if strings.Contains(err.Error(), "insufficient") {
            c.JSON(http.StatusBadRequest, gin.H{"code": "INSUFFICIENT_QUOTA", "error": err.Error()})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        }
        return
    }
    
    c.JSON(http.StatusOK, PreDeductResponse{
        DeductionId:  deductionId,
        Amount:       req.Amount,
        BalanceAfter: balanceAfter,
    })
}
```

- [ ] **Step 4: Register route in `router/main.go`**

Add to internal group: `internal.POST("/wallet/pre-deduct", controller.PreDeductCredits)`

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controller -run TestPreDeduct -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add controller/internal_wallet.go controller/internal_wallet_test.go router/main.go
git commit -m "feat(wallet): add pre-deduct credits endpoint with idempotency"
```

---

### Task 8: Settle Credits API (POST /internal/v1/wallet/settle)

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet_test.go`

**Interfaces:**
- Consumes: `GetTransactionByIdempotencyKey()` from Task 2, pre-deduct transaction from Task 7
- Produces: `POST /internal/v1/wallet/settle` handler that refunds difference between pre-deduct and actual cost

- [ ] **Step 1: Write failing test for settle**

```go
// internal_wallet_test.go (append)
func TestSettleCredits(t *testing.T) {
    user := createTestUserWithQuota(2000000)
    
    // Pre-deduct 10000
    deductionId := preDeductForTest(user.Id, 10000, "settle_test")
    
    router := setupTestRouterWithServiceAuth()
    body := `{
        "deduction_id": "` + deductionId + `",
        "actual_cost": 8500,
        "idempotency_key": "` + strconv.Itoa(user.Id) + `_task_settle_test_settle"
    }`
    req := httptest.NewRequest("POST", "/internal/v1/wallet/settle", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+testServiceToken)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.True(t, resp["settled"].(bool))
    assert.Equal(t, float64(1500), resp["refunded_amount"]) // 10000 - 8500
    assert.Equal(t, float64(1991500), resp["balance_after"]) // 2000000 - 8500
    
    // Verify final used_quota is actual_cost
    user, _ = model.GetUserById(user.Id, false)
    assert.Equal(t, int64(8500), user.UsedQuota)
}

func TestSettleIdempotency(t *testing.T) {
    user := createTestUserWithQuota(2000000)
    deductionId := preDeductForTest(user.Id, 10000, "settle_idem")
    idempotencyKey := strconv.Itoa(user.Id) + "_task_settle_idem_settle"
    
    body := `{
        "deduction_id": "` + deductionId + `",
        "actual_cost": 8500,
        "idempotency_key": "` + idempotencyKey + `"
    }`
    
    router := setupTestRouterWithServiceAuth()
    
    // First settle
    req1 := httptest.NewRequest("POST", "/internal/v1/wallet/settle", strings.NewReader(body))
    req1.Header.Set("Content-Type", "application/json")
    req1.Header.Set("Authorization", "Bearer "+testServiceToken)
    w1 := httptest.NewRecorder()
    router.ServeHTTP(w1, req1)
    assert.Equal(t, 200, w1.Code)
    
    // Second settle with same key
    req2 := httptest.NewRequest("POST", "/internal/v1/wallet/settle", strings.NewReader(body))
    req2.Header.Set("Content-Type", "application/json")
    req2.Header.Set("Authorization", "Bearer "+testServiceToken)
    w2 := httptest.NewRecorder()
    router.ServeHTTP(w2, req2)
    assert.Equal(t, 200, w2.Code)
    
    // Verify only settled once
    user, _ = model.GetUserById(user.Id, false)
    assert.Equal(t, int64(8500), user.UsedQuota)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controller -run TestSettle -v`
Expected: FAIL with "404 page not found"

- [ ] **Step 3: Implement SettleCredits handler in `controller/internal_wallet.go`**

```go
type SettleRequest struct {
    DeductionId    string `json:"deduction_id" binding:"required"`
    ActualCost     int64  `json:"actual_cost" binding:"required"`
    IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

type SettleResponse struct {
    Settled        bool  `json:"settled"`
    RefundedAmount int64 `json:"refunded_amount"`
    BalanceAfter   int64 `json:"balance_after"`
}

func SettleCredits(c *gin.Context) {
    var req SettleRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
        return
    }
    
    // Check idempotency
    existing, _ := model.GetTransactionByIdempotencyKey(req.IdempotencyKey)
    if existing != nil {
        // Already settled, return success
        c.JSON(http.StatusOK, SettleResponse{Settled: true})
        return
    }
    
    // Get pre-deduct transaction
    var preDeduct model.WalletTransaction
    if err := model.DB.Where("id = ? AND type = ?", req.DeductionId, "pre_deduct").First(&preDeduct).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "deduction_not_found"})
        return
    }
    
    refundAmount := preDeduct.Amount - req.ActualCost
    var balanceAfter int64
    
    err := model.DB.Transaction(func(tx *gorm.DB) error {
        var user model.User
        if err := tx.Where("id = ?", preDeduct.UserId).First(&user).Error; err != nil {
            return err
        }
        
        // Refund difference: decrease used_quota by refundAmount
        if refundAmount > 0 {
            if err := tx.Model(&user).Update("used_quota", gorm.Expr("used_quota - ?", refundAmount)).Error; err != nil {
                return err
            }
        }
        
        balanceAfter = user.Quota - user.UsedQuota + refundAmount
        
        // Record settle transaction
        settleTx := &model.WalletTransaction{
            Id:             "settle_" + generateRandomString(16),
            UserId:         preDeduct.UserId,
            Type:           "settle",
            Amount:         req.ActualCost,
            IdempotencyKey: req.IdempotencyKey,
            DeductionId:    req.DeductionId,
        }
        return tx.Create(settleTx).Error
    })
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, SettleResponse{
        Settled:        true,
        RefundedAmount: refundAmount,
        BalanceAfter:   balanceAfter,
    })
}
```

- [ ] **Step 4: Register route in `router/main.go`**

Add to internal group: `internal.POST("/wallet/settle", controller.SettleCredits)`

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controller -run TestSettle -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add controller/internal_wallet.go controller/internal_wallet_test.go router/main.go
git commit -m "feat(wallet): add settle credits endpoint with refund logic"
```

---

### Task 9: Refund Credits API (POST /internal/v1/wallet/refund)

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet.go`
- Test: `E:\ZSXQ\API_Canvas\apps\new-api\controller\internal_wallet_test.go`

**Interfaces:**
- Consumes: Pre-deduct transaction from Task 7
- Produces: `POST /internal/v1/wallet/refund` handler that fully refunds a pre-deducted amount

- [ ] **Step 1: Write failing test for refund**

```go
// internal_wallet_test.go (append)
func TestRefundCredits(t *testing.T) {
    user := createTestUserWithQuota(2000000)
    deductionId := preDeductForTest(user.Id, 10000, "refund_test")
    
    router := setupTestRouterWithServiceAuth()
    body := `{
        "deduction_id": "` + deductionId + `",
        "idempotency_key": "` + strconv.Itoa(user.Id) + `_task_refund_test_refund",
        "reason": "upstream_error"
    }`
    req := httptest.NewRequest("POST", "/internal/v1/wallet/refund", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+testServiceToken)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
    var resp map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &resp)
    assert.True(t, resp["refunded"].(bool))
    assert.Equal(t, float64(10000), resp["amount"])
    assert.Equal(t, float64(2000000), resp["balance_after"])
    
    // Verify used_quota back to 0
    user, _ = model.GetUserById(user.Id, false)
    assert.Equal(t, int64(0), user.UsedQuota)
}

func TestRefundIdempotency(t *testing.T) {
    user := createTestUserWithQuota(2000000)
    deductionId := preDeductForTest(user.Id, 10000, "refund_idem")
    idempotencyKey := strconv.Itoa(user.Id) + "_task_refund_idem_refund"
    
    body := `{
        "deduction_id": "` + deductionId + `",
        "idempotency_key": "` + idempotencyKey + `",
        "reason": "test"
    }`
    
    router := setupTestRouterWithServiceAuth()
    
    // First refund
    req1 := httptest.NewRequest("POST", "/internal/v1/wallet/refund", strings.NewReader(body))
    req1.Header.Set("Content-Type", "application/json")
    req1.Header.Set("Authorization", "Bearer "+testServiceToken)
    w1 := httptest.NewRecorder()
    router.ServeHTTP(w1, req1)
    assert.Equal(t, 200, w1.Code)
    
    // Second refund
    req2 := httptest.NewRequest("POST", "/internal/v1/wallet/refund", strings.NewReader(body))
    req2.Header.Set("Content-Type", "application/json")
    req2.Header.Set("Authorization", "Bearer "+testServiceToken)
    w2 := httptest.NewRecorder()
    router.ServeHTTP(w2, req2)
    assert.Equal(t, 200, w2.Code)
    
    // Verify refunded only once
    user, _ = model.GetUserById(user.Id, false)
    assert.Equal(t, int64(0), user.UsedQuota)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controller -run TestRefund -v`
Expected: FAIL with "404 page not found"

- [ ] **Step 3: Implement RefundCredits handler in `controller/internal_wallet.go`**

```go
type RefundRequest struct {
    DeductionId    string `json:"deduction_id" binding:"required"`
    IdempotencyKey string `json:"idempotency_key" binding:"required"`
    Reason         string `json:"reason"`
}

type RefundResponse struct {
    Refunded     bool  `json:"refunded"`
    Amount       int64 `json:"amount"`
    BalanceAfter int64 `json:"balance_after"`
}

func RefundCredits(c *gin.Context) {
    var req RefundRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
        return
    }
    
    // Check idempotency
    existing, _ := model.GetTransactionByIdempotencyKey(req.IdempotencyKey)
    if existing != nil {
        c.JSON(http.StatusOK, RefundResponse{Refunded: true, Amount: existing.Amount})
        return
    }
    
    // Get pre-deduct transaction
    var preDeduct model.WalletTransaction
    if err := model.DB.Where("id = ? AND type = ?", req.DeductionId, "pre_deduct").First(&preDeduct).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "deduction_not_found"})
        return
    }
    
    var balanceAfter int64
    
    err := model.DB.Transaction(func(tx *gorm.DB) error {
        var user model.User
        if err := tx.Where("id = ?", preDeduct.UserId).First(&user).Error; err != nil {
            return err
        }
        
        // Refund full amount
        if err := tx.Model(&user).Update("used_quota", gorm.Expr("used_quota - ?", preDeduct.Amount)).Error; err != nil {
            return err
        }
        
        balanceAfter = user.Quota - user.UsedQuota + preDeduct.Amount
        
        // Record refund transaction
        refundTx := &model.WalletTransaction{
            Id:             "refund_" + generateRandomString(16),
            UserId:         preDeduct.UserId,
            Type:           "refund",
            Amount:         preDeduct.Amount,
            IdempotencyKey: req.IdempotencyKey,
            DeductionId:    req.DeductionId,
            Metadata:       `{"reason":"` + req.Reason + `"}`,
        }
        return tx.Create(refundTx).Error
    })
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, RefundResponse{
        Refunded:     true,
        Amount:       preDeduct.Amount,
        BalanceAfter: balanceAfter,
    })
}
```

- [ ] **Step 4: Register route in `router/main.go`**

Add to internal group: `internal.POST("/wallet/refund", controller.RefundCredits)`

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controller -run TestRefund -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add controller/internal_wallet.go controller/internal_wallet_test.go router/main.go
git commit -m "feat(wallet): add refund credits endpoint"
```

---

## Part 4: New API - Frontend UI Changes

### Task 10: Gray Out Service Token Edit/Delete Buttons

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\new-api\web\src\pages\Token\index.jsx` (or equivalent token management page)
- Test: Manual UI testing

**Interfaces:**
- Consumes: Token data with `service_name` field from backend API
- Produces: Disabled edit/delete buttons when `service_name` is not null

- [ ] **Step 1: Identify token list rendering component**

Run: `cd E:\ZSXQ\API_Canvas\apps\new-api\web && grep -r "token.*list\|API.*Key" src/pages/`

- [ ] **Step 2: Add conditional rendering for service tokens**

In the token list table rendering:
```jsx
{tokens.map(token => (
  <TableRow key={token.id}>
    <TableCell>{token.name}</TableCell>
    <TableCell>{token.key_prefix}***</TableCell>
    <TableCell>
      {token.service_name ? (
        <>
          <Button disabled variant="outlined">编辑</Button>
          <Button disabled variant="outlined">删除</Button>
          <Chip label={`服务专用: ${token.service_name}`} size="small" />
        </>
      ) : (
        <>
          <Button onClick={() => handleEdit(token.id)}>编辑</Button>
          <Button onClick={() => handleDelete(token.id)} color="error">删除</Button>
        </>
      )}
    </TableCell>
  </TableRow>
))}
```

- [ ] **Step 3: Test locally**

Run: `npm run dev` and navigate to Token management page. Verify:
- User-created tokens show enabled edit/delete buttons
- Service tokens show disabled buttons with label

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Token/index.jsx
git commit -m "feat(ui): disable edit/delete for service tokens"
```

---

## Part 5: TapCanvas - Database Schema

### Task 11: Create sessions Table in TapCanvas

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\prisma\migrations\YYYYMMDD_create_sessions\migration.sql`
- Modify: `E:\ZSXQ\API_Canvas\apps\tapcanvas\prisma\schema.prisma`
- Test: `E:\ZSXQ\API_Canvas\apps\tapcanvas\prisma\migrations\test.ts`

**Interfaces:**
- Consumes: Prisma schema conventions from existing tables
- Produces: `sessions` table with `id`, `user_id`, `token`, `expires_at` columns

- [ ] **Step 1: Add sessions model to `schema.prisma`**

```prisma
model sessions {
  id               String   @id
  user_id          String
  username         String?
  email            String?
  avatar_url       String?
  token            String   @unique(map: "idx_sessions_token")
  expires_at       String
  created_at       String
  last_accessed_at String
  
  @@index([user_id], map: "idx_sessions_user_id")
  @@index([expires_at], map: "idx_sessions_expires_at")
}
```

- [ ] **Step 2: Generate migration**

Run: `cd E:\ZSXQ\API_Canvas\apps\tapcanvas && npx prisma migrate dev --name create_sessions`

- [ ] **Step 3: Verify migration creates table**

Run: `npx prisma migrate status`
Expected: Migration applied successfully

- [ ] **Step 4: Commit**

```bash
git add prisma/schema.prisma prisma/migrations/
git commit -m "feat(db): add sessions table for SSO"
```

---

## Part 6: TapCanvas - New API Client

### Task 12: New API Client Library

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\lib\new-api-client.ts`
- Test: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\lib\new-api-client.test.ts`

**Interfaces:**
- Consumes: `NEW_API_BASE_URL`, `NEW_API_SERVICE_TOKEN` from environment variables
- Produces: `NewAPIClient` class with methods: `exchangeCode()`, `getWallet()`, `preDeduct()`, `settle()`, `refund()`, `getModels()`

- [ ] **Step 1: Write failing test for client**

```typescript
// new-api-client.test.ts
import { describe, it, expect, beforeAll } from 'vitest'
import { NewAPIClient } from './new-api-client'

describe('NewAPIClient', () => {
  let client: NewAPIClient
  
  beforeAll(() => {
    client = new NewAPIClient('http://localhost:8080', 'sk-test-token')
  })
  
  it('should exchange auth code for user info', async () => {
    const result = await client.exchangeCode('test_code', 'http://localhost:5173/callback')
    expect(result).toHaveProperty('user_id')
    expect(result).toHaveProperty('username')
  })
  
  it('should get wallet balance', async () => {
    const wallet = await client.getWallet('123')
    expect(wallet).toHaveProperty('balance')
    expect(wallet.user_id).toBe('123')
  })
  
  it('should pre-deduct credits', async () => {
    const result = await client.preDeduct({
      user_id: '123',
      amount: 10000,
      idempotency_key: '123_task_test_prededuct'
    })
    expect(result).toHaveProperty('deduction_id')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd E:\ZSXQ\API_Canvas\apps\tapcanvas && npm test new-api-client.test.ts`
Expected: FAIL with "Cannot find module"

- [ ] **Step 3: Implement NewAPIClient in `src/lib/new-api-client.ts`**

```typescript
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
      const error = await response.json()
      throw new Error(error.error || error.message || 'API request failed')
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
```

- [ ] **Step 4: Run test to verify it passes (with mock server)**

Run: `npm test new-api-client.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lib/new-api-client.ts src/lib/new-api-client.test.ts
git commit -m "feat(client): add New API client library"
```

---

## Part 7: TapCanvas - SSO Implementation

### Task 13: SSO Middleware for Session Validation

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\middleware\session.ts`
- Test: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\middleware\session.test.ts`

**Interfaces:**
- Consumes: `sessions` table from Task 11, `canvas_session` cookie
- Produces: `sessionMiddleware()` that validates session and attaches user to context

- [ ] **Step 1: Write failing test for session middleware**

```typescript
// session.test.ts
import { describe, it, expect } from 'vitest'
import { Hono } from 'hono'
import { sessionMiddleware } from './session'

describe('sessionMiddleware', () => {
  it('should reject request without session cookie', async () => {
    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => c.json({ ok: true }))
    
    const res = await app.request('/test')
    expect(res.status).toBe(401)
  })
  
  it('should reject expired session', async () => {
    // Create expired session in test DB
    const expiredToken = 'expired_token_123'
    await createTestSession({
      token: expiredToken,
      expires_at: new Date(Date.now() - 1000).toISOString()
    })
    
    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => c.json({ ok: true }))
    
    const res = await app.request('/test', {
      headers: { Cookie: `canvas_session=${expiredToken}` }
    })
    expect(res.status).toBe(401)
  })
  
  it('should attach user to context for valid session', async () => {
    const validToken = 'valid_token_456'
    await createTestSession({
      user_id: '123',
      username: 'alice',
      token: validToken,
      expires_at: new Date(Date.now() + 86400000).toISOString()
    })
    
    const app = new Hono()
    app.use(sessionMiddleware())
    app.get('/test', (c) => {
      const user = c.get('user')
      return c.json({ user_id: user.user_id })
    })
    
    const res = await app.request('/test', {
      headers: { Cookie: `canvas_session=${validToken}` }
    })
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.user_id).toBe('123')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test session.test.ts`
Expected: FAIL with "undefined: sessionMiddleware"

- [ ] **Step 3: Implement sessionMiddleware in `src/middleware/session.ts`**

```typescript
import { Context, Next } from 'hono'
import { getCookie } from 'hono/cookie'
import { prisma } from '../lib/prisma'

export function sessionMiddleware() {
  return async (c: Context, next: Next) => {
    const sessionToken = getCookie(c, 'canvas_session')
    
    if (!sessionToken) {
      return c.json({ error: 'SESSION_EXPIRED', message: 'No session found' }, 401)
    }
    
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test session.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/middleware/session.ts src/middleware/session.test.ts
git commit -m "feat(auth): add session validation middleware"
```

---

### Task 14: OAuth Callback Handler

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\routes\oauth.ts`
- Test: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\routes\oauth.test.ts`

**Interfaces:**
- Consumes: `newApiClient.exchangeCode()` from Task 12
- Produces: `GET /oauth/callback` handler that creates session and sets cookie

- [ ] **Step 1: Write failing test for OAuth callback**

```typescript
// oauth.test.ts
import { describe, it, expect } from 'vitest'
import { Hono } from 'hono'
import { oauthRoutes } from './oauth'

describe('OAuth callback', () => {
  it('should exchange code and create session', async () => {
    const app = new Hono()
    app.route('/oauth', oauthRoutes)
    
    // Mock New API /sso/token response
    mockNewAPITokenExchange({
      user_id: '123',
      username: 'alice',
      email: 'alice@example.com'
    })
    
    const res = await app.request('/oauth/callback?code=test_code_xyz&state=abc')
    
    expect(res.status).toBe(302) // Redirect
    expect(res.headers.get('Location')).toBe('/')
    
    const cookies = res.headers.get('Set-Cookie')
    expect(cookies).toContain('canvas_session=')
    expect(cookies).toContain('HttpOnly')
    expect(cookies).toContain('Max-Age=2592000')
  })
  
  it('should reject invalid code', async () => {
    const app = new Hono()
    app.route('/oauth', oauthRoutes)
    
    mockNewAPITokenExchange({ error: 'invalid_code' }, 400)
    
    const res = await app.request('/oauth/callback?code=invalid_code')
    expect(res.status).toBe(400)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test oauth.test.ts`
Expected: FAIL with "undefined: oauthRoutes"

- [ ] **Step 3: Implement OAuth callback in `src/routes/oauth.ts`**

```typescript
import { Hono } from 'hono'
import { setCookie } from 'hono/cookie'
import { nanoid } from 'nanoid'
import { prisma } from '../lib/prisma'
import { newApiClient } from '../lib/new-api-client'

export const oauthRoutes = new Hono()

oauthRoutes.get('/callback', async (c) => {
  const code = c.req.query('code')
  const state = c.req.query('state')
  
  if (!code) {
    return c.json({ error: 'missing_code' }, 400)
  }
  
  // TODO: Verify state against stored value (CSRF protection)
  
  try {
    // Exchange code for user info
    const redirectUri = process.env.CANVAS_REDIRECT_URI || 'http://localhost:5173/oauth/callback'
    const userInfo = await newApiClient.exchangeCode(code, redirectUri)
    
    // Create session
    const sessionToken = nanoid(64)
    const expiresAt = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000) // 30 days
    
    await prisma.sessions.create({
      data: {
        id: nanoid(),
        user_id: userInfo.user_id,
        username: userInfo.username,
        email: userInfo.email,
        avatar_url: userInfo.avatar_url,
        token: sessionToken,
        expires_at: expiresAt.toISOString(),
        created_at: new Date().toISOString(),
        last_accessed_at: new Date().toISOString()
      }
    })
    
    // Set session cookie
    setCookie(c, 'canvas_session', sessionToken, {
      maxAge: 2592000, // 30 days
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'Lax',
      path: '/'
    })
    
    // Redirect to home
    return c.redirect('/')
  } catch (error) {
    console.error('OAuth callback error:', error)
    return c.json({ error: 'exchange_failed', message: error.message }, 400)
  }
})
```

- [ ] **Step 4: Register routes in main app**

In `src/index.ts`: `app.route('/oauth', oauthRoutes)`

- [ ] **Step 5: Run test to verify it passes**

Run: `npm test oauth.test.ts`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add src/routes/oauth.ts src/routes/oauth.test.ts src/index.ts
git commit -m "feat(auth): add OAuth callback handler"
```

---

## Part 8: TapCanvas - Wallet Integration

### Task 15: Wallet Balance Display Component

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\web\src\components\WalletBalance.tsx`
- Modify: `E:\ZSXQ\API_Canvas\apps\tapcanvas\web\src\layouts\MainLayout.tsx`
- Test: Manual testing

**Interfaces:**
- Consumes: `/internal/v1/users/:id/wallet` via newApiClient
- Produces: React component displaying user balance

- [ ] **Step 1: Create WalletBalance component**

```typescript
// WalletBalance.tsx
import { useEffect, useState } from 'react'
import { newApiClient } from '../lib/api-client'
import { useUser } from '../hooks/useUser'

export function WalletBalance() {
  const { user } = useUser()
  const [balance, setBalance] = useState<string>('--')
  const [loading, setLoading] = useState(true)
  
  useEffect(() => {
    if (!user?.user_id) return
    
    const fetchBalance = async () => {
      try {
        const wallet = await newApiClient.getWallet(user.user_id)
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
  }, [user?.user_id])
  
  if (loading) {
    return <div className="text-sm text-gray-500">Loading...</div>
  }
  
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-gray-600">余额:</span>
      <span className="text-sm font-medium">{balance}</span>
      <a href={`${process.env.NEW_API_BASE_URL}/wallet`} className="text-xs text-blue-500 hover:underline">
        充值
      </a>
    </div>
  )
}
```

- [ ] **Step 2: Add WalletBalance to MainLayout header**

```typescript
// MainLayout.tsx
import { WalletBalance } from '../components/WalletBalance'

// In header section:
<header>
  {/* ... existing nav items ... */}
  <WalletBalance />
</header>
```

- [ ] **Step 3: Test locally**

Run: `npm run dev`, login, verify balance displays in header

- [ ] **Step 4: Commit**

```bash
git add src/components/WalletBalance.tsx src/layouts/MainLayout.tsx
git commit -m "feat(ui): add wallet balance display"
```

---

### Task 16: AI Generation with Pre-Deduct/Settle Flow

**Files:**
- Modify: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\services\image-generation.ts`
- Test: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\services\image-generation.test.ts`

**Interfaces:**
- Consumes: `newApiClient.preDeduct()`, `newApiClient.settle()`, `newApiClient.refund()` from Task 12
- Produces: `generateImage()` function with pre-deduct → AI call → settle/refund flow

- [ ] **Step 1: Write failing test for generation flow**

```typescript
// image-generation.test.ts
import { describe, it, expect } from 'vitest'
import { generateImage } from './image-generation'

describe('generateImage', () => {
  it('should pre-deduct, generate, and settle', async () => {
    const result = await generateImage({
      user_id: '123',
      task_id: 'task_abc',
      model: 'flux-pro',
      prompt: 'a cat',
      estimated_cost: 10000
    })
    
    expect(result).toHaveProperty('image_url')
    expect(result).toHaveProperty('actual_cost')
    
    // Verify transactions
    const wallet = await newApiClient.getWallet('123')
    // Balance should be deducted by actual_cost, not estimated
  })
  
  it('should refund on generation failure', async () => {
    mockNewAPIGenerationFailure()
    
    const initialWallet = await newApiClient.getWallet('123')
    
    await expect(generateImage({
      user_id: '123',
      task_id: 'task_fail',
      model: 'flux-pro',
      prompt: 'invalid',
      estimated_cost: 10000
    })).rejects.toThrow()
    
    const finalWallet = await newApiClient.getWallet('123')
    expect(finalWallet.balance).toBe(initialWallet.balance) // Refunded
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test image-generation.test.ts`
Expected: FAIL

- [ ] **Step 3: Implement generateImage with credit flow**

```typescript
// image-generation.ts
import { newApiClient } from '../lib/new-api-client'
import { nanoid } from 'nanoid'

interface GenerateImageParams {
  user_id: string
  task_id: string
  model: string
  prompt: string
  estimated_cost: number
}

export async function generateImage(params: GenerateImageParams) {
  const { user_id, task_id, model, prompt, estimated_cost } = params
  
  // Step 1: Pre-deduct
  const preDeductKey = `${user_id}_${task_id}_prededuct`
  let deductionId: string
  
  try {
    const preDeductResult = await newApiClient.preDeduct({
      user_id,
      amount: estimated_cost,
      idempotency_key: preDeductKey,
      metadata: { task_id, model, prompt }
    })
    deductionId = preDeductResult.deduction_id
  } catch (error) {
    if (error.message.includes('INSUFFICIENT_QUOTA')) {
      throw new Error('余额不足，请前往充值')
    }
    throw error
  }
  
  // Step 2: Call AI generation via New API
  let imageUrl: string
  let actualCost: number
  
  try {
    const response = await fetch(`${process.env.NEW_API_BASE_URL}/v1/images/generations`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${process.env.NEW_API_SERVICE_TOKEN}`,
        'X-Request-ID': `canvas_${Date.now()}_${nanoid(8)}`,
        'X-Canvas-Task-Id': task_id
      },
      body: JSON.stringify({
        model,
        prompt,
        size: '1024x1024',
        n: 1
      })
    })
    
    if (!response.ok) {
      throw new Error(`Generation failed: ${response.statusText}`)
    }
    
    const data = await response.json()
    imageUrl = data.data[0].url
    actualCost = data.usage?.total_cost || estimated_cost
    
  } catch (error) {
    // Step 3a: Refund on failure
    const refundKey = `${user_id}_${task_id}_refund`
    await newApiClient.refund({
      deduction_id: deductionId,
      idempotency_key: refundKey,
      reason: 'generation_failed'
    })
    throw error
  }
  
  // Step 3b: Settle with actual cost
  const settleKey = `${user_id}_${task_id}_settle`
  await newApiClient.settle({
    deduction_id: deductionId,
    actual_cost: actualCost,
    idempotency_key: settleKey
  })
  
  return { image_url: imageUrl, actual_cost: actualCost }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test image-generation.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/image-generation.ts src/services/image-generation.test.ts
git commit -m "feat(ai): add image generation with pre-deduct/settle flow"
```

---

## Part 9: Error Handling and Degradation

### Task 17: Error Mapper Utility

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\web\src\lib\error-mapper.ts`
- Test: `E:\ZSXQ\API_Canvas\apps\tapcanvas\web\src\lib\error-mapper.test.ts`

**Interfaces:**
- Consumes: API error codes from spec section 8.1
- Produces: `mapError(apiError) => UserFacingError` function

- [ ] **Step 1: Write failing test**

```typescript
// error-mapper.test.ts
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test error-mapper.test.ts`
Expected: FAIL

- [ ] **Step 3: Implement error mapper**

```typescript
// error-mapper.ts
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test error-mapper.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lib/error-mapper.ts src/lib/error-mapper.test.ts
git commit -m "feat(error): add error code mapper"
```

---

### Task 18: Service Degradation Handler

**Files:**
- Create: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\middleware\health-check.ts`
- Modify: `E:\ZSXQ\API_Canvas\apps\tapcanvas\src\index.ts`
- Test: Manual testing

**Interfaces:**
- Consumes: `GET /health` from New API
- Produces: Health check middleware that sets degradation flag

- [ ] **Step 1: Implement health check middleware**

```typescript
// health-check.ts
import { Context } from 'hono'

let lastHealthCheck = Date.now()
let healthStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy'
let consecutiveFailures = 0

export async function checkNewAPIHealth() {
  try {
    const response = await fetch(`${process.env.NEW_API_BASE_URL}/health`, {
      timeout: 3000
    })
    
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
  return async (c: Context, next: any) => {
    // Refresh health status every 30 seconds
    if (Date.now() - lastHealthCheck > 30000) {
      checkNewAPIHealth() // Non-blocking
    }
    
    c.set('new_api_health', healthStatus)
    await next()
  }
}

// Start periodic health checks
setInterval(checkNewAPIHealth, 30000)
```

- [ ] **Step 2: Register middleware in main app**

```typescript
// index.ts
import { healthCheckMiddleware } from './middleware/health-check'

app.use('*', healthCheckMiddleware())
```

- [ ] **Step 3: Add health status banner to UI**

In main layout component:
```typescript
const healthStatus = useHealthStatus()

{healthStatus === 'unhealthy' && (
  <div className="bg-yellow-100 border-l-4 border-yellow-500 text-yellow-700 p-4">
    <p>AI 服务暂时不可用，部分功能受限</p>
  </div>
)}
```

- [ ] **Step 4: Commit**

```bash
git add src/middleware/health-check.ts src/index.ts
git commit -m "feat(ops): add health check and degradation detection"
```

---

## Part 10: Testing and Documentation

### Task 19: Integration Tests

**Files:**
- Create: `E:\ZSXQ\API_Canvas\tests\integration\sso-flow.test.ts`
- Create: `E:\ZSXQ\API_Canvas\tests\integration\wallet-flow.test.ts`

**Interfaces:**
- Consumes: Running New API and TapCanvas instances
- Produces: End-to-end integration tests

- [ ] **Step 1: Write SSO integration test**

```typescript
// sso-flow.test.ts
import { describe, it, expect } from 'vitest'
import puppeteer from 'puppeteer'

describe('SSO Integration', () => {
  it('should complete full SSO flow', async () => {
    const browser = await puppeteer.launch()
    const page = await browser.newPage()
    
    // 1. Visit TapCanvas
    await page.goto('http://localhost:5173')
    
    // 2. Should redirect to New API login
    await page.waitForNavigation()
    expect(page.url()).toContain('localhost:8080')
    expect(page.url()).toContain('/sso/authorize')
    
    // 3. Login
    await page.type('#username', 'testuser')
    await page.type('#password', 'testpass')
    await page.click('button[type="submit"]')
    
    // 4. Should redirect back to TapCanvas
    await page.waitForNavigation({ timeout: 5000 })
    expect(page.url()).toContain('localhost:5173')
    
    // 5. Verify session cookie set
    const cookies = await page.cookies()
    const sessionCookie = cookies.find(c => c.name === 'canvas_session')
    expect(sessionCookie).toBeDefined()
    expect(sessionCookie.httpOnly).toBe(true)
    
    await browser.close()
  })
})
```

- [ ] **Step 2: Write wallet integration test**

```typescript
// wallet-flow.test.ts
import { describe, it, expect } from 'vitest'

describe('Wallet Integration', () => {
  it('should show balance after login', async () => {
    const session = await loginTestUser()
    
    const response = await fetch('http://localhost:8788/api/wallet', {
      headers: { Cookie: `canvas_session=${session.token}` }
    })
    
    expect(response.status).toBe(200)
    const data = await response.json()
    expect(data).toHaveProperty('balance')
    expect(data).toHaveProperty('display_balance')
  })
  
  it('should deduct credits on AI generation', async () => {
    const session = await loginTestUser()
    
    // Get initial balance
    const initialBalance = await getWalletBalance(session.token)
    
    // Generate image
    await fetch('http://localhost:8788/api/generate/image', {
      method: 'POST',
      headers: { 
        'Content-Type': 'application/json',
        'Cookie': `canvas_session=${session.token}`
      },
      body: JSON.stringify({ prompt: 'test', model: 'flux-pro' })
    })
    
    // Verify balance decreased
    const finalBalance = await getWalletBalance(session.token)
    expect(finalBalance).toBeLessThan(initialBalance)
  })
})
```

- [ ] **Step 3: Run integration tests**

Run: `npm run test:integration`

- [ ] **Step 4: Commit**

```bash
git add tests/integration/
git commit -m "test: add SSO and wallet integration tests"
```

---

### Task 20: API Documentation

**Files:**
- Create: `E:\ZSXQ\API_Canvas\docs\API.md`

**Interfaces:**
- Consumes: Implemented endpoints from all tasks
- Produces: Complete API reference documentation

- [ ] **Step 1: Document all New API endpoints**

```markdown
# New API - Internal API Reference

## SSO Endpoints

### GET /sso/authorize
...

### POST /sso/token
...

## Wallet Endpoints

### GET /internal/v1/users/:id/wallet
...

### POST /internal/v1/wallet/pre-deduct
...

### POST /internal/v1/wallet/settle
...

### POST /internal/v1/wallet/refund
...
```

- [ ] **Step 2: Add code examples for each endpoint**

- [ ] **Step 3: Document error codes**

- [ ] **Step 4: Commit**

```bash
git add docs/API.md
git commit -m "docs: add API reference documentation"
```

---

## Part 11: Repository Setup

### Task 0: Create GitHub Repository and Initial Setup

**Files:**
- Modify: `.gitignore`, `README.md`

**Interfaces:**
- Consumes: GitHub account credentials
- Produces: Remote repository `api-canvas` on GitHub, initial project structure

- [ ] **Step 1: Create GitHub repository**

Run: `gh repo create api-canvas --public --description "New API + TapCanvas Integration: SSO, Wallet, AI Model Gateway"`

Or create manually at https://github.com/new with:
- Repository name: `api-canvas`
- Description: "New API + TapCanvas Integration: SSO, Wallet, AI Model Gateway"
- Visibility: Public (or Private based on your preference)
- Initialize: Do NOT initialize with README (we already have commits)

- [ ] **Step 2: Create .gitignore file**

```bash
# .gitignore
# Environment variables
.env
.env.local
.env.production

# Dependencies
node_modules/
vendor/

# Build outputs
dist/
build/
*.exe

# Database
*.db
*.sqlite
*.sqlite3

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Logs
*.log
logs/

# Test coverage
coverage/
*.coverprofile

# Temporary files
tmp/
temp/
```

- [ ] **Step 3: Create README.md**

```markdown
# API Canvas Integration

New API + TapCanvas 轻量级集成方案，实现 SSO 单点登录、共享钱包、AI 模型网关。

## 项目结构

- `apps/new-api/` - AI 模型网关（Go）
- `apps/tapcanvas/` - Canvas 视频创作平台（TypeScript）
- `docs/` - 设计文档和实施计划
- `tests/` - 集成测试

## 快速开始

详见 [设计文档](docs/superpowers/specs/2026-10-03-new-api-tapcanvas-integration-design.md) 和 [实施计划](docs/superpowers/plans/2026-10-03-new-api-tapcanvas-integration.md)。

## 技术栈

- **New API**: Go 1.19+, Gin, GORM, PostgreSQL/MySQL/SQLite
- **TapCanvas**: Node.js 18+, TypeScript, Hono, Prisma, PostgreSQL

## License

MIT
```

- [ ] **Step 4: Add remote and push**

```bash
cd E:\ZSXQ\API_Canvas
git add .gitignore README.md
git commit -m "chore: add .gitignore and README"
git remote add origin https://github.com/YOUR_USERNAME/api-canvas.git
git branch -M main
git push -u origin main
```

- [ ] **Step 5: Verify repository is accessible**

Visit: `https://github.com/YOUR_USERNAME/api-canvas`

---

## Final Verification

### Task 21: Acceptance Testing Checklist

**Files:**
- Create: `E:\ZSXQ\API_Canvas\ACCEPTANCE.md`

- [ ] **Step 1: Verify all P0 requirements from spec section 12**

- SSO login works end-to-end
- Session persists for 30 days
- Balance query shows correct amount
- Insufficient balance blocks generation
- Model list syncs from New API
- AI generation succeeds and deducts credits
- Failed generation refunds credits
- Idempotency prevents double-charging

- [ ] **Step 2: Verify performance targets**

- SSO login < 2s
- Balance query < 500ms
- Model list query < 1s

- [ ] **Step 3: Verify security requirements**

- Service Token not in logs/frontend
- HTTPS enforced in production
- Cookies have HttpOnly, Secure flags
- CSRF protection via state parameter

- [ ] **Step 4: Document verification results**

- [ ] **Step 5: Commit**

```bash
git add ACCEPTANCE.md
git commit -m "docs: add acceptance testing checklist"
```

---

Plan complete and saved to `E:\ZSXQ\API_Canvas\docs\superpowers\plans\2026-10-03-new-api-tapcanvas-integration.md`. Please review the plan. Which execution approach would you prefer?

- **Subagent-driven** - A fresh subagent implements each task and a fresh reviewer checks it before the next one starts, then a whole-branch review at the end. Most thorough; costs a fresh context per task and per review.
- **Native** - I implement every task myself in this session, the way this harness runs work, then one fresh reviewer on the most capable model checks the whole branch. Cheapest and fastest; no independent review until the end.

**For this plan I recommend Subagent-driven, because the tasks span two independent codebases (New API in Go, TapCanvas in TypeScript), involve database migrations that must execute in sequence, and a mistake in the wallet credit flow could cause financial loss. Independent review per task catches integration errors early.** 

**Note: Task 0 (GitHub repository setup) should be completed first before starting the main implementation tasks.**

Does the plan capture what you want, and which approach should we use?