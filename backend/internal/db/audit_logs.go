package db

import (
	"time"
	"zenmind/internal/models"
)

type AuditInput struct {
	ActorUserID   *int64
	ActorUsername string
	Action        string
	TargetType    string
	TargetID      string
	Metadata      models.JSONB
	IP            string
	UA            string
}

func WriteAudit(in AuditInput) error {
	row := models.AuditLog{
		ActorUserID:   in.ActorUserID,
		ActorUsername: in.ActorUsername,
		Action:        in.Action,
		TargetType:    in.TargetType,
		TargetID:      in.TargetID,
		Metadata:      in.Metadata,
		IP:            in.IP,
		UA:            in.UA,
		CreatedAt:     time.Now(),
	}
	return PG.Create(&row).Error
}
