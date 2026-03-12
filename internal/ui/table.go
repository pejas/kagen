package ui

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Table prints a simple tabular view to stdout.
func Table(headers []string, rows [][]string) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)

	fmt.Fprintln(writer, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(writer, strings.Join(row, "\t"))
	}

	_ = writer.Flush()
}
