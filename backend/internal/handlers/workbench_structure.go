package handlers

import (
	"net/http"
	"sort"
	"strings"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"github.com/gin-gonic/gin"
)

type WorkbenchTreeNode struct {
	Key      string              `json:"key"`
	Type     string              `json:"type"`
	ID       int64               `json:"id"`
	Title    string              `json:"title"`
	ParentID *int64              `json:"parent_id,omitempty"`
	Children []WorkbenchTreeNode `json:"children,omitempty"`
}

// GetWorkbenchStructure GET /api/workbench/structure
// Returns two roots: "项目集/项目/迭代" and "产品线/产品".
func GetWorkbenchStructure(c *gin.Context) {
	// Scope enforcement: same rule as workbench list endpoints (must be logged in).
	if GetCurrentUser(c) == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var programs []models.LocalProgram
	var projects []models.LocalProject
	var execs []models.LocalExecution
	var lines []models.LocalProductLine
	var products []models.LocalProduct

	db.PG.Where("deleted = false").Find(&programs)
	db.PG.Where("deleted = false").Find(&projects)
	db.PG.Where("deleted = false").Find(&execs)
	db.PG.Where("deleted = false").Find(&lines)
	db.PG.Where("deleted = false").Find(&products)

	// Build program -> projects -> executions
	progNode := func(p models.LocalProgram) WorkbenchTreeNode {
		title := strings.TrimSpace(p.Name)
		if title == "" {
			title = "未命名项目集"
		}
		return WorkbenchTreeNode{
			Key:      "program:" + itoa64(p.ID),
			Type:     "program",
			ID:       p.ID,
			Title:    title,
			ParentID: p.ParentID,
		}
	}
	projNode := func(p models.LocalProject) WorkbenchTreeNode {
		title := strings.TrimSpace(p.Name)
		if title == "" {
			title = "未命名项目"
		}
		return WorkbenchTreeNode{
			Key:      "project:" + itoa64(p.ID),
			Type:     "project",
			ID:       p.ID,
			Title:    title,
			ParentID: p.ParentID,
		}
	}
	execNode := func(e models.LocalExecution) WorkbenchTreeNode {
		title := strings.TrimSpace(e.Name)
		if title == "" {
			title = "未命名迭代"
		}
		return WorkbenchTreeNode{
			Key:      "execution:" + itoa64(e.ID),
			Type:     "execution",
			ID:       e.ID,
			Title:    title,
			ParentID: e.ParentID,
		}
	}

	progByID := map[int64]WorkbenchTreeNode{}
	for _, p := range programs {
		n := progNode(p)
		progByID[p.ID] = n
	}

	projectsByProgram := map[int64][]WorkbenchTreeNode{}
	orphProjects := []WorkbenchTreeNode{}
	for _, p := range projects {
		n := projNode(p)
		if n.ParentID != nil && *n.ParentID > 0 {
			projectsByProgram[*n.ParentID] = append(projectsByProgram[*n.ParentID], n)
		} else {
			orphProjects = append(orphProjects, n)
		}
	}

	execsByProject := map[int64][]WorkbenchTreeNode{}
	orphExecs := []WorkbenchTreeNode{}
	for _, e := range execs {
		n := execNode(e)
		if n.ParentID != nil && *n.ParentID > 0 {
			execsByProject[*n.ParentID] = append(execsByProject[*n.ParentID], n)
		} else {
			orphExecs = append(orphExecs, n)
		}
	}

	// Attach execs to projects
	for progID, plist := range projectsByProgram {
		for i := range plist {
			pid := plist[i].ID
			plist[i].Children = append(plist[i].Children, execsByProject[pid]...)
			sort.Slice(plist[i].Children, func(a, b int) bool { return plist[i].Children[a].Title < plist[i].Children[b].Title })
		}
		sort.Slice(plist, func(a, b int) bool { return plist[a].Title < plist[b].Title })
		projectsByProgram[progID] = plist
	}
	for i := range orphProjects {
		pid := orphProjects[i].ID
		orphProjects[i].Children = append(orphProjects[i].Children, execsByProject[pid]...)
		sort.Slice(orphProjects[i].Children, func(a, b int) bool { return orphProjects[i].Children[a].Title < orphProjects[i].Children[b].Title })
	}
	sort.Slice(orphProjects, func(a, b int) bool { return orphProjects[a].Title < orphProjects[b].Title })
	sort.Slice(orphExecs, func(a, b int) bool { return orphExecs[a].Title < orphExecs[b].Title })

	// Compose program roots (ignore nested program-of-program for now, keep flat)
	progRoots := make([]WorkbenchTreeNode, 0, len(programs)+1)
	for _, p := range programs {
		n := progByID[p.ID]
		n.Children = append(n.Children, projectsByProgram[p.ID]...)
		sort.Slice(n.Children, func(a, b int) bool { return n.Children[a].Title < n.Children[b].Title })
		progRoots = append(progRoots, n)
	}
	sort.Slice(progRoots, func(a, b int) bool { return progRoots[a].Title < progRoots[b].Title })

	projectRoot := WorkbenchTreeNode{
		Key:   "root:projects",
		Type:  "root",
		ID:    0,
		Title: "项目/迭代",
		Children: append(
			append(progRoots, orphProjects...),
			orphExecs...,
		),
	}

	// Build product line -> products
	lineNode := func(l models.LocalProductLine) WorkbenchTreeNode {
		title := strings.TrimSpace(l.Name)
		if title == "" {
			title = "未命名产品线"
		}
		return WorkbenchTreeNode{
			Key:      "product_line:" + itoa64(l.ID),
			Type:     "product_line",
			ID:       l.ID,
			Title:    title,
			ParentID: l.ParentID,
		}
	}
	productNode := func(p models.LocalProduct) WorkbenchTreeNode {
		title := strings.TrimSpace(p.Name)
		if title == "" {
			title = "未命名产品"
		}
		return WorkbenchTreeNode{
			Key:   "product:" + itoa64(p.ID),
			Type:  "product",
			ID:    p.ID,
			Title: title,
		}
	}

	linesByID := map[int64]WorkbenchTreeNode{}
	for _, l := range lines {
		linesByID[l.ID] = lineNode(l)
	}
	productsByLine := map[int64][]WorkbenchTreeNode{}
	orphProducts := []WorkbenchTreeNode{}
	for _, p := range products {
		n := productNode(p)
		if p.LineID != nil && *p.LineID > 0 {
			productsByLine[*p.LineID] = append(productsByLine[*p.LineID], n)
		} else {
			orphProducts = append(orphProducts, n)
		}
	}
	for id, list := range productsByLine {
		sort.Slice(list, func(a, b int) bool { return list[a].Title < list[b].Title })
		productsByLine[id] = list
	}
	sort.Slice(orphProducts, func(a, b int) bool { return orphProducts[a].Title < orphProducts[b].Title })

	lineRoots := make([]WorkbenchTreeNode, 0, len(linesByID))
	for _, l := range lines {
		n := linesByID[l.ID]
		n.Children = append(n.Children, productsByLine[l.ID]...)
		lineRoots = append(lineRoots, n)
	}
	sort.Slice(lineRoots, func(a, b int) bool { return lineRoots[a].Title < lineRoots[b].Title })

	productRoot := WorkbenchTreeNode{
		Key:      "root:products",
		Type:     "root",
		ID:       0,
		Title:    "产品线/产品",
		Children: append(lineRoots, orphProducts...),
	}

	c.JSON(http.StatusOK, gin.H{
		"roots": []WorkbenchTreeNode{projectRoot, productRoot},
	})
}

func itoa64(v int64) string {
	// avoid importing strconv all over the file by isolating it here
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [32]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v%10]
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
