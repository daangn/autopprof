package autopprof

import (
	"testing"
	"time"
)

var (
	testTimestamp = time.Unix(1660000000, 0)
)

func TestCPUUsageSnapshotQueue_enqueue(t *testing.T) {
	testCases := []struct {
		name  string
		q     *cpuUsageSnapshotQueue
		count int
		want  struct {
			list    []*cpuUsageSnapshot
			headIdx int
			tailIdx int
		}
	}{
		{
			name:  "count is less than cap",
			q:     newCPUUsageSnapshotQueue(10),
			count: 5,
			want: struct {
				list    []*cpuUsageSnapshot
				headIdx int
				tailIdx int
			}{
				list: []*cpuUsageSnapshot{
					{usage: 0, timestamp: testTimestamp},
					{usage: 1, timestamp: testTimestamp},
					{usage: 2, timestamp: testTimestamp},
					{usage: 3, timestamp: testTimestamp},
					{usage: 4, timestamp: testTimestamp},
				},
				headIdx: 0,
				tailIdx: 5,
			},
		},
		{
			name:  "count is equal to cap",
			q:     newCPUUsageSnapshotQueue(10),
			count: 10,
			want: struct {
				list    []*cpuUsageSnapshot
				headIdx int
				tailIdx int
			}{
				list: []*cpuUsageSnapshot{
					{usage: 0, timestamp: testTimestamp},
					{usage: 1, timestamp: testTimestamp},
					{usage: 2, timestamp: testTimestamp},
					{usage: 3, timestamp: testTimestamp},
					{usage: 4, timestamp: testTimestamp},
					{usage: 5, timestamp: testTimestamp},
					{usage: 6, timestamp: testTimestamp},
					{usage: 7, timestamp: testTimestamp},
					{usage: 8, timestamp: testTimestamp},
					{usage: 9, timestamp: testTimestamp},
				},
				headIdx: 0,
				tailIdx: 0,
			},
		},
		{
			name:  "count is greater than cap",
			q:     newCPUUsageSnapshotQueue(10),
			count: 15,
			want: struct {
				list    []*cpuUsageSnapshot
				headIdx int
				tailIdx int
			}{
				list: []*cpuUsageSnapshot{
					{usage: 10, timestamp: testTimestamp},
					{usage: 11, timestamp: testTimestamp},
					{usage: 12, timestamp: testTimestamp},
					{usage: 13, timestamp: testTimestamp},
					{usage: 14, timestamp: testTimestamp},
					{usage: 5, timestamp: testTimestamp},
					{usage: 6, timestamp: testTimestamp},
					{usage: 7, timestamp: testTimestamp},
					{usage: 8, timestamp: testTimestamp},
					{usage: 9, timestamp: testTimestamp},
				},
				headIdx: 5,
				tailIdx: 5,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < tc.count; i++ {
				tc.q.enqueue(&cpuUsageSnapshot{
					usage:     uint64(i),
					timestamp: testTimestamp,
				})
			}
			if got := tc.q.list; !equalCPUUsageSnapshotSlice(got, tc.want.list) {
				t.Errorf("list = %v, want %v", got, tc.want.list)
			}
			if got := tc.q.headIdx; got != tc.want.headIdx {
				t.Errorf("headIdx = %v, want %v", got, tc.want.headIdx)
			}
			if got := tc.q.tailIdx; got != tc.want.tailIdx {
				t.Errorf("tailIdx = %v, want %v", got, tc.want.tailIdx)
			}
		})
	}
}

func TestCPUUsageSnapshotQueue_headAndTail(t *testing.T) {
	testCases := []struct {
		name string
		q    *cpuUsageSnapshotQueue
		want struct {
			head *cpuUsageSnapshot
			tail *cpuUsageSnapshot
		}
	}{
		{
			name: "count is less than cap",
			q: &cpuUsageSnapshotQueue{
				list: []*cpuUsageSnapshot{
					{usage: 0, timestamp: testTimestamp},
					{usage: 1, timestamp: testTimestamp},
					{usage: 2, timestamp: testTimestamp},
					{usage: 3, timestamp: testTimestamp},
					{usage: 4, timestamp: testTimestamp},
				},
				headIdx: 0,
				tailIdx: 5,
			},
			want: struct {
				head *cpuUsageSnapshot
				tail *cpuUsageSnapshot
			}{
				head: &cpuUsageSnapshot{usage: 0, timestamp: testTimestamp},
				tail: &cpuUsageSnapshot{usage: 4, timestamp: testTimestamp},
			},
		},
		{
			name: "count is equal to cap",
			q: &cpuUsageSnapshotQueue{
				list: []*cpuUsageSnapshot{
					{usage: 0, timestamp: testTimestamp},
					{usage: 1, timestamp: testTimestamp},
					{usage: 2, timestamp: testTimestamp},
					{usage: 3, timestamp: testTimestamp},
					{usage: 4, timestamp: testTimestamp},
					{usage: 5, timestamp: testTimestamp},
					{usage: 6, timestamp: testTimestamp},
					{usage: 7, timestamp: testTimestamp},
					{usage: 8, timestamp: testTimestamp},
					{usage: 9, timestamp: testTimestamp},
				},
				headIdx: 0,
				tailIdx: 0,
			},
			want: struct {
				head *cpuUsageSnapshot
				tail *cpuUsageSnapshot
			}{
				head: &cpuUsageSnapshot{usage: 0, timestamp: testTimestamp},
				tail: &cpuUsageSnapshot{usage: 9, timestamp: testTimestamp},
			},
		},
		{
			name: "count is greater than cap",
			q: &cpuUsageSnapshotQueue{
				list: []*cpuUsageSnapshot{
					{usage: 10, timestamp: testTimestamp},
					{usage: 11, timestamp: testTimestamp},
					{usage: 12, timestamp: testTimestamp},
					{usage: 13, timestamp: testTimestamp},
					{usage: 14, timestamp: testTimestamp},
					{usage: 5, timestamp: testTimestamp},
					{usage: 6, timestamp: testTimestamp},
					{usage: 7, timestamp: testTimestamp},
					{usage: 8, timestamp: testTimestamp},
					{usage: 9, timestamp: testTimestamp},
				},
				headIdx: 5,
				tailIdx: 5,
			},
			want: struct {
				head *cpuUsageSnapshot
				tail *cpuUsageSnapshot
			}{
				head: &cpuUsageSnapshot{usage: 5, timestamp: testTimestamp},
				tail: &cpuUsageSnapshot{usage: 14, timestamp: testTimestamp},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.q.head(); *got != *tc.want.head {
				t.Errorf("head() = %v, want %v", got, tc.want.head)
			}
			if got := tc.q.tail(); *got != *tc.want.tail {
				t.Errorf("tail() = %v, want %v", got, tc.want.tail)
			}
		})
	}
}

func TestCPUUsageSnapshotQueue_cap(t *testing.T) {
	testCases := []struct {
		name string
		newQ func() *cpuUsageSnapshotQueue
		want int
	}{
		{
			name: "empty",
			newQ: func() *cpuUsageSnapshotQueue {
				return newCPUUsageSnapshotQueue(10)
			},
			want: 10,
		},
		{
			name: "full",
			newQ: func() *cpuUsageSnapshotQueue {
				q := newCPUUsageSnapshotQueue(10)
				for i := 0; i < 10; i++ {
					q.enqueue(&cpuUsageSnapshot{usage: uint64(i)})
				}
				return q
			},
			want: 10,
		},
		{
			name: "partially filled",
			newQ: func() *cpuUsageSnapshotQueue {
				q := newCPUUsageSnapshotQueue(10)
				for i := 0; i < 5; i++ {
					q.enqueue(&cpuUsageSnapshot{usage: uint64(i)})
				}
				return q
			},
			want: 10,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.newQ()
			if got := q.cap(); got != tc.want {
				t.Errorf("cap() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCPUUsageSnapshotQueue_len(t *testing.T) {
	testCases := []struct {
		name string
		newQ func() *cpuUsageSnapshotQueue
		want int
	}{
		{
			name: "empty",
			newQ: func() *cpuUsageSnapshotQueue {
				return newCPUUsageSnapshotQueue(10)
			},
			want: 0,
		},
		{
			name: "full",
			newQ: func() *cpuUsageSnapshotQueue {
				q := newCPUUsageSnapshotQueue(10)
				for i := 0; i < 10; i++ {
					q.enqueue(&cpuUsageSnapshot{usage: uint64(i)})
				}
				return q
			},
			want: 10,
		},
		{
			name: "partially filled",
			newQ: func() *cpuUsageSnapshotQueue {
				q := newCPUUsageSnapshotQueue(10)
				for i := 0; i < 5; i++ {
					q.enqueue(&cpuUsageSnapshot{usage: uint64(i)})
				}
				return q
			},
			want: 5,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.newQ()
			if got := q.len(); got != tc.want {
				t.Errorf("len() = %v, want %v", got, tc.want)
			}
		})
	}
}

func equalCPUUsageSnapshotSlice(a []*cpuUsageSnapshot, b []*cpuUsageSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if *a[i] != *b[i] {
			return false
		}
	}
	return true
}
