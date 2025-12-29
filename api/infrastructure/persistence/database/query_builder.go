package database

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hilthontt/visper/api/domain/filter"
	"github.com/hilthontt/visper/api/infrastructure/common"
	"gorm.io/gorm"
)

type QueryBuilder struct {
	Conditions []string
	Args       []any
}

func (qb *QueryBuilder) Add(condition string, args ...any) {
	qb.Conditions = append(qb.Conditions, condition)
	qb.Args = append(qb.Args, args...)
}

func (qb *QueryBuilder) Build() (string, []any) {
	if len(qb.Conditions) == 0 {
		return "", nil
	}
	return strings.Join(qb.Conditions, " AND "), qb.Args
}

func GenerateDynamicQuery[T any](f *filter.DynamicFilter) (string, []any) {
	qb := &QueryBuilder{}
	qb.Add("deleted_by IS NULL")

	if f == nil || len(f.Filter) == 0 {
		return qb.Build()
	}

	t := new(T)
	typeT := reflect.TypeOf(*t)

	for fieldName, filterCfg := range f.Filter {
		fld, ok := typeT.FieldByName(fieldName)
		if !ok {
			continue
		}

		condition, args := generateFilterCondition(fld, filterCfg)
		if condition != "" {
			qb.Add(condition, args...)
		}
	}

	return qb.Build()
}

// generateFilterCondition creates a single filter condition with parameters
func generateFilterCondition(fld reflect.StructField, f filter.Filter) (string, []any) {
	columnName := common.ToSnakeCase(fld.Name)

	switch f.Type {
	case filter.FilterContains:
		return fmt.Sprintf("%s ILIKE ?", columnName), []any{"%" + f.From + "%"}

	case filter.FilterNotContains:
		return fmt.Sprintf("%s NOT ILIKE ?", columnName), []any{"%" + f.From + "%"}

	case filter.FilterStartsWith:
		return fmt.Sprintf("%s ILIKE ?", columnName), []any{f.From + "%"}

	case filter.FilterEndsWith:
		return fmt.Sprintf("%s ILIKE ?", columnName), []any{"%" + f.From}

	case filter.FilterEquals:
		return fmt.Sprintf("%s = ?", columnName), []any{f.From}

	case filter.FilterNotEqual:
		return fmt.Sprintf("%s != ?", columnName), []any{f.From}

	case filter.FilterLessThan:
		return fmt.Sprintf("%s < ?", columnName), []any{f.From}

	case filter.FilterLessThanOrEqual:
		return fmt.Sprintf("%s <= ?", columnName), []any{f.From}

	case filter.FilterGreaterThan:
		return fmt.Sprintf("%s > ?", columnName), []any{f.From}

	case filter.FilterGreaterThanOrEqual:
		return fmt.Sprintf("%s >= ?", columnName), []any{f.From}

	case filter.FilterInRange:
		return fmt.Sprintf("%s BETWEEN ? AND ?", columnName), []any{f.From, f.To}

	default:
		return "", nil
	}
}

// GenerateDynamicSort creates ORDER BY clause
func GenerateDynamicSort[T any](f *filter.DynamicFilter) string {
	if f == nil || len(f.Sort) == 0 {
		return ""
	}

	t := new(T)
	typeT := reflect.TypeOf(*t)
	sortClauses := make([]string, 0, len(f.Sort))

	for _, sortCfg := range f.Sort {
		fld, ok := typeT.FieldByName(sortCfg.ColID)
		if !ok {
			continue
		}

		// Validate sort direction
		direction := strings.ToUpper(string(sortCfg.Sort))
		if direction != "ASC" && direction != "DESC" {
			continue
		}

		columnName := common.ToSnakeCase(fld.Name)
		sortClauses = append(sortClauses, fmt.Sprintf("%s %s", columnName, direction))
	}

	return strings.Join(sortClauses, ", ")
}

// ApplyDynamicFilter applies filtering and sorting to a GORM query
func ApplyDynamicFilter[T any](db *gorm.DB, f *filter.DynamicFilter) *gorm.DB {
	// Apply WHERE conditions
	if whereClause, args := GenerateDynamicQuery[T](f); whereClause != "" {
		db = db.Where(whereClause, args...)
	}

	// Apply ORDER BY
	if orderClause := GenerateDynamicSort[T](f); orderClause != "" {
		db = db.Order(orderClause)
	}

	return db
}

// Preload applies multiple preload clauses to a GORM query
func Preload(db *gorm.DB, preloads []string) *gorm.DB {
	for _, entity := range preloads {
		if entity != "" {
			db = db.Preload(entity)
		}
	}
	return db
}

// PreloadWithConditions applies preloads with custom conditions
func PreloadWithConditions(db *gorm.DB, preloads map[string]func(*gorm.DB) *gorm.DB) *gorm.DB {
	for entity, condition := range preloads {
		if condition != nil {
			db = db.Preload(entity, condition)
		} else {
			db = db.Preload(entity)
		}
	}
	return db
}

func SafeColumnName(fieldName string, modelType reflect.Type) (string, bool) {
	fld, ok := modelType.FieldByName(fieldName)
	if !ok {
		return "", false
	}

	if tag := fld.Tag.Get("gorm"); tag != "" {
		for part := range strings.SplitSeq(tag, ";") {
			if after, ok0 := strings.CutPrefix(part, "column:"); ok0 {
				return after, true
			}
		}
	}

	return common.ToSnakeCase(fld.Name), true
}
