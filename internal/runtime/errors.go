package runtime

import "errors"

type ClassifiedError struct {
	err       error
	permanent bool
}

func (e *ClassifiedError) Error() string {
	return e.err.Error()
}

func (e *ClassifiedError) Unwrap() error {
	return e.err
}

func (e *ClassifiedError) Permanent() bool {
	return e.permanent
}

func PermanentError(err error) error {
	if err == nil {
		return nil
	}
	return &ClassifiedError{
		err:       err,
		permanent: true,
	}
}

func RetryableError(err error) error {
	if err == nil {
		return nil
	}
	return &ClassifiedError{
		err: err,
	}
}

func IsPermanentError(err error) bool {
	var classified *ClassifiedError
	return errors.As(err, &classified) && classified.Permanent()
}
