package middleware

import (
	"bytes"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	model.DB = db
	model.LOG_DB = db

	common.UsingSQLite = true
	common.RedisEnabled = false

	// Initialize column names for queries
	model.InitColumnNames()

	// Auto-migrate Token table
	if err := db.AutoMigrate(&model.Token{}); err != nil {
		panic("failed to migrate Token: " + err.Error())
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestServiceTokenAuthSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup: create service token
	serviceName := "tapcanvas"
	token := &model.Token{
		Key:            "sk-canvas-service-test123",
		UserId:         1,
		Status:         1,
		ServiceName:    &serviceName,
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	result := model.DB.Create(token)
	if result.Error != nil {
		t.Fatalf("failed to create token: %v", result.Error)
	}
	defer model.DB.Delete(token)

	router := gin.New()
	router.Use(ServiceTokenAuth())
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
	gin.SetMode(gin.TestMode)

	token := &model.Token{
		Key:            "sk-user-key-abc",
		UserId:         1,
		Status:         1,
		ServiceName:    nil, // User token, not service token
		UnlimitedQuota: true,
		ExpiredTime:    -1,
	}
	model.DB.Create(token)
	defer model.DB.Delete(token)

	router := gin.New()
	router.Use(ServiceTokenAuth())
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
	gin.SetMode(gin.TestMode)

	// Capture stderr where logger writes
	oldStderr := gin.DefaultErrorWriter
	var logBuf bytes.Buffer
	gin.DefaultErrorWriter = &logBuf
	defer func() {
		gin.DefaultErrorWriter = oldStderr
	}()

	router := gin.New()
	router.Use(ServiceTokenAuth())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Use a token that doesn't exist to trigger error logging
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer sk-canvas-service-secret123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	logOutput := logBuf.String()
	// Verify the actual token is NOT in the logs
	assert.NotContains(t, logOutput, "sk-canvas-service-secret123")
	// Verify error was logged
	assert.Contains(t, logOutput, "invalid service token")
}
