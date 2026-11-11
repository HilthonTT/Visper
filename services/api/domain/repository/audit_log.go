package repository

import (
	"context"

	"github.com/hilthontt/visper/services/api/domain/model"
)

type AuditLogRepository interface {
	CreateAuditLog(ctx context.Context, a model.AuditLog) (model.AuditLog, error)
}
