package queryer

import (
	"sync"
	"time"
)

// cpuUsageSnapshotQueue is a circular queue of cpuUsageSnapshot.
// It doesn't implement dequeue() method because it's not needed.
type cpuUsageSnapshotQueuer interface {
	// Enqueue adds an element to the queue.
	// If the queue is full, the oldest element is overwritten.
	enqueue(snapshot *cpuUsageSnapshot)

	// head returns the oldest element in the queue.
	head() *cpuUsageSnapshot
	// tail returns the newest element in the queue.
	tail() *cpuUsageSnapshot

	// IsFull returns true if the queue is full.
	isFull() bool
}

type cpuUsageSnapshot struct {
	// The CPU usage of the process at the time of the snapshot.
	usage uint64
	// The time at which the snapshot was taken.
	timestamp time.Time
}

// cpuUsageSnapshotQueue is goroutine-safe: every exported method takes
// the internal mutex. Callers don't need to serialize access.
type cpuUsageSnapshotQueue struct {
	mu      sync.Mutex
	list    []*cpuUsageSnapshot
	headIdx int
	tailIdx int
}

func newCPUUsageSnapshotQueue(cap int) *cpuUsageSnapshotQueue {
	return &cpuUsageSnapshotQueue{
		list: make([]*cpuUsageSnapshot, 0, cap),
	}
}

func (q *cpuUsageSnapshotQueue) enqueue(cs *cpuUsageSnapshot) {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := cap(q.list)
	if len(q.list) == c {
		q.list[q.tailIdx] = cs
		q.tailIdx = (q.tailIdx + 1) % c
		q.headIdx = (q.headIdx + 1) % c
	} else {
		q.list = append(q.list, cs)
		q.tailIdx = (q.tailIdx + 1) % c
	}
}

func (q *cpuUsageSnapshotQueue) head() *cpuUsageSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.list) == 0 {
		return nil
	}
	return q.list[q.headIdx]
}

func (q *cpuUsageSnapshotQueue) tail() *cpuUsageSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.list) == 0 {
		return nil
	}
	c := cap(q.list)
	baseIdx := q.tailIdx
	if baseIdx == 0 {
		baseIdx = c
	}
	return q.list[(baseIdx-1)%c]
}

func (q *cpuUsageSnapshotQueue) isFull() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.list) == cap(q.list)
}
