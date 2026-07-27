package errors

import (
	"errors"
	"testing"

	"github.com/mushroomyuan/vpp-backend/platform/consts"
)

func TestErrno(t *testing.T) {
	t.Parallel()
	if Errno(nil) != consts.ErrnoSuccess {
		t.Fatal("nil")
	}
	if Errno(errors.New("x")) != -1 {
		t.Fatal("plain")
	}
	e := NewWithErr(consts.ErrnoBindRequestError, errors.New("bad json"))
	if Errno(e) != consts.ErrnoBindRequestError {
		t.Fatalf("got %d", Errno(e))
	}
	wrapped := errors.Join(errors.New("wrap"), e)
	if Errno(wrapped) != consts.ErrnoBindRequestError {
		t.Fatalf("unwrap got %d", Errno(wrapped))
	}
}

func TestOutput(t *testing.T) {
	t.Parallel()
	code, msg := Output(nil)
	if code != consts.ErrnoSuccess || msg != consts.ErrMsg[consts.ErrnoSuccess] {
		t.Fatalf("%d %q", code, msg)
	}
	code, msg = Output(errors.New("boom"))
	if code != consts.ErrnoUnknownError || msg != "boom" {
		t.Fatalf("%d %q", code, msg)
	}
	e := NewWithErr(consts.ErrnoBindRequestError, errors.New("detail"))
	code, msg = Output(e)
	if code != consts.ErrnoBindRequestError || msg == "" {
		t.Fatalf("%d %q", code, msg)
	}
}

func TestError_UsesErrMsg(t *testing.T) {
	t.Parallel()
	e := NewWithErr(consts.ErrnoBindRequestError, errors.New("detail"))
	got := e.Error()
	if got != consts.ErrMsg[consts.ErrnoBindRequestError]+"->detail" {
		t.Fatalf("got %q", got)
	}
}
