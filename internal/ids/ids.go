package ids

import (
	mathrand "math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(mathrand.New(mathrand.NewSource(time.Now().UnixNano())), 0)
)

// New returns a lexicographically sortable identifier suitable for storage keys.
func New() string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
