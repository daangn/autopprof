package report

// ReporterOption is the option for the reporter.
type ReporterOption struct {
	Type                ReporterType
	SlackReporterOption *SlackReporterOption
}
