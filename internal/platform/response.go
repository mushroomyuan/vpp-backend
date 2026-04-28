package pkg

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/handler/errors"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

type BaseResponse struct{}

type response struct {
	Errno   int    `json:"errno"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	TraceID string `json:"trace_id"`
}

func (base *BaseResponse) Response(ctx *gin.Context, err error, data any) {
	if err != nil {
		base.error(ctx, err)
	} else {
		base.success(ctx, data)
	}
}

func (base *BaseResponse) success(ctx *gin.Context, data any) {
	errno, errmsg := errors.Output(nil)
	r := response{
		Errno:   errno,
		Message: errmsg,
		Data:    data,
		TraceID: telemetry.TraceID(ctx.Request.Context()),
	}

	resp, _ := json.Marshal(r)
	ctx.Set("response", string(resp))
	ctx.JSON(http.StatusOK, r)
}

func (base *BaseResponse) error(ctx *gin.Context, err error) {
	errno, errmsg := errors.Output(err)
	r := response{
		Errno:   errno,
		Message: errmsg,
		Data:    nil,
		TraceID: telemetry.TraceID(ctx.Request.Context()),
	}

	resp, _ := json.Marshal(r)
	ctx.Set("response", string(resp))
	ctx.JSON(http.StatusOK, r)
}
