//go:build !linux
// +build !linux

package autopprof

// Start does not do anything on unsupported platforms.
func Start(opt *Option) error {
	return ErrUnsupportedPlatform
}

// Stop does not do anything on unsupported platforms.
func Stop() {}
