package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
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
