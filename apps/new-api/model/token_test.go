package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenServiceNameField(t *testing.T) {
	truncateTables(t)

	serviceName := "tapcanvas"
	token := &Token{
		Key:         "sk-canvas-service-test123",
		UserId:      1,
		Status:      1,
		ServiceName: &serviceName,
	}
	err := DB.Create(token).Error
	assert.NoError(t, err)

	var loaded Token
	err = DB.First(&loaded, token.Id).Error
	assert.NoError(t, err)
	assert.NotNil(t, loaded.ServiceName)
	assert.Equal(t, "tapcanvas", *loaded.ServiceName)
}

func TestTokenServiceNameNullable(t *testing.T) {
	truncateTables(t)

	token := &Token{
		Key:    "sk-user-key-abc",
		UserId: 1,
		Status: 1,
		// ServiceName omitted - should be NULL
	}
	err := DB.Create(token).Error
	assert.NoError(t, err)

	var loaded Token
	err = DB.First(&loaded, token.Id).Error
	assert.NoError(t, err)
	assert.Nil(t, loaded.ServiceName)
}
