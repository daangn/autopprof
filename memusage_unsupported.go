//go:build !linux
// +build !linux

package autopprof

func memUsage() (float64, error) {
	return 0, ErrUnsupportedPlatform
}
