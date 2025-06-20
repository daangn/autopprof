//go:build linux
// +build linux

package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/daangn/autopprof"
	"github.com/daangn/autopprof/report"
)

type mm struct {
	m map[int64]string
}

func main() {
	err := autopprof.Start(autopprof.Option{
		CPUThreshold: 0.8, // Default: 0.75.
		MemThreshold: 0.8, // Default: 0.75.
		Reporter: report.NewSlackReporter(
			&report.SlackReporterOption{
				App:       "YOUR_APP_NAME",
				Token:     "YOUR_TOKEN_HERE",
				ChannelID: "REPORT_CHANNEL_ID",
			},
		),
	})
	if errors.Is(err, autopprof.ErrUnsupportedPlatform) {
		// You can just skip the autopprof.
		log.Println(err)
	} else if err != nil {
		log.Fatalln(err)
	}
	defer autopprof.Stop()

	eatMemory()

	go func() {
		for {
			iterative(1000)
		}
	}()
	go func() {
		for {
			recursive(15)
		}
	}()

	for {
		fmt.Println("main")
	}
}

func eatMemory() {
	m := make(map[int64]string, 20000000)
	for i := 0; i < 20000000; i++ {
		m[int64(i)] = "eating heap memory"
	}
	_ = mm{m: m}
}

// Iterative fibonacci func implementation.
func iterative(n int) int64 {
	var a, b int64 = 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a
}

// Recursive fibonacci func implementation.
func recursive(n int) int64 {
	if n <= 1 {
		return int64(n)
	}
	return recursive(n-1) + recursive(n-2)
}
