package filter

import (
	"math"

	"github.com/hilthontt/visper/api/infrastructure/common"
)

const (
	DefaultPageSize = 10
	MaxPageSize     = 100
)

type PagedList[T any] struct {
	PageNumber      int   `json:"pageNumber"`
	PageSize        int64 `json:"pageSize"`
	TotalRows       int64 `json:"totalRows"`
	TotalPages      int   `json:"totalPages"`
	HasPreviousPage bool  `json:"hasPreviousPage"`
	HasNextPage     bool  `json:"hasNextPage"`
	Items           *[]T  `json:"items"`
}

type PaginationInput struct {
	PageSize   int `json:"pageSize"`
	PageNumber int `json:"pageNumber"`
}

type PaginationInputWithFilter struct {
	PaginationInput
	DynamicFilter
}

func NewPagedList[T any](items *[]T, count int64, pageNumber int, pageSize int64) *PagedList[T] {
	pl := &PagedList[T]{
		PageNumber: pageNumber,
		PageSize:   pageSize,
		TotalRows:  count,
		Items:      items,
	}

	if pageSize > 0 {
		pl.TotalPages = int(math.Ceil(float64(count) / float64(pageSize)))
	}
	pl.HasNextPage = pl.PageNumber < pl.TotalPages
	pl.HasPreviousPage = pl.PageNumber > 1

	return pl
}

func Paginate[TInput any, TOutput any](totalRows int64, items *[]TInput, pageNumber int, pageSize int64) (*PagedList[TOutput], error) {
	var rItems []TOutput

	rItems, err := common.TypeConverter[[]TOutput](items)
	if err != nil {
		return nil, err
	}

	return NewPagedList(&rItems, totalRows, pageNumber, pageSize), err
}

// GetOffset returns the number of rows to skip. Pages are 1-based:
// page 2 with size 10 covers rows 11-20 (offset 10).
func (p *PaginationInputWithFilter) GetOffset() int {
	return (p.GetPageNumber() - 1) * p.GetPageSize()
}

// GetPageSize returns the page size, defaulting non-positive values to
// DefaultPageSize and capping it at MaxPageSize.
func (p *PaginationInputWithFilter) GetPageSize() int {
	switch {
	case p.PageSize <= 0:
		p.PageSize = DefaultPageSize
	case p.PageSize > MaxPageSize:
		p.PageSize = MaxPageSize
	}
	return p.PageSize
}

// GetPageNumber returns the 1-based page number; non-positive values (which
// would produce a negative offset) are treated as the first page.
func (p *PaginationInputWithFilter) GetPageNumber() int {
	if p.PageNumber <= 0 {
		p.PageNumber = 1
	}
	return p.PageNumber
}
