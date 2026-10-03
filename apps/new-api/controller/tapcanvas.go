package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type BalanceQueryRequest struct {
	UserID int `json:"user_id" binding:"required"`
}

type BalanceQueryResponse struct {
	Balance        int    `json:"balance"`
	UsedQuota      int    `json:"used_quota"`
	DisplayBalance string `json:"display_balance"`
}

func QueryUserBalance(c *gin.Context) {
	var req BalanceQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}

	user, err := model.GetUserById(req.UserID, false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}

	displayBalance := logger.LogQuota(user.Quota)

	c.JSON(http.StatusOK, BalanceQueryResponse{
		Balance:        user.Quota,
		UsedQuota:      user.UsedQuota,
		DisplayBalance: displayBalance,
	})
}
