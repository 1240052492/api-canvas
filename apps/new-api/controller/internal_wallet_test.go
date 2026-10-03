package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var testServiceToken = "sk-test-service-token-prededuct"

func setupTestRouterWithServiceAuth() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ServiceTokenAuth())
	router.GET("/internal/v1/users/:id/wallet", GetWalletBalance)
	router.POST("/internal/v1/wallet/pre-deduct", PreDeductCredits)
	router.POST("/internal/v1/wallet/settle", SettleCredits)
	return router
}

func preDeductForTest(userId int, amount int64, taskId string) string {
	idempotencyKey := strconv.Itoa(userId) + "_task_" + taskId + "_prededuct"

	router := setupTestRouterWithServiceAuth()
	body := `{
        "user_id": ` + strconv.Itoa(userId) + `,
        "amount": ` + strconv.FormatInt(amount, 10) + `,
        "idempotency_key": "` + idempotencyKey + `",
        "metadata": {"task_id": "` + taskId + `"}
    }`
	req := httptest.NewRequest("POST", "/internal/v1/wallet/pre-deduct", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testServiceToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["deduction_id"].(string)
}

func setupInternalWalletTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	// Migrate models
	if err := db.AutoMigrate(
		&model.User{},
		&model.WalletTransaction{},
		&model.Token{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func createTestServiceToken(serviceName string) (*model.Token, error) {
	token := &model.Token{
		Key:            testServiceToken,
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	if err := model.DB.Create(token).Error; err != nil {
		return nil, err
	}
	return token, nil
}

func createTestUserWithQuota(quota int) *model.User {
	user := &model.User{
		Username: "test_user_" + strconv.Itoa(int(common.GetTimestamp())),
		Password: "testpass123",
		Email:    "test_" + strconv.Itoa(int(common.GetTimestamp())) + "@example.com",
		Quota:    quota,
	}
	if err := model.DB.Create(user).Error; err != nil {
		panic(err)
	}
	return user
}

func TestGetWalletBalance(t *testing.T) {
	// Setup: create service token for authentication
	serviceName := "tapcanvas"
	testServiceToken := "sk-test-service-token-wallet"
	token := &model.Token{
		Key:            testServiceToken,
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	model.DB.Create(token)
	defer model.DB.Delete(token)

	user := &model.User{
		Username:  "alice",
		Password:  "hashedpassword",
		Role:      1,
		Status:    1,
		Quota:     2000000, // $2.00
		UsedQuota: 500000,  // $0.50
	}
	model.DB.Create(user)
	defer model.DB.Delete(user)

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
	// Setup: create service token for authentication
	serviceName := "tapcanvas"
	testServiceToken := "sk-test-service-token-wallet-404"
	token := &model.Token{
		Key:            testServiceToken,
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	model.DB.Create(token)
	defer model.DB.Delete(token)

	router := setupTestRouterWithServiceAuth()
	req := httptest.NewRequest("GET", "/internal/v1/users/99999/wallet", nil)
	req.Header.Set("Authorization", "Bearer "+testServiceToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)
}

func TestPreDeductSuccess(t *testing.T) {
	setupInternalWalletTestDB(t)
	_, err := createTestServiceToken("tapcanvas")
	if err != nil {
		t.Fatalf("failed to create service token: %v", err)
	}

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
	updatedUser, _ := model.GetUserById(user.Id, false)
	assert.Equal(t, int64(10000), int64(updatedUser.UsedQuota))
}

func TestPreDeductIdempotency(t *testing.T) {
	setupInternalWalletTestDB(t)
	_, err := createTestServiceToken("tapcanvas")
	if err != nil {
		t.Fatalf("failed to create service token: %v", err)
	}

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
	balanceAfter1 := resp1["balance_after"]

	// Second request with same idempotency key
	req2 := httptest.NewRequest("POST", "/internal/v1/wallet/pre-deduct", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+testServiceToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	// Should return identical response (strict idempotency)
	assert.Equal(t, deductionId1, resp2["deduction_id"])
	assert.Equal(t, balanceAfter1, resp2["balance_after"]) // Byte-identical response

	updatedUser, _ := model.GetUserById(user.Id, false)
	assert.Equal(t, int64(10000), int64(updatedUser.UsedQuota)) // Only deducted once
}

func TestPreDeductInsufficientBalance(t *testing.T) {
	setupInternalWalletTestDB(t)
	_, err := createTestServiceToken("tapcanvas")
	if err != nil {
		t.Fatalf("failed to create service token: %v", err)
	}

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

func TestSettleCredits(t *testing.T) {
	setupInternalWalletTestDB(t)
	_, err := createTestServiceToken("tapcanvas")
	if err != nil {
		t.Fatalf("failed to create service token: %v", err)
	}

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
	assert.Equal(t, int64(8500), int64(user.UsedQuota))
}

func TestSettleIdempotency(t *testing.T) {
	setupInternalWalletTestDB(t)
	_, err := createTestServiceToken("tapcanvas")
	if err != nil {
		t.Fatalf("failed to create service token: %v", err)
	}

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
	assert.Equal(t, int64(8500), int64(user.UsedQuota))
}
