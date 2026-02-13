package loginauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/config"
	"github.com/gin-gonic/gin"
)

const DefaultAdmin = "admin"

func AuthRequired(s *config.SystemHandler) gin.HandlerFunc {
	return func(c *gin.Context) {

		// Try Bearer token from Authorization header first (CLI-friendly)
		var tokenStr string
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		} else {
			// Fall back to cookie (browser-friendly)
			tokenStr, _ = c.Cookie("auth_token")
		}
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		claims, err := parseAdminToken(tokenStr)
		if err != nil {
			clearAuthCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// Invalidate tokens issued before the server last started
		if claims.IssuedAt == nil || claims.IssuedAt.Time.Before(bootTime) {
			clearAuthCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			return
		}

		// Check JWT version using *cached* + fallback to GetLogin() call
		curVer, err := getCurrentJWTVersion(s, claims.Username)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "admin lookup failed"})
			return
		}

		if claims.JWTVersion != curVer {
			// try refresh from cache
			u, p, v, err2 := fetchAdminCredentials(s)
			if err2 != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "admin lookup failed"})
				return
			}
			cacheSet(u, p, v) // update cache

			if claims.JWTVersion != v {
				clearAuthCookie(c)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
				return
			}
		}

		c.Set("admin_username", claims.Username)
		c.Next()
	}
}

func BasicAuthRequired(s *config.SystemHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if !strings.HasPrefix(authz, "Basic ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		b64 := strings.TrimPrefix(authz, "Basic ")
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid basic auth"})
			return
		}
		userPass := string(raw)
		i := strings.IndexByte(userPass, ':')
		if i <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid basic auth"})
			return
		}
		username := userPass[:i]
		password := userPass[i+1:]

		ok, _, err := validateAdmin(s, username, password)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "admin lookup failed"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		c.Set("admin_username", username)
		c.Next()
	}
}

func AuthRequiredOrBasic(s *config.SystemHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenStr string
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		} else {
			tokenStr, _ = c.Cookie("auth_token")
		}

		if tokenStr == "" {
			authz := c.GetHeader("Authorization")
			ua := strings.ToLower(c.GetHeader("User-Agent"))
			isBrowser := c.GetHeader("Sec-Fetch-Mode") != "" || c.GetHeader("Sec-CH-UA") != "" || strings.Contains(ua, "mozilla")
			if strings.HasPrefix(authz, "Basic ") && !isBrowser {
				BasicAuthRequired(s)(c)
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		claims, err := parseAdminToken(tokenStr)
		if err != nil {
			clearAuthCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// Invalidate tokens issued before the server last started (or legacy tokens missing iat)
		if claims.IssuedAt == nil || claims.IssuedAt.Time.Before(bootTime) {
			clearAuthCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			return
		}

		curVer, err := getCurrentJWTVersion(s, claims.Username)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "admin lookup failed"})
			return
		}

		if claims.JWTVersion != curVer {
			u, p, v, err2 := fetchAdminCredentials(s)
			if err2 != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "admin lookup failed"})
				return
			}
			cacheSet(u, p, v)
			if claims.JWTVersion != v {
				clearAuthCookie(c)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
				return
			}
		}

		c.Set("admin_username", claims.Username)
		c.Next()
	}
}

// minimal inline helpers to avoid separate helpers.go
var (
	cacheMu       sync.RWMutex
	cachedUser    string
	cachedPass    string
	cachedVersion int
)

func cacheSet(u, p string, v int) {
	cacheMu.Lock()
	cachedUser = u
	cachedPass = p
	cachedVersion = v
	cacheMu.Unlock()
}

func fetchAdminCredentials(s *config.SystemHandler) (string, string, int, error) {
	if s == nil || s.AuthClient == nil {
		return "", "", 0, errors.New("auth client not initialized")
	}
	l := s.AuthClient.GetLogin()
	if l.Username == "" {
		return "", "", 0, errors.New("empty login from db")
	}
	v := l.JwtVersion
	if v <= 0 {
		v = 1
	}
	return l.Username, l.Password, v, nil
}

func getCurrentJWTVersion(s *config.SystemHandler, username string) (int, error) {
	cacheMu.RLock()
	u := cachedUser
	v := cachedVersion
	cacheMu.RUnlock()
	if u == username && u != "" {
		return v, nil
	}
	u2, p2, v2, err := fetchAdminCredentials(s)
	if err != nil {
		return 0, err
	}
	cacheSet(u2, p2, v2)
	if u2 != username {
		return 0, errors.New("username mismatch")
	}
	return v2, nil
}

// hash helper used by validation and cache updates
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// validate admin credentials using cached hash, fallback to bus
func validateAdmin(s *config.SystemHandler, username, password string) (bool, int, error) {
	cacheMu.RLock()
	u, h, v := cachedUser, cachedPass, cachedVersion
	cacheMu.RUnlock()
	if u != "" && username == u && sha256Hex(password) == h {
		return true, v, nil
	}
	u, h, v, err := fetchAdminCredentials(s)
	if err != nil {
		return false, 0, err
	}
	cacheSet(u, h, v)
	if username != u || sha256Hex(password) != h {
		return false, 0, nil
	}
	return true, v, nil
}

// change password by bumping jwt version and sending plaintext to db service
func changeAdminPassword(s *config.SystemHandler, username, oldPassword, newPassword string) (bool, int, error) {
	ok, curVer, err := validateAdmin(s, username, oldPassword)
	if err != nil || !ok {
		return false, 0, err
	}
	newVer := curVer + 1
	upd := auth.Login{Username: username, Password: newPassword, JwtVersion: newVer}
	if err := s.AuthClient.UpdateLogin(upd); err != nil {
		return false, 0, err
	}
	cacheSet(username, sha256Hex(newPassword), newVer)
	return true, newVer, nil
}

// reset to default admin/admin and bump version
func resetAdminToDefault(s *config.SystemHandler) (bool, int, error) {
	cacheMu.RLock()
	v := cachedVersion
	cacheMu.RUnlock()
	if v <= 0 {
		if u2, _, v2, err := fetchAdminCredentials(s); err == nil && u2 != "" {
			v = v2
		}
	}
	newVer := v + 1
	upd := auth.Login{Username: DefaultAdmin, Password: DefaultAdmin, JwtVersion: newVer}
	if err := s.AuthClient.UpdateLogin(upd); err != nil {
		return false, 0, err
	}
	cacheSet(DefaultAdmin, sha256Hex(DefaultAdmin), newVer)
	return true, newVer, nil
}
