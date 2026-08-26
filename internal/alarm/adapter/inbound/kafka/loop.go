package kafka

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

type consumerMetrics interface {
	IncConsumerMessages(source string)
	IncConsumerHandlerErrors(source string)
	SetConsumerLag(source string, lag int64)
}

// runReader fetches one message at a time. Poison/drop commit and move on.
// Transient failures retry the SAME message (do not Fetch the next offset).
func runReader(
	ctx context.Context,
	reader *kafka.Reader,
	component string,
	source string,
	m consumerMetrics,
	handle func(context.Context, kafka.Message) Class,
) error {
	if reader == nil {
		<-ctx.Done()
		return nil
	}

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if m != nil {
				m.IncConsumerHandlerErrors(source)
			}
			logging.Warnf(ctx, logrus.Fields{
				"component": component,
				"error":     err.Error(),
			}, "kafka: fetch message failed, retrying")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if m != nil {
			m.IncConsumerMessages(source)
			m.SetConsumerLag(source, reader.Stats().Lag)
		}

		for {
			class := handle(ctx, msg)
			if !class.Commit {
				if m != nil {
					m.IncConsumerHandlerErrors(source)
				}
				logging.Warnf(ctx, logrus.Fields{
					"component": component,
					"topic":     msg.Topic,
					"partition": msg.Partition,
					"offset":    msg.Offset,
					"result":    class.Result,
					"reason":    class.Reason,
				}, "kafka: transient ingest, retrying same message")
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(2 * time.Second):
				}
				continue
			}
			if err := reader.CommitMessages(ctx, msg); err != nil {
				if m != nil {
					m.IncConsumerHandlerErrors(source)
				}
				logging.Warnf(ctx, logrus.Fields{
					"component": component,
					"error":     err.Error(),
				}, "kafka: commit failed — message may be reprocessed")
			}
			break
		}
	}
}
