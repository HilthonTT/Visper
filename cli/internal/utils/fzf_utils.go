package utils

import (
	"time"

	"github.com/reinhrst/fzf-lib"
)

func FzfSearch(query string, source []string, resultChan chan<- []fzf.MatchResult, done chan<- bool) {
	go func() {
		defer close(resultChan)
		defer close(done)

		fzfSearcher := fzf.New(source, fzf.DefaultOptions())
		fzfSearcher.Search(query)

		select {
		case results := <-fzfSearcher.GetResultChannel():
			resultChan <- results.Matches
		case <-time.After(500 * time.Millisecond):
			return
		}
		fzfSearcher.End()
	}()
}
