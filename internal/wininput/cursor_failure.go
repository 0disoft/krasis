package wininput

import "github.com/0disoft/krasis/internal/application"

func cursorTransactionFailure(operation, stage string, err error) error {
	if err == nil {
		return nil
	}
	return &application.CursorTransactionFailure{Operation: operation, Stage: stage, Cause: err}
}
