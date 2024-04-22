package queryer

import (
	"sync"
	"testing"
	"time"
)

func Test_runtimeQuery_GoroutineCount(t *testing.T) {
	r := newRuntimeQuery()

	initGoroutineCnt := r.GoroutineCount()
	if initGoroutineCnt < 1 {
		t.Errorf("GoroutineCount() = %d; want is > 0", initGoroutineCnt)
	}

	wg := sync.WaitGroup{}

	goroutineCnt := 1000
	for i := 0; i < goroutineCnt; i++ {
		wg.Add(1)
		go func() {
			time.Sleep(500 * time.Millisecond)
			wg.Done()
		}()
	}

	addedGoroutineCnt := r.GoroutineCount()
	if addedGoroutineCnt != initGoroutineCnt+goroutineCnt {
		t.Errorf("GoroutineCount() = %d; want is %d", addedGoroutineCnt, initGoroutineCnt+1)
	}

	wg.Wait()

	endGoroutineCnt := r.GoroutineCount()
	if endGoroutineCnt != initGoroutineCnt {
		t.Errorf("GoroutineCount() = %d; want is %d", endGoroutineCnt, initGoroutineCnt)
	}
}
