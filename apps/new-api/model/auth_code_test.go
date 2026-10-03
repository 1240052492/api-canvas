package model

import (
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestGenerateAuthCodeFormat(t *testing.T) {
	// Ensure we're in master node mode for migrations
	common.IsMasterNode = true

	// Remove old test database if exists
	os.Remove("one-api.db")

	// Setup test database
	err := InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Generate an auth code
	authCode, err := GenerateAuthCode(1, "http://localhost:5173/oauth/callback", "test-state")
	assert.NoError(t, err)
	assert.NotNil(t, authCode)

	// Verify code format: 24 bytes encoded to base64url = 32 characters
	assert.Equal(t, 32, len(authCode.Code), "Auth code should be exactly 32 characters")

	// Verify it's valid base64url
	decoded, err := base64.URLEncoding.DecodeString(authCode.Code)
	assert.NoError(t, err, "Auth code should be valid base64url")
	assert.Equal(t, 24, len(decoded), "Decoded auth code should be exactly 24 bytes")

	// Verify other fields
	assert.Equal(t, 1, authCode.UserId)
	assert.Equal(t, "http://localhost:5173/oauth/callback", authCode.RedirectUri)
	assert.Equal(t, "test-state", authCode.State)
	assert.False(t, authCode.Used)

	// Verify expiry is 5 minutes from now
	expectedExpiry := time.Now().Add(5 * time.Minute)
	assert.WithinDuration(t, expectedExpiry, authCode.ExpiresAt, 2*time.Second)
}

func TestConsumeAuthCode(t *testing.T) {
	common.IsMasterNode = true
	os.Remove("one-api.db")

	err := InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Generate an auth code
	authCode, err := GenerateAuthCode(1, "http://localhost:5173/oauth/callback", "test-state")
	assert.NoError(t, err)

	// Consume it
	consumed, err := ConsumeAuthCode(authCode.Code, "http://localhost:5173/oauth/callback")
	assert.NoError(t, err)
	assert.Equal(t, authCode.Code, consumed.Code)
	assert.True(t, consumed.Used)

	// Try to consume again - should fail
	_, err = ConsumeAuthCode(authCode.Code, "http://localhost:5173/oauth/callback")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already used")
}

func TestConsumeAuthCodeWrongRedirectUri(t *testing.T) {
	common.IsMasterNode = true
	os.Remove("one-api.db")

	err := InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Generate an auth code
	authCode, err := GenerateAuthCode(1, "http://localhost:5173/oauth/callback", "test-state")
	assert.NoError(t, err)

	// Try to consume with wrong redirect_uri
	_, err = ConsumeAuthCode(authCode.Code, "http://evil.com/callback")
	assert.Error(t, err)
}

func TestConsumeAuthCodeExpired(t *testing.T) {
	common.IsMasterNode = true
	os.Remove("one-api.db")

	err := InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Generate an auth code
	authCode, err := GenerateAuthCode(1, "http://localhost:5173/oauth/callback", "test-state")
	assert.NoError(t, err)

	// Manually expire it
	authCode.ExpiresAt = time.Now().Add(-1 * time.Minute)
	DB.Save(authCode)

	// Try to consume - should fail
	_, err = ConsumeAuthCode(authCode.Code, "http://localhost:5173/oauth/callback")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestCleanupExpiredAuthCodes(t *testing.T) {
	common.IsMasterNode = true
	os.Remove("one-api.db")

	err := InitDB()
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// Generate two auth codes
	authCode1, _ := GenerateAuthCode(1, "http://localhost:5173/oauth/callback", "state1")
	authCode2, _ := GenerateAuthCode(2, "http://localhost:5173/oauth/callback", "state2")

	// Expire one of them
	authCode1.ExpiresAt = time.Now().Add(-1 * time.Minute)
	DB.Save(authCode1)

	// Cleanup
	err = CleanupExpiredAuthCodes()
	assert.NoError(t, err)

	// Verify expired code is gone
	var count int64
	DB.Model(&AuthCode{}).Where("code = ?", authCode1.Code).Count(&count)
	assert.Equal(t, int64(0), count)

	// Verify non-expired code still exists
	DB.Model(&AuthCode{}).Where("code = ?", authCode2.Code).Count(&count)
	assert.Equal(t, int64(1), count)
}
