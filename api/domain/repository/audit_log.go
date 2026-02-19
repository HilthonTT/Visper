package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
)

type AuditLogRepository interface {
	CreateAuditLog(ctx context.Context, a model.AuditLog) (model.AuditLog, error)
}
