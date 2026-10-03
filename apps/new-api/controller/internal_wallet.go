package controller

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

	balance := int64(user.Quota - user.UsedQuota)
	displayBalance := fmt.Sprintf("$%.2f", float64(balance)/1000000.0)

	c.JSON(http.StatusOK, WalletBalanceResponse{
		UserId:         user.Id,
		Balance:        balance,
		UsedQuota:      int64(user.UsedQuota),
		DisplayBalance: displayBalance,
	})
}

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

func generateSecureRandomId() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return "deduct_" + strconv.FormatInt(int64(len(b))*1000000+int64(b[0]), 36)
	}
	return "deduct_" + hex.EncodeToString(b)
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
		// Return cached result with stored balance_after for strict idempotency
		var cachedBalance int64
		if existing.Metadata != "" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(existing.Metadata), &metadata); err == nil {
				if bal, ok := metadata["balance_after"].(float64); ok {
					cachedBalance = int64(bal)
				}
			}
		}
		// If balance_after not cached in metadata, recalculate (backward compat)
		if cachedBalance == 0 {
			user, err := model.GetUserById(req.UserId, false)
			if err != nil || user == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "user_not_found"})
				return
			}
			cachedBalance = int64(user.Quota) - int64(user.UsedQuota)
		}
		c.JSON(http.StatusOK, PreDeductResponse{
			DeductionId:  existing.Id,
			Amount:       existing.Amount,
			BalanceAfter: cachedBalance,
		})
		return
	}

	// Transaction: check balance and deduct
	var balanceAfter int64
	deductionId := generateSecureRandomId()

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Where("id = ?", req.UserId).First(&user).Error; err != nil {
			return err
		}

		balance := int64(user.Quota) - int64(user.UsedQuota)
		if balance < req.Amount {
			return errors.New("insufficient quota")
		}

		// Increase used_quota (freeze credits)
		if err := tx.Model(&user).Update("used_quota", gorm.Expr("used_quota + ?", req.Amount)).Error; err != nil {
			return err
		}

		balanceAfter = balance - req.Amount

		// Record transaction with balance_after in metadata for idempotency
		reqMetadata := req.Metadata
		if reqMetadata == nil {
			reqMetadata = make(map[string]string)
		}
		// Store the balance_after for strict idempotency on repeated calls
		metadataMap := make(map[string]interface{})
		for k, v := range reqMetadata {
			metadataMap[k] = v
		}
		metadataMap["balance_after"] = balanceAfter

		metadataJSON, err := json.Marshal(metadataMap)
		if err != nil {
			// Log error but don't fail the transaction
			fmt.Printf("failed to marshal metadata: %v\n", err)
			metadataJSON = []byte("{}")
		}

		walletTx := &model.WalletTransaction{
			Id:             deductionId,
			UserId:         req.UserId,
			Type:           "pre_deduct",
			Amount:         req.Amount,
			IdempotencyKey: req.IdempotencyKey,
			DeductionId:    deductionId, // Self-reference for audit trail
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

		balanceAfter = int64(user.Quota) - int64(user.UsedQuota) + refundAmount

		// Record settle transaction
		settleTx := &model.WalletTransaction{
			Id:             "settle_" + hex.EncodeToString(make([]byte, 8)),
			UserId:         preDeduct.UserId,
			Type:           "settle",
			Amount:         req.ActualCost,
			IdempotencyKey: req.IdempotencyKey,
			DeductionId:    req.DeductionId,
		}
		// Generate proper random ID
		b := make([]byte, 8)
		rand.Read(b)
		settleTx.Id = "settle_" + hex.EncodeToString(b)

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
