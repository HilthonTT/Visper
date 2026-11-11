package filter

import "testing"

func TestPaginationInput(t *testing.T) {
	tests := []struct {
		name               string
		page, size         int
		wantPage, wantSize int
		wantOffset         int
	}{
		{"defaults", 0, 0, 1, DefaultPageSize, 0},
		{"first page", 1, 10, 1, 10, 0},
		{"second page", 2, 10, 2, 10, 10},
		{"negative page", -3, 10, 1, 10, 0},
		{"negative size", 2, -5, 2, DefaultPageSize, DefaultPageSize},
		{"size capped", 2, 1000, 2, MaxPageSize, MaxPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PaginationInputWithFilter{PaginationInput: PaginationInput{PageNumber: tt.page, PageSize: tt.size}}

			if got := p.GetPageNumber(); got != tt.wantPage {
				t.Errorf("GetPageNumber() = %d, want %d", got, tt.wantPage)
			}
			if got := p.GetPageSize(); got != tt.wantSize {
				t.Errorf("GetPageSize() = %d, want %d", got, tt.wantSize)
			}
			if got := p.GetOffset(); got != tt.wantOffset {
				t.Errorf("GetOffset() = %d, want %d", got, tt.wantOffset)
			}
		})
	}
}

func TestNewPagedList(t *testing.T) {
	items := []int{1, 2, 3}

	tests := []struct {
		name               string
		count              int64
		page               int
		size               int64
		wantPages          int
		wantNext, wantPrev bool
	}{
		{"single page", 3, 1, 10, 1, false, false},
		{"exact multiple", 20, 1, 10, 2, true, false},
		{"partial last page", 21, 3, 10, 3, false, true},
		{"empty", 0, 1, 10, 0, false, false},
		{"zero page size", 5, 1, 0, 0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := NewPagedList(&items, tt.count, tt.page, tt.size)
			if pl.TotalPages != tt.wantPages {
				t.Errorf("TotalPages = %d, want %d", pl.TotalPages, tt.wantPages)
			}
			if pl.HasNextPage != tt.wantNext {
				t.Errorf("HasNextPage = %v, want %v", pl.HasNextPage, tt.wantNext)
			}
			if pl.HasPreviousPage != tt.wantPrev {
				t.Errorf("HasPreviousPage = %v, want %v", pl.HasPreviousPage, tt.wantPrev)
			}
		})
	}
}
