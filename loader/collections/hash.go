package collections

import (
	"fmt"

	"github.com/OneOfOne/xxhash"
)

type Hash uint64

// Hasher is the interface by which an element in a graph exposes its key
type Hasher interface {
	fmt.Stringer
	Hash() Hash
}

// CalculateHashFromString calculates the hash of a string
func CalculateHashFromString(s string) Hash {
	h := xxhash.New64()
	h.WriteString(s)
	hash := h.Sum64()

	return Hash(hash)
}
