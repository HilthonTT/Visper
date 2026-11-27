package filter

const (
	DefaultPageSize = 10
	MaxPageSize     = 100
)

type PaginationInput struct {
	PageSize   int `json:"pageSize"`
	PageNumber int `json:"pageNumber"`
}

type PaginationInputWithFilter struct {
	PaginationInput
	DynamicFilter
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
