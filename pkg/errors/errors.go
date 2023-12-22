package errors

import "github.com/pkg/errors"

var (
	New          = errors.New
	Errorf       = errors.Errorf
	Wrap         = errors.Wrap
	Wrapf        = errors.Wrapf
	WithStack    = errors.WithStack
	WithMessagef = errors.WithMessagef
	Cause        = errors.Cause
	Is           = errors.Is
	As           = errors.As
	Unwrap       = errors.Unwrap
)
