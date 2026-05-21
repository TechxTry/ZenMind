package handlers

import (
	"net/http"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type CurrentUser struct {
	User          models.SystemUser     `json:"user"`
	ZentaoBinding *models.ZentaoBinding `json:"zentao_binding,omitempty"`
}

// Login godoc
// POST /api/login
func Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	u, ok, err := db.GetSystemUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok || u.Disabled || !db.CheckPassword(u.PasswordHash, req.Password) {
		_ = db.WriteAudit(db.AuditInput{
			ActorUsername: req.Username,
			Action:        "login_failed",
			TargetType:    "system_user",
			TargetID:      req.Username,
			Metadata:      models.JSONB{"reason": "invalid_credentials"},
			IP:            c.ClientIP(),
			UA:            c.GetHeader("User-Agent"),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":   u.ID,
		"sub":   u.Username,
		"role":  u.Role,
		"scope": u.DataScope,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})
	cfg := config.Global
	signed, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &u.ID,
		ActorUsername: u.Username,
		Action:        "login_success",
		TargetType:    "system_user",
		TargetID:      u.Username,
		IP:            c.ClientIP(),
		UA:            c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"token": signed, "username": u.Username})
}

// JWTMiddleware validates Bearer token.
func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		tokenStr := authHeader[7:]
		tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(config.Global.JWTSecret), nil
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}

		var uid int64
		switch v := claims["uid"].(type) {
		case float64:
			uid = int64(v)
		case int64:
			uid = v
		case int:
			uid = int64(v)
		}
		sub, _ := claims["sub"].(string)
		if uid <= 0 && sub == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing subject"})
			return
		}

		// Load system user from DB (authoritative for role/scope/disabled).
		var u models.SystemUser
		var found bool
		if uid > 0 {
			uu, ok, e := db.GetSystemUserByID(uid)
			if e != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": e.Error()})
				return
			}
			u, found = uu, ok
		} else {
			uu, ok, e := db.GetSystemUserByUsername(sub)
			if e != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": e.Error()})
				return
			}
			u, found = uu, ok
		}
		if !found || u.Disabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user disabled or missing"})
			return
		}

		// Load zentao binding (optional)
		var binding *models.ZentaoBinding
		if b, ok, e := db.GetZentaoBindingBySystemUserID(u.ID); e != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": e.Error()})
			return
		} else if ok {
			binding = &b
		}

		c.Set("current_user", CurrentUser{User: u, ZentaoBinding: binding})
		// Backward-compatible: old handlers expect sub as username string.
		c.Set("sub", u.Username)
		c.Next()
	}
}

// Me GET /api/me
func Me(c *gin.Context) {
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, cu)
}

func GetCurrentUser(c *gin.Context) *CurrentUser {
	v, ok := c.Get("current_user")
	if !ok {
		return nil
	}
	if cu, ok := v.(CurrentUser); ok {
		return &cu
	}
	if cu, ok := v.(*CurrentUser); ok {
		return cu
	}
	return nil
}
