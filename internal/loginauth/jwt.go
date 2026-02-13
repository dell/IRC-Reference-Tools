package loginauth

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte
var bootTime time.Time

type AdminClaims struct {
	Username   string `json:"username"`
	JWTVersion int    `json:"jwt_version"`
	jwt.RegisteredClaims
}

func InitJWT() {
	jwtSecret = []byte(os.Getenv("CONFIGUI_JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("dev-secret-unsafe-change-me")
	}
	bootTime = time.Now()
}

func makeAdminToken(username string, version int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := AdminClaims{
		Username:   username,
		JWTVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(jwtSecret)
}

func parseAdminToken(tokenStr string) (*AdminClaims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &AdminClaims{}, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if c, ok := tok.Claims.(*AdminClaims); ok && tok.Valid {
		return c, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}

func writeAuthCookie(c *gin.Context, token string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	_ = ttl
	// Session cookie (no Max-Age). JWT expiry is still enforced server-side.
	c.SetCookie("auth_token", token, 0, "/", "", true, true)
}

func clearAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("auth_token", "", -1, "/", "", true, true)
}
