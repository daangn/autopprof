package autopprof

import "time"

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

	// The maximum number of elements that the queue can hold.
	cap() int
	// The number of elements that the queue holds.
	len() int
}

type cpuUsageSnapshot struct {
	// The CPU usage of the process at the time of the snapshot.
	usage uint64
	// The time at which the snapshot was taken.
	timestamp time.Time
}

type cpuUsageSnapshotQueue struct {
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
	if q.len() == q.cap() {
		q.list[q.tailIdx] = cs
		q.tailIdx = (q.tailIdx + 1) % q.cap()
		q.headIdx = (q.headIdx + 1) % q.cap()
	} else {
		q.list = append(q.list, cs)
		q.tailIdx = (q.tailIdx + 1) % q.cap()
	}
}

func (q *cpuUsageSnapshotQueue) head() *cpuUsageSnapshot {
	if q.len() == 0 {
		return nil
	}
	return q.list[q.headIdx]
}

func (q *cpuUsageSnapshotQueue) tail() *cpuUsageSnapshot {
	if q.len() == 0 {
		return nil
	}
	baseIdx := q.tailIdx
	if baseIdx == 0 {
		baseIdx = q.cap()
	}
	return q.list[(baseIdx-1)%q.cap()]
}

func (q *cpuUsageSnapshotQueue) isFull() bool {
	return q.len() == q.cap()
}

func (q *cpuUsageSnapshotQueue) cap() int {
	return cap(q.list)
}

func (q *cpuUsageSnapshotQueue) len() int {
	return len(q.list)
}
