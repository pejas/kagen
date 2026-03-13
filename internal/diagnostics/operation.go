package diagnostics

import (
	"fmt"
	"sort"
	"strings"
	"time"

	kagerr "github.com/pejas/kagen/internal/errors"
)

type StepStatus string

const (
	StatusPending   StepStatus = "pending"
	StatusRunning   StepStatus = "running"
	StatusSucceeded StepStatus = "succeeded"
	StatusFailed    StepStatus = "failed"
)

type StepRecord struct {
	Name         string
	Status       StepStatus
	StartedAt    time.Time
	FinishedAt   time.Time
	Duration     time.Duration
	ErrorSummary string
	FailureClass kagerr.FailureClass
	Metadata     map[string]string
}

type Operation struct {
	Name       string
	Status     StepStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Metadata   map[string]string
	Steps      []StepRecord
}

type Reporter interface {
	StepStarted(Operation, StepRecord)
	OperationFinished(Operation)
}

type Recorder struct {
	now      func() time.Time
	reporter Reporter
	current  Operation
	indices  map[string]int
	reported bool
}

type StepContext struct {
	recorder *Recorder
	index    int
}

type StepError struct {
	Operation    string
	Step         string
	FailureClass kagerr.FailureClass
	Err          error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("%s failed at step %s: %v", e.Operation, e.Step, e.Err)
}

func (e *StepError) Unwrap() error {
	return e.Err
}

func NewRecorder(name string, plannedSteps []string, now func() time.Time, reporter Reporter) *Recorder {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	steps := make([]StepRecord, 0, len(plannedSteps))
	indices := make(map[string]int, len(plannedSteps))
	for i, name := range plannedSteps {
		steps = append(steps, StepRecord{
			Name:   name,
			Status: StatusPending,
		})
		indices[name] = i
	}

	startedAt := now()

	return &Recorder{
		now:      now,
		reporter: reporter,
		current: Operation{
			Name:      name,
			Status:    StatusRunning,
			StartedAt: startedAt,
			Metadata:  map[string]string{},
			Steps:     steps,
		},
		indices: indices,
	}
}

func (r *Recorder) AddMetadata(key, value string) {
	if r == nil {
		return
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}
	if r.current.Metadata == nil {
		r.current.Metadata = map[string]string{}
	}
	r.current.Metadata[key] = value
}

func (r *Recorder) AddMetadataMap(values map[string]string) {
	for key, value := range values {
		r.AddMetadata(key, value)
	}
}

func (r *Recorder) RunStep(name string, run func(*StepContext) error) error {
	if r == nil {
		if run == nil {
			return nil
		}
		return run(nil)
	}

	index, ok := r.indices[name]
	if !ok {
		index = len(r.current.Steps)
		r.indices[name] = index
		r.current.Steps = append(r.current.Steps, StepRecord{
			Name:   name,
			Status: StatusPending,
		})
	}

	step := &r.current.Steps[index]
	step.Status = StatusRunning
	step.StartedAt = r.now()
	step.FinishedAt = time.Time{}
	step.Duration = 0
	step.ErrorSummary = ""
	if step.Metadata == nil {
		step.Metadata = map[string]string{}
	}

	if r.reporter != nil {
		r.reporter.StepStarted(r.Snapshot(), copyStep(*step))
	}

	ctx := &StepContext{recorder: r, index: index}
	if run == nil {
		run = func(*StepContext) error { return nil }
	}

	err := run(ctx)
	finishedAt := r.now()
	step.FinishedAt = finishedAt
	if !step.StartedAt.IsZero() {
		step.Duration = finishedAt.Sub(step.StartedAt)
	}
	if err != nil {
		step.Status = StatusFailed
		step.ErrorSummary = err.Error()
		step.FailureClass = kagerr.Classify(err)
		r.current.Status = StatusFailed
		if step.FailureClass != "" {
			step.Metadata["failure_class"] = string(step.FailureClass)
		}
		return &StepError{
			Operation:    r.current.Name,
			Step:         name,
			FailureClass: step.FailureClass,
			Err:          err,
		}
	}

	step.Status = StatusSucceeded
	step.FailureClass = ""
	return nil
}

func (r *Recorder) Complete() Operation {
	if r == nil {
		return Operation{}
	}
	if r.current.FinishedAt.IsZero() {
		r.current.FinishedAt = r.now()
	}
	if !r.current.StartedAt.IsZero() {
		r.current.Duration = r.current.FinishedAt.Sub(r.current.StartedAt)
	}
	if r.current.Status != StatusFailed {
		r.current.Status = StatusSucceeded
		for _, step := range r.current.Steps {
			if step.Status == StatusRunning {
				r.current.Status = StatusRunning
				break
			}
		}
	}

	snapshot := r.Snapshot()
	if r.reporter != nil && !r.reported {
		r.reporter.OperationFinished(snapshot)
		r.reported = true
	}

	return snapshot
}

func (r *Recorder) Snapshot() Operation {
	if r == nil {
		return Operation{}
	}

	metadata := make(map[string]string, len(r.current.Metadata))
	for key, value := range r.current.Metadata {
		metadata[key] = value
	}

	steps := make([]StepRecord, 0, len(r.current.Steps))
	for _, step := range r.current.Steps {
		steps = append(steps, copyStep(step))
	}

	return Operation{
		Name:       r.current.Name,
		Status:     r.current.Status,
		StartedAt:  r.current.StartedAt,
		FinishedAt: r.current.FinishedAt,
		Duration:   r.current.Duration,
		Metadata:   metadata,
		Steps:      steps,
	}
}

func (s *StepContext) AddMetadata(key, value string) {
	if s == nil || s.recorder == nil {
		return
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}

	step := &s.recorder.current.Steps[s.index]
	if step.Metadata == nil {
		step.Metadata = map[string]string{}
	}
	step.Metadata[key] = value
}

func (s *StepContext) AddMetadataMap(values map[string]string) {
	for key, value := range values {
		s.AddMetadata(key, value)
	}
}

func FormatSummary(operation Operation) []string {
	lines := []string{
		fmt.Sprintf(
			"Runtime trace for %s: %s (%s)%s",
			operation.Name,
			operation.Status,
			formatDuration(operation.Duration),
			formatMetadata(operation.Metadata),
		),
	}

	for _, step := range operation.Steps {
		line := fmt.Sprintf("step %s: %s", step.Name, step.Status)
		switch step.Status {
		case StatusPending:
		default:
			line += " (" + formatDuration(step.Duration) + ")"
		}
		if step.FailureClass != "" {
			line += " [" + string(step.FailureClass) + "]"
		}
		if step.ErrorSummary != "" {
			line += ": " + step.ErrorSummary
		}
		line += formatMetadata(step.Metadata)
		lines = append(lines, line)
	}

	return lines
}

func copyStep(step StepRecord) StepRecord {
	metadata := make(map[string]string, len(step.Metadata))
	for key, value := range step.Metadata {
		metadata[key] = value
	}
	step.Metadata = metadata
	return step
}

func formatDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0s"
	}

	return duration.Round(time.Millisecond).String()
}

func formatMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}

	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, metadata[key]))
	}

	return " [" + strings.Join(parts, " ") + "]"
}
