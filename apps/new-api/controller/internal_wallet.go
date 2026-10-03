package controller

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
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
		// Return existing result
		user, err := model.GetUserById(req.UserId, false)
		if err != nil || user == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user_not_found"})
			return
		}
		balance := int64(user.Quota) - int64(user.UsedQuota)
		c.JSON(http.StatusOK, PreDeductResponse{
			DeductionId:  existing.Id,
			Amount:       existing.Amount,
			BalanceAfter: balance,
		})
		return
	}

	// Transaction: check balance and deduct
	var balanceAfter int64
	deductionId := "deduct_" + generateRandomString(16)

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

		// Record transaction
		metadataJSON, _ := json.Marshal(req.Metadata)
		walletTx := &model.WalletTransaction{
			Id:             deductionId,
			UserId:         req.UserId,
			Type:           "pre_deduct",
			Amount:         req.Amount,
			IdempotencyKey: req.IdempotencyKey,
			DeductionId:    "",
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
			Id:             "settle_" + generateRandomString(16),
			UserId:         preDeduct.UserId,
			Type:           "settle",
			Amount:         req.ActualCost,
			IdempotencyKey: req.IdempotencyKey,
			DeductionId:    req.DeductionId,
		}
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

type RefundRequest struct {
	DeductionId    string `json:"deduction_id" binding:"required"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
	Reason         string `json:"reason"`
}

type RefundResponse struct {
	Refunded     bool  `json:"refunded"`
	Amount       int64 `json:"amount"`
	BalanceAfter int64 `json:"balance_after"`
}

func RefundCredits(c *gin.Context) {
	var req RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	// Check idempotency
	existing, _ := model.GetTransactionByIdempotencyKey(req.IdempotencyKey)
	if existing != nil {
		c.JSON(http.StatusOK, RefundResponse{Refunded: true, Amount: existing.Amount})
		return
	}

	// Get pre-deduct transaction
	var preDeduct model.WalletTransaction
	if err := model.DB.Where("id = ? AND type = ?", req.DeductionId, "pre_deduct").First(&preDeduct).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deduction_not_found"})
		return
	}

	var balanceAfter int64

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Where("id = ?", preDeduct.UserId).First(&user).Error; err != nil {
			return err
		}

		// Refund full amount
		if err := tx.Model(&user).Update("used_quota", gorm.Expr("used_quota - ?", preDeduct.Amount)).Error; err != nil {
			return err
		}

		balanceAfter = int64(user.Quota) - int64(user.UsedQuota) + preDeduct.Amount

		// Record refund transaction
		refundTx := &model.WalletTransaction{
			Id:             "refund_" + generateRandomString(16),
			UserId:         preDeduct.UserId,
			Type:           "refund",
			Amount:         preDeduct.Amount,
			IdempotencyKey: req.IdempotencyKey,
			DeductionId:    req.DeductionId,
			Metadata:       `{"reason":"` + req.Reason + `"}`,
		}
		return tx.Create(refundTx).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, RefundResponse{
		Refunded:     true,
		Amount:       preDeduct.Amount,
		BalanceAfter: balanceAfter,
	})
}
