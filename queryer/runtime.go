package queryer

import "runtime/pprof"

type runtimeQuery struct {
}

func newRuntimeQuery() *runtimeQuery {
	return &runtimeQuery{}
}

func (r runtimeQuery) GoroutineCount() int {
	return pprof.Lookup("goroutine").Count()
}
