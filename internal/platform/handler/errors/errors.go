package errors

import (
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/platform/consts"
)

type Error struct {
	code int
	msg  string
	err  error
}

func (e *Error) Error() string {
	var msg string
	if e.msg != "" {
		msg = e.msg
	}
	msg = consts.ErrMsg[e.code]
	return msg + "->" + e.err.Error()
}

func New(code int) *Error {
	return &Error{
		code: code,
	}
}

func NewWithMsg(code int, msg string) *Error {
	return &Error{
		code: code,
		msg:  msg,
	}
}

func NewWithMsgf(code int, format string, args ...any) *Error {
	return &Error{
		code: code,
		msg:  fmt.Sprintf(format, args...),
	}
}

func NewWithErr(code int, err error) *Error {
	return &Error{
		code: code,
		err:  err,
	}
}

func Errno(err error) int {
	if err == nil {
		return consts.ErrnoSuccess
	}
	targetError := &Error{}
	if errors.As(err, &targetError) {
		return targetError.code
	}
	return -1
}

func Output(err error) (int, string) {
	if err == nil {
		return consts.ErrnoSuccess, consts.ErrMsg[consts.ErrnoSuccess]
	}
	errno := Errno(err)
	if errno == -1 {
		return consts.ErrnoUnknownError, err.Error()
	}
	return errno, err.Error()
}
