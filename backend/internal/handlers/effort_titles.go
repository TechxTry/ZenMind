package handlers

import (
	"strings"

	"zenmind/internal/db"
	"zenmind/internal/models"
)

type effortWithTitle struct {
	models.LocalEffort
	ObjectTitle string `json:"object_title,omitempty"`
}

func effortsWithTitles(rows []models.LocalEffort) []effortWithTitle {
	names := lookupTaskNamesForEfforts(rows)
	out := make([]effortWithTitle, len(rows))
	for i, r := range rows {
		out[i] = effortWithTitle{LocalEffort: r}
		if strings.EqualFold(strings.TrimSpace(r.ObjectType), "task") && r.ObjectID > 0 {
			if n, ok := names[r.ObjectID]; ok {
				out[i].ObjectTitle = n
			}
		}
	}
	return out
}

func lookupTaskNamesForEfforts(efforts []models.LocalEffort) map[int64]string {
	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for _, e := range efforts {
		if !strings.EqualFold(strings.TrimSpace(e.ObjectType), "task") || e.ObjectID <= 0 {
			continue
		}
		if _, ok := seen[e.ObjectID]; ok {
			continue
		}
		seen[e.ObjectID] = struct{}{}
		ids = append(ids, e.ObjectID)
	}
	if len(ids) == 0 {
		return nil
	}
	var tasks []models.LocalTask
	if err := db.PG.Model(&models.LocalTask{}).
		Select("id, name").
		Where("deleted = false AND id IN ?", ids).
		Find(&tasks).Error; err != nil {
		return nil
	}
	names := make(map[int64]string, len(tasks))
	for _, t := range tasks {
		names[t.ID] = t.Name
	}
	return names
}
