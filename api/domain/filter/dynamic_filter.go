package filter

import "fmt"

// SortDirection represents valid sorting directions
type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

// Sort defines column sorting configuration
type Sort struct {
	ColID string        `json:"colId"`
	Sort  SortDirection `json:"sort"`
}

// FilterType represents the type of filtering operation
type FilterType string

const (
	FilterContains           FilterType = "contains"
	FilterNotContains        FilterType = "notContains"
	FilterEquals             FilterType = "equals"
	FilterNotEqual           FilterType = "notEqual"
	FilterStartsWith         FilterType = "startsWith"
	FilterEndsWith           FilterType = "endsWith"
	FilterLessThan           FilterType = "lessThan"
	FilterLessThanOrEqual    FilterType = "lessThanOrEqual"
	FilterGreaterThan        FilterType = "greaterThan"
	FilterGreaterThanOrEqual FilterType = "greaterThanOrEqual"
	FilterInRange            FilterType = "inRange"
)

// DataType represents the data type being filtered
type DataType string

const (
	DataTypeText   DataType = "text"
	DataTypeNumber DataType = "number"
	DataTypeDate   DataType = "date"
)

// Filter defines filtering criteria for a column
type Filter struct {
	Type       FilterType `json:"type"`
	FilterType DataType   `json:"filterType"`
	From       string     `json:"from,omitempty"`
	To         string     `json:"to,omitempty"`
}

// DynamicFilter encapsulates sorting and filtering configuration
type DynamicFilter struct {
	Sort   []Sort            `json:"sort,omitempty"`
	Filter map[string]Filter `json:"filter,omitempty"`
}

// Validate checks if the filter configuration is valid
func (f *Filter) Validate() error {
	if f.Type == FilterInRange && (f.From == "" || f.To == "") {
		return fmt.Errorf("inRange filter requires both 'from' and 'to' values")
	}
	return nil
}

// HasFilters returns true if any filters are applied
func (df *DynamicFilter) HasFilters() bool {
	return len(df.Filter) > 0
}

// HasSort returns true if sorting is configured
func (df *DynamicFilter) HasSort() bool {
	return len(df.Sort) > 0
}
