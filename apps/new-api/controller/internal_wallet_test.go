package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var testServiceToken = "test-internal-token-12345"

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

func setupTestRouterWithServiceAuth() *gin.Engine {
	router := gin.New()
	os.Setenv("TAPCANVAS_INTERNAL_TOKEN", testServiceToken)

	// Set up the internal v1 group with service token auth
	internal := router.Group("/internal/v1")
	internal.POST("/wallet/pre-deduct", PreDeductCredits)

	return router
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

func TestPreDeductSuccess(t *testing.T) {
	setupInternalWalletTestDB(t)
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

	updatedUser, _ := model.GetUserById(user.Id, false)
	assert.Equal(t, int64(10000), int64(updatedUser.UsedQuota)) // Only deducted once
}

func TestPreDeductInsufficientBalance(t *testing.T) {
	setupInternalWalletTestDB(t)
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
