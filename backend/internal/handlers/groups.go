package handlers

import (
	"net/http"
	"strconv"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

type createGroupReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type updateMembersReq struct {
	Accounts []string `json:"accounts" binding:"required"`
}

// ListGroups GET /api/groups
func ListGroups(c *gin.Context) {
	var groups []models.ProjectGroup
	db.PG.Find(&groups)
	c.JSON(http.StatusOK, gin.H{"data": groups})
}

// CreateGroup POST /api/groups
func CreateGroup(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	var req createGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	g := models.ProjectGroup{Name: req.Name, Description: req.Description}
	if err := db.PG.Create(&g).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, g)
}

// UpdateGroup PUT /api/groups/:id
func UpdateGroup(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	var req createGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db.PG.Model(&models.ProjectGroup{}).Where("id = ?", id).
		Updates(map[string]interface{}{"name": req.Name, "description": req.Description})
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DeleteGroup DELETE /api/groups/:id
func DeleteGroup(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	db.PG.Where("id = ?", id).Delete(&models.ProjectGroup{})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// groupMemberRow is used for GET /groups/:id/members — join local_users for display names.
type groupMemberRow struct {
	Account  string `json:"account"`
	Realname string `json:"realname"`
}

// GetGroupMembers GET /api/groups/:id/members
func GetGroupMembers(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var rows []groupMemberRow
	db.PG.Table("group_members").
		Select("group_members.account, COALESCE(local_users.realname, '') AS realname").
		Joins("LEFT JOIN local_users ON local_users.account = group_members.account").
		Where("group_members.group_id = ?", id).
		Order("group_members.account").
		Scan(&rows)

	accounts := make([]string, len(rows))
	for i, r := range rows {
		accounts[i] = r.Account
	}
	c.JSON(http.StatusOK, gin.H{
		"accounts": accounts,
		"members":  rows,
	})
}

// UpdateGroupMembers PUT /api/groups/:id/members
// Replaces all members atomically.
func UpdateGroupMembers(c *gin.Context) {
	if _, ok := RequireAdmin(c); !ok {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	var req updateMembersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Transaction: delete old, insert new
	tx := db.PG.Begin()
	tx.Where("group_id = ?", id).Delete(&models.GroupMember{})
	for _, acc := range req.Accounts {
		tx.Create(&models.GroupMember{GroupID: id, Account: acc})
	}
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "transaction failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "members updated", "count": len(req.Accounts)})
}
