package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"zenmind/internal/config"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
)

// GetZentaoAPIConfig GET /api/config/zentao-api
func GetZentaoAPIConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"base_url":  config.Global.ZentaoBaseURL,
		"login_url": config.Global.ZentaoLoginURL,
	})
}

type putZentaoAPIConfigBody struct {
	BaseURL  string `json:"base_url" binding:"required"`
	LoginURL string `json:"login_url" binding:"required"`
}

// PutZentaoAPIConfig PUT /api/config/zentao-api
func PutZentaoAPIConfig(c *gin.Context) {
	var req putZentaoAPIConfigBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s := strings.TrimSpace(req.BaseURL)
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base_url (expected like https://zentao.example.com)"})
		return
	}
	s = strings.TrimRight(s, "/")

	loginURL := strings.TrimSpace(req.LoginURL)
	lu, err := url.Parse(loginURL)
	if err != nil || lu.Scheme == "" || lu.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login_url (expected like https://zentao.example.com/user-login.html)"})
		return
	}
	loginURL = strings.TrimSpace(loginURL)

	if err := db.SetZentaoBaseURL(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := db.SetZentaoLoginURL(loginURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	config.Global.ZentaoBaseURL = s
	config.Global.ZentaoLoginURL = loginURL
	c.JSON(http.StatusOK, gin.H{"message": "zentao api config updated", "base_url": s, "login_url": loginURL})
}
