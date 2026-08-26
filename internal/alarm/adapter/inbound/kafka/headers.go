package kafka

import (
	"github.com/segmentio/kafka-go"

	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

func kafkaHeadersToCarrier(headers []kafka.Header) plattelemetry.MapCarrier {
	if len(headers) == 0 {
		return nil
	}
	c := make(plattelemetry.MapCarrier, len(headers))
	for _, h := range headers {
		c[h.Key] = string(h.Value)
	}
	return c
}
