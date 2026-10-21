package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

type PostgresAuditLogRepository struct {
	*BaseRepository[model.AuditLog]
}

func NewAuditLogRepository(zapLogger *zap.Logger) repository.AuditLogRepository {
	return &PostgresAuditLogRepository{
		BaseRepository: NewBaseRepository[model.AuditLog](nil, zapLogger),
	}
}

// CreateAuditLog inserts the entry; an entry whose event_id already exists is
// silently skipped so that replayed events are not duplicated.
func (r *PostgresAuditLogRepository) CreateAuditLog(ctx context.Context, a model.AuditLog) (model.AuditLog, error) {
	result := r.database.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_id"}},
			DoNothing: true,
		}).
		Create(&a)

	if result.Error != nil {
		r.logger.Error(ctx, "create audit log failed: %v", result.Error)
		return a, result.Error
	}

	return a, nil
}
