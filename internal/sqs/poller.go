package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type EventType int

const (
	EventCreated EventType = iota
	EventRemoved
	EventUnknown
)

type Event struct {
	Type EventType
	Key  string
	Size int64
}

type Handler interface {
	OnObjectCreated(ctx context.Context, key string) error
	OnObjectRemoved(ctx context.Context, key string) error
}

type Poller struct {
	client   *sqs.Client
	queueURL string
	handler  Handler
}

func New(client *sqs.Client, queueURL string, handler Handler) *Poller {
	return &Poller{
		client:   client,
		queueURL: queueURL,
		handler:  handler,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	log.Printf("[sqs] starting poller on %s", p.queueURL)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[sqs] poller shutting down")
			return nil
		default:
		}

		if err := p.poll(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			log.Printf("[sqs] poll error: %v", err)
		}
	}
}

func (p *Poller) poll(ctx context.Context) error {
	result, err := p.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(p.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20,
		VisibilityTimeout:   60,
	})
	if err != nil {
		return err
	}

	if len(result.Messages) == 0 {
		return nil
	}

	for _, msg := range result.Messages {
		body := aws.ToString(msg.Body)
		if body == "" {
			continue
		}

		s3Event, isTest := p.parseBody(body)
		if isTest {
			log.Printf("[sqs] received S3 test event, ignoring")
			p.deleteMessage(ctx, msg)
			continue
		}
		if s3Event == nil {
			log.Printf("[sqs] failed to parse S3 event from message %s", aws.ToString(msg.MessageId))
			continue
		}

		for _, record := range s3Event.Records {
			evt := classify(record)
			key := decodeKey(record.S3.Object.Key)

			log.Printf("[sqs] event=%s key=%s", evtTypeName(evt.Type), key)

			var handleErr error
			switch evt.Type {
			case EventCreated:
				handleErr = p.handler.OnObjectCreated(ctx, key)
			case EventRemoved:
				handleErr = p.handler.OnObjectRemoved(ctx, key)
			default:
				log.Printf("[sqs] unknown event: %s", record.EventName)
			}

			if handleErr != nil {
				log.Printf("[sqs] error handling event %s: %v", key, handleErr)
			}
		}

		p.deleteMessage(ctx, msg)
	}

	return nil
}

func (p *Poller) parseBody(body string) (*events.S3Event, bool) {
	if strings.Contains(body, `"Event":"s3:TestEvent"`) {
		return nil, true
	}

	var s3Event events.S3Event
	if err := json.Unmarshal([]byte(body), &s3Event); err != nil {
		log.Printf("[sqs] unmarshal error: %v", err)
		return nil, false
	}
	return &s3Event, false
}

func (p *Poller) deleteMessage(ctx context.Context, msg sqstypes.Message) {
	_, err := p.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(p.queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("[sqs] delete message error: %v", err)
	}
}

func classify(record events.S3EventRecord) Event {
	switch {
	case strings.HasPrefix(record.EventName, "ObjectCreated:"):
		return Event{Type: EventCreated, Key: record.S3.Object.Key}
	case strings.HasPrefix(record.EventName, "ObjectRemoved:"):
		return Event{Type: EventRemoved, Key: record.S3.Object.Key}
	default:
		return Event{Type: EventUnknown, Key: record.S3.Object.Key}
	}
}

func evtTypeName(t EventType) string {
	switch t {
	case EventCreated:
		return "created"
	case EventRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

func decodeKey(key string) string {
	if strings.Contains(key, "%") || strings.Contains(key, "+") {
		decoded, err := url.QueryUnescape(key)
		if err == nil {
			return decoded
		}
	}
	return key
}
