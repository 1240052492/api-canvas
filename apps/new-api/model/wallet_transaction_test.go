package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateWalletTransaction(t *testing.T) {
	truncateTables(t)
	tx := &WalletTransaction{
		Id:             "tx_abc123",
		UserId:         1,
		Type:           "pre_deduct",
		Amount:         10000,
		IdempotencyKey: "1_task_abc_prededuct",
		Metadata:       `{"task_id":"task_abc"}`,
	}
	err := CreateWalletTransaction(tx)
	assert.NoError(t, err)
}

func TestIdempotencyKeyUnique(t *testing.T) {
	truncateTables(t)

	tx1 := &WalletTransaction{
		Id:             "tx_1",
		UserId:         1,
		Type:           "pre_deduct",
		Amount:         10000,
		IdempotencyKey: "1_task_duplicate_prededuct",
	}
	err := CreateWalletTransaction(tx1)
	assert.NoError(t, err)

	tx2 := &WalletTransaction{
		Id:             "tx_2",
		UserId:         1,
		Type:           "pre_deduct",
		Amount:         10000,
		IdempotencyKey: "1_task_duplicate_prededuct",
	}
	err = CreateWalletTransaction(tx2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "constraint")
}

func TestGetTransactionByIdempotencyKey(t *testing.T) {
	truncateTables(t)

	// Test finding existing transaction
	tx := &WalletTransaction{
		Id:             "tx_get_test",
		UserId:         1,
		Type:           "settle",
		Amount:         5000,
		IdempotencyKey: "1_task_get_settle",
		Metadata:       `{"test":"data"}`,
	}
	err := CreateWalletTransaction(tx)
	assert.NoError(t, err)

	found, err := GetTransactionByIdempotencyKey("1_task_get_settle")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tx_get_test", found.Id)
	assert.Equal(t, "settle", found.Type)
	assert.Equal(t, int64(5000), found.Amount)

	// Test not found case
	notFound, err := GetTransactionByIdempotencyKey("nonexistent_key")
	assert.NoError(t, err)
	assert.Nil(t, notFound)
}
