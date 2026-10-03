package controller

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func SSOAuthorize(c *gin.Context) {
	redirectUri := c.Query("redirect_uri")
	state := c.Query("state")

	// Explicit validation for empty redirect_uri (before whitelist check)
	if redirectUri == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "redirect_uri is required"})
		return
	}

	// Validate redirect_uri against whitelist
	whitelist := strings.Split(common.GetEnvOrDefaultString("CANVAS_REDIRECT_URI_WHITELIST", ""), ",")
	valid := false
	for _, allowed := range whitelist {
		if strings.TrimSpace(allowed) == redirectUri {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_redirect_uri"})
		return
	}

	// Check if user is logged in
	userId := c.GetInt("user_id")
	if userId == 0 {
		// Redirect to login with return_url
		returnUrl := url.QueryEscape(c.Request.URL.String())
		c.Redirect(http.StatusFound, "/login?return_url="+returnUrl)
		return
	}

	// Generate auth code (24 bytes = 32 base64url characters)
	authCode, err := model.GenerateAuthCode(userId, redirectUri, state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate code"})
		return
	}

	// Redirect back with code
	separator := "?"
	if strings.Contains(redirectUri, "?") {
		separator = "&"
	}
	location := redirectUri + separator + "code=" + authCode.Code
	if state != "" {
		location += "&state=" + state
	}
	c.Redirect(http.StatusFound, location)
}
