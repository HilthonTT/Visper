package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/hilthontt/visper/api/domain/filter"
	"github.com/hilthontt/visper/api/infrastructure/common"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/hilthontt/visper/api/infrastructure/persistence/database"
	"github.com/hilthontt/visper/api/presentation/middlewares"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	RecordNotFound   = errors.New("Record not found")
	PermissionDenied = errors.New("Permission denied")
	Rollback         = errors.New("Rollback")
)

const softDeleteExp string = "id = ? and deleted_by is null"

type BaseRepository[TEntity any] struct {
	database *gorm.DB
	logger   *logger.GormZapLogger
	preloads []string
}

func NewBaseRepository[TEntity any](cfg *config.Config, preloads []string, zapLogger *zap.Logger) *BaseRepository[TEntity] {
	gormLogger := logger.NewGormLogger(zapLogger)

	return &BaseRepository[TEntity]{
		database: database.GetDb(),
		logger:   gormLogger,
	}
}

func (r BaseRepository[TEntity]) Create(ctx context.Context, entity TEntity) (TEntity, error) {
	tx := r.database.WithContext(ctx).Begin()
	err := tx.Create(&entity).Error
	if err != nil {
		tx.Rollback()
		r.logger.Error(ctx, err.Error())
		return entity, nil
	}

	tx.Commit()

	return entity, nil
}

func (r BaseRepository[TEntity]) Update(ctx context.Context, id int, entity map[string]any) (TEntity, error) {
	snakeMap := map[string]any{}
	for k, v := range entity {
		snakeMap[common.ToSnakeCase(k)] = v
	}
	snakeMap["modified_by"] = &sql.NullString{String: string(ctx.Value(middlewares.UserContextKey).(string)), Valid: true}
	snakeMap["modified_at"] = sql.NullTime{Valid: true, Time: time.Now().UTC()}
	model := new(TEntity)
	tx := r.database.WithContext(ctx).Begin()

	err := tx.Model(model).
		Where(softDeleteExp, id).
		Updates(snakeMap).
		Error

	if err != nil {
		tx.Rollback()
		r.logger.Error(ctx, err.Error())
		return *model, err
	}

	tx.Commit()
	return *model, nil
}

func (r BaseRepository[TEntity]) Delete(ctx context.Context, id int) error {
	tx := r.database.WithContext(ctx).Begin()

	model := new(TEntity)

	deleteMap := map[string]any{
		"deleted_by": &sql.NullString{String: string(ctx.Value(middlewares.UserContextKey).(string)), Valid: true},
		"deleted_at": sql.NullTime{Valid: true, Time: time.Now().UTC()},
	}

	if ctx.Value(middlewares.UserContextKey) == nil {
		return PermissionDenied
	}

	count := tx.
		Model(model).
		Where(softDeleteExp, id).
		Updates(deleteMap).
		RowsAffected

	if count == 0 {
		tx.Rollback()
		r.logger.Error(ctx, "record not found")
		return RecordNotFound
	}

	tx.Commit()

	return nil
}

func (r BaseRepository[TEntity]) GetByID(ctx context.Context, id int) (TEntity, error) {
	model := new(TEntity)
	db := database.Preload(r.database, r.preloads)

	err := db.
		Where(softDeleteExp, id).
		First(model).
		Error

	return *model, err
}

func (r BaseRepository[TEntity]) GetByFilter(ctx context.Context, req filter.PaginationInputWithFilter) (int64, *[]TEntity, error) {
	model := new(TEntity)
	var items *[]TEntity

	db := database.Preload(r.database, r.preloads)
	query, _ := database.GenerateDynamicQuery[TEntity](&req.DynamicFilter)
	sort := database.GenerateDynamicSort[TEntity](&req.DynamicFilter)
	var totalRows int64 = 0

	db.
		Model(model).
		Where(query).
		Count(&totalRows)

	err := db.
		Where(query).
		Offset(req.GetOffset()).
		Limit(req.GetPageSize()).
		Order(sort).
		Find(&items).
		Error

	if err != nil {
		return 0, &[]TEntity{}, err
	}
	return totalRows, items, err
}
