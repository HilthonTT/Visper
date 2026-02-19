package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
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
	result := r.database.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_id"}},
			DoNothing: true,
		}).
		Create(&a)

	if result.Error != nil {
		r.logger.Error(ctx, result.Error.Error())
		return a, result.Error
	}

	return a, nil
}
