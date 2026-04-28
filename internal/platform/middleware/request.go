package middleware

import (
	"bytes"
	"io"
	"time"

	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/sirupsen/logrus"
)

func RequestLog(l *logrus.Entry) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestIn(c, l)
		defer requestOut(c, l)
		c.Next()

	}
}

func requestIn(c *gin.Context, l *logrus.Entry) {
	c.Set("request_start", time.Now())
	body := c.Request.Body
	bodyBytes, _ := io.ReadAll(body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	var compactJson bytes.Buffer
	_ = json.Compact(&compactJson, bodyBytes)
	l.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"start":      time.Now(),
		logging.Args: compactJson.String(),
		"from":       c.RemoteIP(),
		"uri":        c.Request.RequestURI,
	}).Info("__request_in")

}

func requestOut(c *gin.Context, l *logrus.Entry) {
	response, _ := c.Get("response")
	start, _ := c.Get("request_start")
	startTime := start.(time.Time)
	l.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		logging.Cost:     time.Since(startTime).Milliseconds(),
		logging.Response: response,
	}).Info("__request_out")
}
