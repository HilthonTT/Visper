package generator

import (
	"crypto/rand"
	"math/big"
	"strings"
)

type CustomGenerator struct {
	wordLists [][]string
	separator string
}

func NewCustomGenerator(wordLists [][]string, separator string) *CustomGenerator {
	return &CustomGenerator{
		wordLists: wordLists,
		separator: separator,
	}
}

func (cg *CustomGenerator) Generate() string {
	parts := make([]string, len(cg.wordLists))
	for i, list := range cg.wordLists {
		if len(list) > 0 {
			idx := cg.secureRandom(len(list))
			parts[i] = list[idx]
		}
	}

	return strings.Join(parts, cg.separator)
}

func (cg *CustomGenerator) secureRandom(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
