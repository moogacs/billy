package proxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Command       string
	Output        string
	ExitCode      int
	RawBytes      int
	CompactBytes  int
	RawTokens     int
	CompactTokens int
}

type CommandExitError struct {
	Code int
}

func (e CommandExitError) Error() string {
	return fmt.Sprintf("command failed with exit code %d", e.Code)
}

func Run(args []string) (Result, error) {
	if len(args) == 0 {
		return Result{}, fmt.Errorf("usage: billy proxy <command> [args...]")
	}
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()

	raw := string(out)
	compact := Compact(args, raw)
	if estimateTokens(compact) > estimateTokens(raw) {
		compact = raw
	}
	res := Result{
		Command:       strings.Join(args, " "),
		Output:        compact,
		RawBytes:      len(raw),
		CompactBytes:  len(compact),
		RawTokens:     estimateTokens(raw),
		CompactTokens: estimateTokens(compact),
	}

	_ = appendUsage(UsageRecord{
		Time:          time.Now(),
		Command:       res.Command,
		RawBytes:      res.RawBytes,
		CompactBytes:  res.CompactBytes,
		RawTokens:     res.RawTokens,
		CompactTokens: res.CompactTokens,
	})

	if err == nil {
		return res, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, CommandExitError{Code: res.ExitCode}
	}
	return res, err
}
