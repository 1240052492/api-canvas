package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type BalanceQueryResponse struct {
	Balance        int    `json:"balance"`
	UsedQuota      int    `json:"used_quota"`
	DisplayBalance string `json:"display_balance"`
}

func QueryUserBalance(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "invalid user_id"})
		return
	}

	user, err := model.GetUserById(userID, false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}

	// Convert quota to dollar amount for TapCanvas
	dollarAmount := common.QuotaToCNYAmount(user.Quota)
	displayBalance := fmt.Sprintf("$%.2f", dollarAmount)

	c.JSON(http.StatusOK, BalanceQueryResponse{
		Balance:        user.Quota,
		UsedQuota:      user.UsedQuota,
		DisplayBalance: displayBalance,
	})
}
