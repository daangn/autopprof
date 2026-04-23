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
		opt  Option
		want error
	}{
		{
			name: "unsupported platform",
			opt:  Option{},
			want: ErrUnsupportedPlatform,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Start(tc.opt); !errors.Is(err, tc.want) {
				t.Errorf("Start() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRegister_unsupportedPlatform(t *testing.T) {
	m := NewMetric("x", 1, 0,
		func() (float64, error) { return 0, nil },
		func(float64) (CollectResult, error) { return CollectResult{}, nil },
	)
	if err := Register(m); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Register() = %v, want %v", err, ErrUnsupportedPlatform)
	}
}
