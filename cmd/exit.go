package cmd

import "fmt"

const statusFailure = 1

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func failure(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: statusFailure, Err: err}
}
