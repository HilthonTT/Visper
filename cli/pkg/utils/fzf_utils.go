package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/reinhrst/fzf-lib"
)

func FzfSearch(query string, source []string, timeout time.Duration) ([]fzf.MatchResult, error) {
	if timeout == 0 {
		timeout = 500 * time.Millisecond
	}

	fzfSearcher := fzf.New(source, fzf.DefaultOptions())
	defer fzfSearcher.End()

	fzfSearcher.Search(query)

	select {
	case fzfResults := <-fzfSearcher.GetResultChannel():
		return fzfResults.Matches, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("search timed out after: %v", timeout)
	}
}

func FzfSearchAsync(ctx context.Context, query string, source []string) <-chan fzf.SearchResult {
	resultChan := make(chan fzf.SearchResult, 1)

	go func() {
		defer close(resultChan)

		fzfSearcher := fzf.New(source, fzf.DefaultOptions())
		defer fzfSearcher.End()

		select {
		case fzfResults := <-fzfSearcher.GetResultChannel():
			resultChan <- fzf.SearchResult{Matches: fzfResults.Matches}
		case <-ctx.Done():
			resultChan <- fzf.SearchResult{}
		}
	}()

	return resultChan
}
