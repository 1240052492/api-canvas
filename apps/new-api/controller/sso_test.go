package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	// Session middleware
	store := cookie.NewStore([]byte("test-secret"))
	router.Use(sessions.Sessions("session", store))

	// Mock user auth middleware that checks session
	router.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		if userId := session.Get("id"); userId != nil {
			c.Set("user_id", userId)
		}
		c.Next()
	})

	router.GET("/sso/authorize", SSOAuthorize)
	router.GET("/login", func(c *gin.Context) {
		c.String(http.StatusOK, "login page")
	})

	return router
}

func TestSSOAuthorizeRedirectsToLogin(t *testing.T) {
	os.Setenv("CANVAS_REDIRECT_URI_WHITELIST", "http://localhost:5173/oauth/callback")
	defer os.Unsetenv("CANVAS_REDIRECT_URI_WHITELIST")

	router := setupTestRouter()
	req := httptest.NewRequest("GET", "/sso/authorize?redirect_uri=http://localhost:5173/oauth/callback&state=xyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "/login")
}

func TestSSOAuthorizeGeneratesCode(t *testing.T) {
	// Ensure we're in master node mode for migrations
	common.IsMasterNode = true

	// Remove old test database if exists
	os.Remove("one-api.db")

	// Setup test database
	err := model.InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	os.Setenv("CANVAS_REDIRECT_URI_WHITELIST", "http://localhost:5173/oauth/callback")
	defer os.Unsetenv("CANVAS_REDIRECT_URI_WHITELIST")

	router := gin.Default()
	store := cookie.NewStore([]byte("test-secret"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 1)
		session.Save()
		c.String(200, "ok")
	})
	router.GET("/sso/authorize", func(c *gin.Context) {
		session := sessions.Default(c)
		if userId := session.Get("id"); userId != nil {
			c.Set("user_id", userId)
		}
		SSOAuthorize(c)
	})

	// First request to set up session
	setupReq := httptest.NewRequest("GET", "/setup", nil)
	setupW := httptest.NewRecorder()
	router.ServeHTTP(setupW, setupReq)

	// Extract cookies
	cookies := setupW.Result().Cookies()

	// Second request with session cookie
	req := httptest.NewRequest("GET", "/sso/authorize?redirect_uri=http://localhost:5173/oauth/callback&state=xyz", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "http://localhost:5173/oauth/callback?code=")
	assert.Contains(t, location, "&state=xyz")
}

func TestSSOAuthorizeRejectsInvalidRedirectUri(t *testing.T) {
	os.Setenv("CANVAS_REDIRECT_URI_WHITELIST", "http://localhost:5173/oauth/callback")
	defer os.Unsetenv("CANVAS_REDIRECT_URI_WHITELIST")

	router := setupTestRouter()
	req := httptest.NewRequest("GET", "/sso/authorize?redirect_uri=http://evil.com/callback", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSSOAuthorizeRejectsEmptyRedirectUri(t *testing.T) {
	router := setupTestRouter()
	req := httptest.NewRequest("GET", "/sso/authorize", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSSOTokenExchangeSuccess(t *testing.T) {
	// Ensure we're in master node mode for migrations
	common.IsMasterNode = true

	// Remove old test database if exists
	os.Remove("one-api.db")

	// Setup test database
	err := model.InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Disable Redis for testing (avoid nil pointer when user cache tries to update)
	common.RedisEnabled = false

	// Create test user
	testUser := &model.User{
		Username: "testuser",
		Password: "hashedpass",
		Email:    "test@example.com",
	}
	err = testUser.Insert(0)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Setup: create auth code
	authCode, _ := model.GenerateAuthCode(testUser.Id, "http://localhost:5173/oauth/callback", "xyz")

	router := gin.Default()
	router.POST("/sso/token", SSOTokenExchange)

	body := `{"code":"` + authCode.Code + `","redirect_uri":"http://localhost:5173/oauth/callback"}`
	req := httptest.NewRequest("POST", "/sso/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	common.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, strconv.Itoa(testUser.Id), resp["user_id"])
	assert.Equal(t, "testuser", resp["username"])
	assert.Equal(t, "test@example.com", resp["email"])
	assert.Equal(t, float64(2592000), resp["expires_in"])
}

func TestSSOTokenExchangeRejectsUsedCode(t *testing.T) {
	// Ensure we're in master node mode for migrations
	common.IsMasterNode = true

	// Remove old test database if exists
	os.Remove("one-api.db")

	// Setup test database
	err := model.InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Disable Redis for testing (avoid nil pointer when user cache tries to update)
	common.RedisEnabled = false

	// Create test user
	testUser := &model.User{
		Username: "testuser2",
		Password: "hashedpass",
		Email:    "test2@example.com",
	}
	err = testUser.Insert(0)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	authCode, _ := model.GenerateAuthCode(testUser.Id, "http://localhost:5173/oauth/callback", "")
	model.ConsumeAuthCode(authCode.Code, "http://localhost:5173/oauth/callback")

	router := gin.Default()
	router.POST("/sso/token", SSOTokenExchange)

	body := `{"code":"` + authCode.Code + `","redirect_uri":"http://localhost:5173/oauth/callback"}`
	req := httptest.NewRequest("POST", "/sso/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
