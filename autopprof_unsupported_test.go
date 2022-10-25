//go:build !linux
// +build !linux

package autopprof

import (
	"errors"
	"testing"
)

func TestStart(t *testing.T) {
	testCases := []struct {
		name string
		opt  *Option
		want error
	}{
		{
			name: "unsupported platform",
			opt:  nil,
			want: ErrUnsupportedPlatform,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Start(tc.opt); errors.Is(err, tc.want) {
				t.Errorf("Start() = %v, want %v", err, tc.want)
			}
		})
	}
}
