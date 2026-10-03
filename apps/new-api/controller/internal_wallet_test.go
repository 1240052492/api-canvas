package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestRouterWithServiceAuth() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ServiceTokenAuth())
	router.GET("/internal/v1/users/:id/wallet", GetWalletBalance)
	return router
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
