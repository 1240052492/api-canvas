package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func ServiceTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			c.Abort()
			return
		}

		tokenKey := parts[1]

		// Redact token in request context for logging
		c.Set("_redacted_token", "sk-***")

		token, err := model.ValidateUserToken(tokenKey)
		if err != nil || token == nil {
			logger.LogError(c, "invalid service token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			c.Abort()
			return
		}

		// Verify this is a service token
		if token.ServiceName == nil || *token.ServiceName == "" {
			logger.LogError(c, "user token used for service endpoint")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token_type"})
			c.Abort()
			return
		}

		c.Set("service_name", *token.ServiceName)
		c.Set("token_id", token.Id)
		c.Next()
	}
}
