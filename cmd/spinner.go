package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
)

type stepSpinner struct {
	enabled bool
	inner   *spinner.Spinner
}

func startSpinner(message string) *stepSpinner {
	if noSpinner || !isInteractiveTTY() {
		return &stepSpinner{enabled: false}
	}

	s := spinner.New(
		spinner.CharSets[14],
		120*time.Millisecond,
		spinner.WithWriter(os.Stdout),
	)
	s.Suffix = " " + message
	s.Start()

	return &stepSpinner{enabled: true, inner: s}
}

func (s *stepSpinner) stopWithMessage(message string) {
	if s == nil {
		if message != "" {
			fmt.Println(message)
		}
		return
	}

	if s.enabled && s.inner != nil {
		s.inner.Stop()
	}

	if message != "" {
		fmt.Println(message)
	}
}

func isInteractiveTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func runStep(spinMessage, doneMessage string, fn func() error) error {
	s := startSpinner(spinMessage)
	err := fn()
	if err != nil {
		s.stopWithMessage("")
		return err
	}
	s.stopWithMessage(doneMessage)
	return nil
}
