package app

import (
	"fmt"
	"io"
)

type Env struct {
	In       io.Reader
	Out, Err io.Writer
	Code     int
	Color    string
}

func (e *Env) Fail(code int, format string, args ...any) error {
	e.Code = code
	err := fmt.Errorf(format, args...)
	fmt.Fprintln(e.Err, err.Error())
	return err
}

func (e *Env) Set(code int) { e.Code = code }
