// Package ui provides terminal output helpers for the kagen CLI.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

var verboseMode atomic.Bool

// Info prints an informational message to stdout.
func Info(format string, args ...any) {
	fmt.Printf(colorCyan+"ℹ "+colorReset+format+"\n", args...)
}

// SetVerbose configures whether verbose output is emitted.
func SetVerbose(enabled bool) {
	verboseMode.Store(enabled)
}

// VerboseEnabled reports whether verbose output is enabled.
func VerboseEnabled() bool {
	return verboseMode.Load()
}

// Verbose prints an informational message only when verbose mode is enabled.
func Verbose(format string, args ...any) {
	if !VerboseEnabled() {
		return
	}

	fmt.Printf(colorCyan+"- "+colorReset+format+"\n", args...)
}

// Warn prints a warning message to stderr.
func Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, colorYellow+"⚠ "+colorReset+format+"\n", args...)
}

// Error prints an error message to stderr.
func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, colorRed+"✖ "+colorReset+format+"\n", args...)
}

// Success prints a success message to stdout.
func Success(format string, args ...any) {
	fmt.Printf(colorGreen+"✔ "+colorReset+format+"\n", args...)
}

// Header prints a bold header to stdout.
func Header(format string, args ...any) {
	fmt.Printf(colorBold+format+colorReset+"\n", args...)
}

// Prompt displays a numbered list of options and returns the user's selection.
// Returns an error if input is invalid or the reader encounters an error.
func Prompt(question string, options []string) (string, error) {
	fmt.Println(colorBold + question + colorReset)
	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt)
	}
	fmt.Print("Select [1-" + strconv.Itoa(len(options)) + "]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("reading input: %w", err)
		}
		return "", fmt.Errorf("no input received")
	}

	input := strings.TrimSpace(scanner.Text())
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(options) {
		return "", fmt.Errorf("invalid selection: %q", input)
	}

	return options[idx-1], nil
}
