package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// getGroupAccounts returns member accounts for a group.
func getGroupAccounts(groupID int) []string {
	var members []models.GroupMember
	db.PG.Where("group_id = ?", groupID).Find(&members)
	accounts := make([]string, len(members))
	for i, m := range members {
		accounts[i] = m.Account
	}
	return accounts
}

// groupFilter resolves project-group isolation for workbench queries.
// When groupID <= 0, no group filter is applied (accounts == nil, showNone == false).
// When groupID > 0 but the group has no members, showNone is true → caller should return no rows.
// When groupID > 0 and there are members, accounts is non-nil.
func groupFilter(groupID int) (accounts []string, showNone bool) {
	if groupID <= 0 {
		return nil, false
	}
	accounts = getGroupAccounts(groupID)
	if len(accounts) == 0 {
		return nil, true
	}
	return accounts, false
}

// enforceMaxRange checks date range ≤ 6 months (for large tables).
func enforceMaxRange(from, to time.Time) bool {
	return to.Sub(from) <= 6*30*24*time.Hour
}

// firstVisibleTask loads a task by id if it exists and passes workbench group isolation
// (assignee must be in group when group_id > 0).
func firstVisibleTask(groupID int, taskID int64) (models.LocalTask, error) {
	accounts, showNone := groupFilter(groupID)
	if showNone {
		return models.LocalTask{}, gorm.ErrRecordNotFound
	}
	q := db.PG.Where("id = ? AND deleted = false", taskID)
	if accounts != nil {
		q = q.Where("assigned_to IN ?", accounts)
	}
	var t models.LocalTask
	err := q.First(&t).Error
	return t, err
}

// GetTask GET /api/workbench/tasks/:id
func GetTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}
	groupID := queryInt(c, "group_id")

	if queryMyBinding(c) {
		acc, ok := requireBinding(c)
		if !ok {
			return
		}
		t, err := firstVisibleTask(0, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if strings.TrimSpace(t.AssignedTo) != acc {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusOK, t)
		return
	}

	// Scope enforcement: SELF can only access own tasks (by assignee).
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			t, err := firstVisibleTask(0, id)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if strings.TrimSpace(t.AssignedTo) != acc {
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
				return
			}
			c.JSON(http.StatusOK, t)
			return
		case scopeGroup:
			// Allow GROUP users to view tasks assigned to themselves
			// without group isolation (needed by "My Workbench").
			t, err := firstVisibleTask(0, id)
			if err == nil && cu.ZentaoBinding != nil {
				selfAcc := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
				if selfAcc != "" && strings.TrimSpace(t.AssignedTo) == selfAcc {
					c.JSON(http.StatusOK, t)
					return
				}
			}
			gid, ok := effectiveGroupID(c, groupID)
			if !ok {
				return
			}
			groupID = gid
		default:
			// ALL: keep requested group_id
		}
	}

	t, err := firstVisibleTask(groupID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// ListTasks GET /api/workbench/tasks
func ListTasks(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	status := c.Query("status")
	assignedTo := c.Query("assigned_to")
	dateFrom := queryDate(c, "date_from")
	dateTo := queryDate(c, "date_to")
	page, pageSize := parsePagination(c)
	projectID := int64(queryInt(c, "project_id"))
	programID := int64(queryInt(c, "program_id"))
	productID := int64(queryInt(c, "product_id"))

	query := db.PG.Model(&models.LocalTask{}).Where("deleted = false")

	// Scope enforcement
	myBind := queryMyBinding(c)
	if myBind {
		acc, ok := requireBinding(c)
		if !ok {
			return
		}
		assignedTo = acc
		groupID = 0
	} else if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			assignedTo = acc
			groupID = 0
		case scopeGroup:
			// If the user is querying their own data (assigned_to matches their
			// zentao binding), let it through without group isolation so that the
			// "My Workbench" page always shows only the user's own tasks.
			if cu.ZentaoBinding != nil {
				selfAcc := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
				if selfAcc != "" && strings.TrimSpace(assignedTo) == selfAcc {
					groupID = 0 // skip group filter, keep assignedTo as-is
					break
				}
			}
			gid, ok := effectiveGroupID(c, groupID)
			if !ok {
				return
			}
			groupID = gid
		}
	}

	if accounts, showNone := groupFilter(groupID); showNone {
		query = query.Where("1 = 0")
	} else if accounts != nil {
		query = query.Where("assigned_to IN ?", accounts)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if assignedTo != "" {
		query = query.Where("assigned_to = ?", assignedTo)
	}
	if !dateFrom.IsZero() && !dateTo.IsZero() {
		query = query.Where("last_edited_date BETWEEN ? AND ?", dateFrom, dateTo)
	}
	if execID := int64(queryInt(c, "execution_id")); execID > 0 {
		query = query.Where("execution_id = ?", execID)
	}
	if projectID > 0 {
		query = query.Where("execution_id IN (SELECT id FROM local_executions WHERE deleted = false AND parent_id = ?)", projectID)
	}
	if programID > 0 {
		query = query.Where(`
			execution_id IN (
				SELECT e.id FROM local_executions e
				JOIN local_projects p ON p.id = e.parent_id
				WHERE e.deleted = false AND p.deleted = false AND p.parent_id = ?
			)`, programID)
	}
	if productID > 0 {
		query = query.Where("story_id IN (SELECT id FROM local_stories WHERE deleted = false AND product_id = ?)", productID)
	}
	if recordID := int64(queryInt(c, "id")); recordID > 0 {
		query = query.Where("id = ?", recordID)
	}

	var total int64
	query.Count(&total)
	var rows []models.LocalTask
	query.Offset((page - 1) * pageSize).Limit(pageSize).Order("last_edited_date DESC").Find(&rows)
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}

// ListStories GET /api/workbench/stories
func ListStories(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	status := c.Query("status")
	assignedTo := c.Query("assigned_to")
	dateFrom := queryDate(c, "date_from")
	dateTo := queryDate(c, "date_to")
	page, pageSize := parsePagination(c)
	productID := int64(queryInt(c, "product_id"))

	query := db.PG.Model(&models.LocalStory{}).Where("deleted = false")
	// Scope enforcement
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			assignedTo = acc
			groupID = 0
		case scopeGroup:
			if cu.ZentaoBinding != nil {
				selfAcc := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
				if selfAcc != "" && strings.TrimSpace(assignedTo) == selfAcc {
					groupID = 0
					break
				}
			}
			gid, ok := effectiveGroupID(c, groupID)
			if !ok {
				return
			}
			groupID = gid
		}
	}
	if accounts, showNone := groupFilter(groupID); showNone {
		query = query.Where("1 = 0")
	} else if accounts != nil {
		query = query.Where("assigned_to IN ?", accounts)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if assignedTo != "" {
		query = query.Where("assigned_to = ?", assignedTo)
	}
	if !dateFrom.IsZero() && !dateTo.IsZero() {
		query = query.Where("last_edited_date BETWEEN ? AND ?", dateFrom, dateTo)
	}
	if execID := int64(queryInt(c, "execution_id")); execID > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT story_id FROM local_tasks
				WHERE deleted = false AND execution_id = ? AND story_id > 0
				UNION
				SELECT DISTINCT story_id FROM local_bugs
				WHERE deleted = false AND execution_id = ? AND story_id > 0
			)`, execID, execID)
	}
	if productID > 0 {
		query = query.Where("product_id = ?", productID)
	}
	if recordID := int64(queryInt(c, "id")); recordID > 0 {
		query = query.Where("id = ?", recordID)
	}

	var total int64
	query.Count(&total)
	var rows []models.LocalStory
	query.Offset((page - 1) * pageSize).Limit(pageSize).Order("last_edited_date DESC").Find(&rows)
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}

// ListBugs GET /api/workbench/bugs
func ListBugs(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	status := c.Query("status")
	severity := c.Query("severity")
	assignedTo := c.Query("assigned_to")
	page, pageSize := parsePagination(c)
	projectID := int64(queryInt(c, "project_id"))
	programID := int64(queryInt(c, "program_id"))
	productID := int64(queryInt(c, "product_id"))

	query := db.PG.Model(&models.LocalBug{}).Where("deleted = false")
	// Scope enforcement
	myBind := queryMyBinding(c)
	if myBind {
		acc, ok := requireBinding(c)
		if !ok {
			return
		}
		assignedTo = acc
		groupID = 0
	} else if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			assignedTo = acc
			groupID = 0
		case scopeGroup:
			if cu.ZentaoBinding != nil {
				selfAcc := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
				if selfAcc != "" && strings.TrimSpace(assignedTo) == selfAcc {
					groupID = 0
					break
				}
			}
			gid, ok := effectiveGroupID(c, groupID)
			if !ok {
				return
			}
			groupID = gid
		}
	}
	if accounts, showNone := groupFilter(groupID); showNone {
		query = query.Where("1 = 0")
	} else if accounts != nil {
		query = query.Where("assigned_to IN ?", accounts)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if severity != "" {
		query = query.Where("severity = ?", severity)
	}
	if assignedTo != "" {
		query = query.Where("assigned_to = ?", assignedTo)
	}
	if execID := int64(queryInt(c, "execution_id")); execID > 0 {
		query = query.Where("execution_id = ?", execID)
	}
	if projectID > 0 {
		query = query.Where("execution_id IN (SELECT id FROM local_executions WHERE deleted = false AND parent_id = ?)", projectID)
	}
	if programID > 0 {
		query = query.Where(`
			execution_id IN (
				SELECT e.id FROM local_executions e
				JOIN local_projects p ON p.id = e.parent_id
				WHERE e.deleted = false AND p.deleted = false AND p.parent_id = ?
			)`, programID)
	}
	if productID > 0 {
		query = query.Where("story_id IN (SELECT id FROM local_stories WHERE deleted = false AND product_id = ?)", productID)
	}
	if recordID := int64(queryInt(c, "id")); recordID > 0 {
		query = query.Where("id = ?", recordID)
	}

	var total int64
	query.Count(&total)
	var rows []models.LocalBug
	query.Offset((page - 1) * pageSize).Limit(pageSize).Order("last_edited_date DESC").Find(&rows)
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}

// ListEfforts GET /api/workbench/efforts
// 强制 6 个月时间跨度限制
func ListEfforts(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	account := c.Query("account")
	objectType := strings.TrimSpace(c.Query("object_type"))
	objectIDStr := strings.TrimSpace(c.Query("object_id"))
	dateFrom := queryDate(c, "date_from")
	dateTo := queryDate(c, "date_to")
	page, pageSize := parsePagination(c)

	// Scope enforcement
	myBind := queryMyBinding(c)
	var bindingAcc string
	if myBind {
		acc, ok := requireBinding(c)
		if !ok {
			return
		}
		account = acc
		groupID = 0
		bindingAcc = acc
	} else if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			account = acc
			groupID = 0
		case scopeGroup:
			// If the user is querying their own data (account matches their
			// zentao binding), let it through without group isolation.
			if cu.ZentaoBinding != nil {
				selfAcc := strings.TrimSpace(cu.ZentaoBinding.ZentaoAccount)
				if selfAcc != "" && strings.TrimSpace(account) == selfAcc {
					groupID = 0
					break
				}
			}
			gid, ok := effectiveGroupID(c, groupID)
			if !ok {
				return
			}
			groupID = gid
		}
	}

	if dateFrom.IsZero() || dateTo.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date_from and date_to are required for effort queries"})
		return
	}
	if !enforceMaxRange(dateFrom, dateTo) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date range must not exceed 6 months"})
		return
	}

	query := db.PG.Model(&models.LocalEffort{}).Where("deleted = false").
		Where("work_date BETWEEN ? AND ?", dateFrom, dateTo)
	if recordID := int64(queryInt(c, "id")); recordID > 0 {
		query = query.Where("id = ?", recordID)
	}

	if accounts, showNone := groupFilter(groupID); showNone {
		query = query.Where("1 = 0")
	} else if accounts != nil {
		query = query.Where("account IN ?", accounts)
	}
	if account != "" {
		query = query.Where("account = ?", account)
	}

	execID := int64(queryInt(c, "execution_id"))
	taskID := int64(queryInt(c, "task_id"))
	var objectID int64
	if objectIDStr != "" {
		oid, err := strconv.ParseInt(objectIDStr, 10, 64)
		if err != nil || oid <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid object_id"})
			return
		}
		objectID = oid
	}

	if taskID > 0 {
		t, err := firstVisibleTask(groupID, taskID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "task not found or not visible"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if myBind && strings.TrimSpace(t.AssignedTo) != bindingAcc {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task not found or not visible"})
			return
		}
		if execID > 0 && t.ExecutionID != execID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task does not belong to selected iteration"})
			return
		}
		query = query.Where("LOWER(TRIM(object_type)) = 'task' AND object_id = ?", taskID)
	} else if execID > 0 {
		query = query.Where(`
			(
				(LOWER(TRIM(object_type)) = 'task' AND object_id IN (SELECT id FROM local_tasks WHERE deleted = false AND execution_id = ?))
				OR (LOWER(TRIM(object_type)) = 'bug' AND object_id IN (SELECT id FROM local_bugs WHERE deleted = false AND execution_id = ?))
				OR (LOWER(TRIM(object_type)) = 'story' AND object_id IN (
					SELECT story_id FROM local_tasks WHERE deleted = false AND execution_id = ? AND story_id > 0
					UNION
					SELECT story_id FROM local_bugs WHERE deleted = false AND execution_id = ? AND story_id > 0
				))
			)`, execID, execID, execID, execID)
	}

	// Drilldown filters (optional). These are applied only when task_id is not forcing object_type=task.
	if taskID <= 0 {
		if objectType != "" {
			query = query.Where("LOWER(TRIM(object_type)) = LOWER(TRIM(?))", objectType)
		}
		if objectID > 0 {
			query = query.Where("object_id = ?", objectID)
		}
	}

	var total int64
	query.Count(&total)
	var rows []models.LocalEffort
	query.Offset((page - 1) * pageSize).Limit(pageSize).Order("work_date DESC").Find(&rows)
	c.JSON(http.StatusOK, pageResponse(effortsWithTitles(rows), total, page, pageSize))
}

// ListExecutions GET /api/workbench/executions
// 迭代表无 account 字段：通过组成员在任务/缺陷上出现过的 execution_id 界定可见迭代。
func ListExecutions(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	status := c.Query("status")
	name := strings.TrimSpace(c.Query("name"))
	dateFrom := queryDate(c, "date_from")
	dateTo := queryDate(c, "date_to")
	page, pageSize := parsePagination(c)
	projectID := int64(queryInt(c, "project_id"))

	query := db.PG.Model(&models.LocalExecution{}).Where("deleted = false")

	// Scope enforcement
	var selfAccounts []string
	if cu := GetCurrentUser(c); cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	} else {
		switch normalizeScope(cu.User.DataScope) {
		case scopeSelf:
			acc, ok := requireBinding(c)
			if !ok {
				return
			}
			selfAccounts = []string{acc}
			groupID = 0
		case scopeGroup:
			gid, ok := effectiveGroupID(c, groupID)
			if !ok {
				return
			}
			groupID = gid
		}
	}

	if accounts, showNone := groupFilter(groupID); showNone {
		query = query.Where("1 = 0")
	} else if accounts != nil {
		query = query.Where(`
			id IN (
				SELECT DISTINCT execution_id FROM local_tasks
				WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
				UNION
				SELECT DISTINCT execution_id FROM local_bugs
				WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
			)`, accounts, accounts)
	}
	if len(selfAccounts) > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT execution_id FROM local_tasks
				WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
				UNION
				SELECT DISTINCT execution_id FROM local_bugs
				WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
			)`, selfAccounts, selfAccounts)
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if execID := int64(queryInt(c, "execution_id")); execID > 0 {
		query = query.Where("id = ?", execID)
	}
	if name != "" {
		like := "%" + strings.ToLower(name) + "%"
		query = query.Where("LOWER(name) LIKE ?", like)
	}
	if !dateFrom.IsZero() && !dateTo.IsZero() {
		query = query.Where("begin_date >= ? AND end_date <= ?", dateFrom, dateTo)
	}
	if projectID > 0 {
		query = query.Where("parent_id = ?", projectID)
	}

	var total int64
	query.Count(&total)
	var rows []models.LocalExecution
	query.Offset((page - 1) * pageSize).Limit(pageSize).Order("begin_date DESC").Find(&rows)
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}

// workbenchProjectBaseQuery applies the same project visibility rules as execution visibility:
// in group/self scope, only projects that own at least one execution where the group/self
// has assigned tasks or bugs are listed.
func workbenchProjectBaseQuery(c *gin.Context, groupID int) (*gorm.DB, int, bool) {
	query := db.PG.Model(&models.LocalProject{}).Where("deleted = false")

	var selfAccounts []string
	cu := GetCurrentUser(c)
	if cu == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, 0, false
	}
	switch normalizeScope(cu.User.DataScope) {
	case scopeSelf:
		acc, ok := requireBinding(c)
		if !ok {
			return nil, 0, false
		}
		selfAccounts = []string{acc}
		groupID = 0
	case scopeGroup:
		gid, ok := effectiveGroupID(c, groupID)
		if !ok {
			return nil, 0, false
		}
		groupID = gid
	}

	if accounts, showNone := groupFilter(groupID); showNone {
		query = query.Where("1 = 0")
	} else if accounts != nil {
		query = query.Where(`
			id IN (
				SELECT DISTINCT e.parent_id FROM local_executions e
				WHERE e.deleted = false AND e.parent_id > 0 AND e.id IN (
					SELECT DISTINCT execution_id FROM local_tasks
					WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
					UNION
					SELECT DISTINCT execution_id FROM local_bugs
					WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
				)
			)`, accounts, accounts)
	}
	if len(selfAccounts) > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT e.parent_id FROM local_executions e
				WHERE e.deleted = false AND e.parent_id > 0 AND e.id IN (
					SELECT DISTINCT execution_id FROM local_tasks
					WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
					UNION
					SELECT DISTINCT execution_id FROM local_bugs
					WHERE deleted = false AND execution_id > 0 AND assigned_to IN ?
				)
			)`, selfAccounts, selfAccounts)
	}
	return query, groupID, true
}

// ListWorkbenchProjects GET /api/workbench/projects
func ListWorkbenchProjects(c *gin.Context) {
	groupID := queryInt(c, "group_id")
	name := strings.TrimSpace(c.Query("name"))
	page, pageSize := parsePagination(c)
	projectID := int64(queryInt(c, "project_id"))
	programID := int64(queryInt(c, "program_id"))
	execID := int64(queryInt(c, "execution_id"))

	query, _, ok := workbenchProjectBaseQuery(c, groupID)
	if !ok {
		return
	}

	if programID > 0 {
		query = query.Where("parent_id = ?", programID)
	}
	if projectID > 0 {
		query = query.Where("id = ?", projectID)
	}
	if execID > 0 {
		query = query.Where("id = (SELECT parent_id FROM local_executions WHERE deleted = false AND id = ?)", execID)
	}
	if name != "" {
		like := "%" + strings.ToLower(name) + "%"
		query = query.Where("LOWER(name) LIKE ?", like)
	}

	var total int64
	query.Count(&total)
	var rows []models.LocalProject
	query.Offset((page - 1) * pageSize).Limit(pageSize).Order("id DESC").Find(&rows)
	c.JSON(http.StatusOK, pageResponse(rows, total, page, pageSize))
}

type workbenchProjectExecSummary struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	BeginDate *time.Time `json:"begin_date"`
	EndDate   *time.Time `json:"end_date"`
}

// GetWorkbenchProject GET /api/workbench/projects/:id
func GetWorkbenchProject(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	groupID := queryInt(c, "group_id")

	query, _, ok := workbenchProjectBaseQuery(c, groupID)
	if !ok {
		return
	}
	query = query.Where("id = ?", id)

	var proj models.LocalProject
	if err := query.First(&proj).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	programName := ""
	if proj.ParentID != nil && *proj.ParentID > 0 {
		var prog models.LocalProgram
		if err := db.PG.Where("id = ? AND deleted = false", *proj.ParentID).First(&prog).Error; err == nil {
			programName = prog.Name
		}
	}

	var execs []models.LocalExecution
	db.PG.Where("deleted = false AND parent_id = ?", proj.ID).
		Order("begin_date DESC NULLS LAST").
		Limit(100).
		Find(&execs)
	summaries := make([]workbenchProjectExecSummary, 0, len(execs))
	for _, e := range execs {
		summaries = append(summaries, workbenchProjectExecSummary{
			ID:        e.ID,
			Name:      e.Name,
			Status:    e.Status,
			BeginDate: e.BeginDate,
			EndDate:   e.EndDate,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"project":      proj,
		"program_name": programName,
		"executions":   summaries,
	})
}
