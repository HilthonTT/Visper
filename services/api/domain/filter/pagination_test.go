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
