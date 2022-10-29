# autopprof

Automatically profile the Go applications when CPU or memory utilization crosses
threshold levels.

Once you start the autopprof, the autopprof process will periodically check the CPU and
memory utilization of the Go applications. If the resource utilization crosses the
specified threshold for each type of resource, the autopprof will automatically profile
the application (heap or cpu) and report the profiling report to the specific reporter (
e.g. Slack).

| CPU Profiling Report                                       | Memory Profiling Report                                    |
|------------------------------------------------------------|------------------------------------------------------------|
| ![profiling example cpu](images/profiling_example_cpu.png) | ![profiling example mem](images/profiling_example_mem.png) |

## Installation

```bash
go get -u github.com/daangn/autopprof
```

## Usage

> If your application is running on non-linux systems, you should check the
> ErrUnsupportedPlatform error returned from `autopprof.Start()` and handle it properly.

```go
package main

import (
	"errors"
	"log"

	"github.com/daangn/autopprof"
	"github.com/daangn/autopprof/report"
)

func main() {
	err := autopprof.Start(autopprof.Option{
		CPUThreshold: 0.8, // Default: 0.75.
		MemThreshold: 0.8, // Default: 0.75.
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
}
```

> You can create the custom reporter by implementing the `report.Reporter` interface.

## License

[Apache 2.0](LICENSE)
