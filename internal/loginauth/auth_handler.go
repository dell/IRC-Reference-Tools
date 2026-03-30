package loginauth

import (
	"net/http"
	"strings"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/config"
	"github.com/gin-gonic/gin"
)

const tokenExpiry = 24 * time.Hour

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

func AdminLogin(s *config.SystemHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AdminLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ok, version, err := validateAdmin(s, req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		token, err := makeAdminToken(req.Username, version, tokenExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token creation failed"})
			return
		}
		writeAuthCookie(c, token, tokenExpiry)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "token": token})
	}
}

func Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		clearAuthCookie(c)
		c.JSON(http.StatusOK, gin.H{"status": "logged out"})
	}
}

func ChangePassword(s *config.SystemHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ChangePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		username := DefaultAdmin

		if len(strings.TrimSpace(req.NewPassword)) < 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 4 characters"})
			return
		}

		ok, newVer, err := changeAdminPassword(s, username, req.OldPassword, req.NewPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
			return
		}

		token, err := makeAdminToken(username, newVer, tokenExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token creation failed"})
			return
		}

		writeAuthCookie(c, token, tokenExpiry)
		c.JSON(http.StatusOK, gin.H{"status": "password changed", "token": token})
	}
}

func ResetToDefault(s *config.SystemHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, _, err := resetAdminToDefault(s)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "reset failed"})
			return
		}
		clearAuthCookie(c)
		c.JSON(http.StatusOK, gin.H{"status": "reset to default"})
	}
}

func Me() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("admin_username")
		c.JSON(http.StatusOK, gin.H{"user": user})
	}
}
