package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"go.uber.org/zap"
)

type PostgresAuditLogRepository struct {
	*BaseRepository[model.AuditLog]
}

func NewAuditLogRepository(cfg *config.Config, zapLogger *zap.Logger) repository.AuditLogRepository {
	var preloads []string = []string{}
	return &PostgresAuditLogRepository{
		BaseRepository: NewBaseRepository[model.AuditLog](cfg, preloads, zapLogger),
	}
}

func (r *PostgresAuditLogRepository) CreateAuditLog(ctx context.Context, a model.AuditLog) (model.AuditLog, error) {
	tx := r.database.WithContext(ctx).Begin()
	err := tx.Create(&a).Error
	if err != nil {
		tx.Rollback()
		r.logger.Error(ctx, Rollback.Error())
		return a, nil
	}

	tx.Commit()
	return a, nil
}
