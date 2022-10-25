//go:build linux
// +build linux

package main

import (
	"errors"
	"log"

	"github.com/daangn/autopprof"
	"github.com/daangn/autopprof/report"
)

func main() {
	err := autopprof.Start(&autopprof.Option{
		MemThreshold: 0.5,
		Reporter: report.NewSlackReporter(
			&report.SlackReporterOption{
				App:     "YOUR_APP_NAME",
				Token:   "YOUR_TOKEN_HERE",
				Channel: "#REPORT_CHANNEL",
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

	// Your code here.
	for {
	}
}
