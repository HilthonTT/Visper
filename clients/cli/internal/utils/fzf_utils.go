package utils

import (
	"fmt"
	"time"

	"github.com/reinhrst/fzf-lib"
)

// FzfSearch fuzzy-matches query against source, giving up after timeout
// (500ms if zero).
func FzfSearch(query string, source []string, timeout time.Duration) ([]fzf.MatchResult, error) {
	if timeout == 0 {
		timeout = 500 * time.Millisecond
	}

	fzfSearcher := fzf.New(source, fzf.DefaultOptions())
	defer fzfSearcher.End()

	fzfSearcher.Search(query)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case fzfResults := <-fzfSearcher.GetResultChannel():
		return fzfResults.Matches, nil
	case <-timer.C:
		return nil, fmt.Errorf("search timed out after: %v", timeout)
	}
}
