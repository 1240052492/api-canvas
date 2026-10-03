package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database: " + err.Error())
	}

	model.DB = db
	common.UsingSQLite = true
	common.RedisEnabled = false

	// Initialize column variables
	model.InitColumnNames()

	// Run migrations
	if err := db.AutoMigrate(&model.User{}, &model.Token{}); err != nil {
		panic("failed to migrate database: " + err.Error())
	}

	os.Exit(m.Run())
}

func TestQueryUserBalanceSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup: create user
	user := &model.User{
		Username:  "testuser",
		Password:  "password",
		Role:      1,
		Status:    1,
		Quota:     500000,
		UsedQuota: 100000,
	}
	result := model.DB.Create(user)
	if result.Error != nil {
		t.Fatalf("failed to create user: %v", result.Error)
	}
	defer model.DB.Delete(user)

	// Setup: create service token
	serviceName := "tapcanvas"
	token := &model.Token{
		Key:            "sk-canvas-balance-test",
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	model.DB.Create(token)
	defer model.DB.Delete(token)

	router := gin.New()
	router.Use(middleware.ServiceTokenAuth())
	router.POST("/service/balance", QueryUserBalance)

	reqBody := BalanceQueryRequest{UserID: user.Id}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/service/balance", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-canvas-balance-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp BalanceQueryResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 500000, resp.Balance)
	assert.Equal(t, 100000, resp.UsedQuota)
	assert.NotEmpty(t, resp.DisplayBalance)
}

func TestQueryUserBalanceUserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup: create service token
	serviceName := "tapcanvas"
	token := &model.Token{
		Key:            "sk-canvas-balance-test2",
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	model.DB.Create(token)
	defer model.DB.Delete(token)

	router := gin.New()
	router.Use(middleware.ServiceTokenAuth())
	router.POST("/service/balance", QueryUserBalance)

	reqBody := BalanceQueryRequest{UserID: 99999}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/service/balance", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-canvas-balance-test2")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "user_not_found")
}

func TestQueryUserBalanceInvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup: create service token
	serviceName := "tapcanvas"
	token := &model.Token{
		Key:            "sk-canvas-balance-test3",
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	model.DB.Create(token)
	defer model.DB.Delete(token)

	router := gin.New()
	router.Use(middleware.ServiceTokenAuth())
	router.POST("/service/balance", QueryUserBalance)

	// Missing user_id
	req := httptest.NewRequest("POST", "/service/balance", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer sk-canvas-balance-test3")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "invalid_request")
}
