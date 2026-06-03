package datafetch

import (
	"sync/atomic"
	"unsafe"
)

// Store provides lock-free atomic snapshot reads and writes.
// Writers build a new Snapshot then call Swap(); readers call Current()
// which returns the latest snapshot with zero contention.
type Store struct {
	ptr unsafe.Pointer // *Snapshot
}

// NewStore creates a new empty Store.
func NewStore() *Store {
	return &Store{}
}

// Current returns the latest snapshot (nil if none has been swapped in).
func (s *Store) Current() *Snapshot {
	return (*Snapshot)(atomic.LoadPointer(&s.ptr))
}

// Swap atomically replaces the current snapshot with a new one.
func (s *Store) Swap(snap *Snapshot) {
	atomic.StorePointer(&s.ptr, unsafe.Pointer(snap))
}
