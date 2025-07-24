package conditions

import (
	"fmt"
	"regexp"
)

// Result represents the outcome of a condition check
type Result struct {
	Success bool
	Reason  string
}

// Checker handles success/failure condition checking
type Checker struct {
	successPattern  *regexp.Regexp
	failurePattern  *regexp.Regexp
	caseInsensitive bool
}

// NewChecker creates a new condition checker
// successPattern: regex pattern that indicates success in stdout/stderr
// failurePattern: regex pattern that indicates failure in stdout/stderr
// caseInsensitive: whether to ignore case when matching patterns
func NewChecker(successPattern, failurePattern string, caseInsensitive bool) (*Checker, error) {
	checker := &Checker{
		caseInsensitive: caseInsensitive,
	}

	// Compile success pattern if provided
	if successPattern != "" {
		var err error
		if caseInsensitive {
			checker.successPattern, err = regexp.Compile("(?i)" + successPattern)
		} else {
			checker.successPattern, err = regexp.Compile(successPattern)
		}

		if err != nil {
			return nil, fmt.Errorf("invalid success pattern: %w", err)
		}
	}

	// Compile failure pattern if provided
	if failurePattern != "" {
		var err error
		if caseInsensitive {
			checker.failurePattern, err = regexp.Compile("(?i)" + failurePattern)
		} else {
			checker.failurePattern, err = regexp.Compile(failurePattern)
		}

		if err != nil {
			return nil, fmt.Errorf("invalid failure pattern: %w", err)
		}
	}

	return checker, nil
}

// CheckSuccess determines if a command execution was successful
// It checks patterns first, then falls back to exit code
func (c *Checker) CheckSuccess(exitCode int, stdout, stderr string) Result {
	// Check failure pattern first (takes precedence)
	if c.failurePattern != nil {
		if c.failurePattern.MatchString(stdout) || c.failurePattern.MatchString(stderr) {
			return Result{
				Success: false,
				Reason:  "failure pattern matched",
			}
		}
	}

	// Check success pattern
	if c.successPattern != nil {
		if c.successPattern.MatchString(stdout) || c.successPattern.MatchString(stderr) {
			return Result{
				Success: true,
				Reason:  "success pattern matched",
			}
		}
	}

	// Fall back to exit code
	if exitCode == 0 {
		return Result{
			Success: true,
			Reason:  "exit code 0",
		}
	} else {
		return Result{
			Success: false,
			Reason:  fmt.Sprintf("exit code %d", exitCode),
		}
	}
}
