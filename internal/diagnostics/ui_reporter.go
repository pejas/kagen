package diagnostics

import "github.com/pejas/kagen/internal/ui"

type uiReporter struct{}

func NewUIReporter() Reporter {
	return uiReporter{}
}

func (uiReporter) StepStarted(_ Operation, step StepRecord) {
	ui.Verbose("Step %s: running", step.Name)
}

func (uiReporter) OperationFinished(operation Operation) {
	for _, line := range FormatSummary(operation) {
		ui.Verbose("%s", line)
	}
}
