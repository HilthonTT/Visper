package repository

import (
	"context"
	"errors"

	"github.com/hilthontt/visper/api/domain/filter"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/hilthontt/visper/api/infrastructure/persistence/database"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrRecordNotFound   = errors.New("record not found")
	ErrPermissionDenied = errors.New("permission denied")
)

const softDeleteExp string = "id = ? and deleted_by is null"

// BaseRepository provides generic GORM helpers for Postgres-backed entities.
// GetByID and GetByFilter assume soft-deletable entities (see model.BaseModel).
type BaseRepository[TEntity any] struct {
	database *gorm.DB
	logger   *logger.GormZapLogger
	preloads []string
}

func NewBaseRepository[TEntity any](preloads []string, zapLogger *zap.Logger) *BaseRepository[TEntity] {
	return &BaseRepository[TEntity]{
		database: database.GetDb(),
		logger:   logger.NewGormLogger(zapLogger),
		preloads: preloads,
	}
}

func (r BaseRepository[TEntity]) Create(ctx context.Context, entity TEntity) (TEntity, error) {
	err := r.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&entity).Error
	})
	if err != nil {
		r.logger.Error(ctx, "create failed: %v", err)
		return entity, err
	}

	return entity, nil
}

func (r BaseRepository[TEntity]) GetByID(ctx context.Context, id int) (TEntity, error) {
	model := new(TEntity)
	db := database.Preload(r.database.WithContext(ctx), r.preloads)

	err := db.
		Where(softDeleteExp, id).
		First(model).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrRecordNotFound
	}

	return *model, err
}

func (r BaseRepository[TEntity]) GetByFilter(ctx context.Context, req filter.PaginationInputWithFilter) (int64, *[]TEntity, error) {
	model := new(TEntity)
	items := []TEntity{}

	db := database.Preload(r.database.WithContext(ctx), r.preloads)
	query, args := database.GenerateDynamicQuery[TEntity](&req.DynamicFilter)
	sort := database.GenerateDynamicSort[TEntity](&req.DynamicFilter)

	var totalRows int64
	if err := db.Model(model).Where(query, args...).Count(&totalRows).Error; err != nil {
		return 0, &items, err
	}

	q := db.
		Where(query, args...).
		Offset(req.GetOffset()).
		Limit(req.GetPageSize())
	if sort != "" {
		q = q.Order(sort)
	}

	if err := q.Find(&items).Error; err != nil {
		return 0, &[]TEntity{}, err
	}

	return totalRows, &items, nil
}
