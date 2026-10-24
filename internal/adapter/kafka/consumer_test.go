package kafka

import (
	"context"
	"errors"
	"fmt"
	"github.com/segmentio/kafka-go"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/requestid"
	"reflect"
	"testing"
	"time"
)

var errUnavailable = errors.New("dependency unavailable")

const commitFirst = "commit:1"
const processCreateCall = "process:create"

type fakeReader struct {
	messages       []kafka.Message
	next           int
	trace          *[]string
	fetchFailures  int
	commitFailures int
	commitCalls    int
	closeCalls     int
	closeErr       error
	cancel         context.CancelFunc
	fetch          func(context.Context) (kafka.Message, error)
}

func (r *fakeReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if r.fetch != nil {
		return r.fetch(ctx)
	}
	if r.fetchFailures > 0 {
		r.fetchFailures--
		*r.trace = append(*r.trace, "fetch-error")
		return kafka.Message{}, errUnavailable
	}
	if r.next == len(r.messages) {
		r.cancel()
		return kafka.Message{}, ctx.Err()
	}
	message := r.messages[r.next]
	r.next++
	*r.trace = append(*r.trace, fmt.Sprintf("fetch:%d", message.Offset))
	return message, nil
}
func (r *fakeReader) CommitMessages(_ context.Context, messages ...kafka.Message) error {
	r.commitCalls++
	*r.trace = append(*r.trace, fmt.Sprintf("commit:%d", messages[0].Offset))
	if r.commitFailures > 0 {
		r.commitFailures--
		return errUnavailable
	}
	return nil
}
func (r *fakeReader) Close() error {
	r.closeCalls++
	*r.trace = append(*r.trace, "close")
	return r.closeErr
}

type fakeProcessor struct {
	trace     *[]string
	failures  []error
	ids       []string
	calls     int
	dummy     domain.Dummy
	patch     domain.Patch
	onProcess func()
}

func (p *fakeProcessor) result(ctx context.Context, kind string) error {
	p.calls++
	p.ids = append(p.ids, requestid.ID(ctx))
	*p.trace = append(*p.trace, "process:"+kind)
	if p.onProcess != nil {
		p.onProcess()
	}
	if len(p.failures) == 0 {
		return nil
	}
	err := p.failures[0]
	p.failures = p.failures[1:]
	return err
}
func (p *fakeProcessor) ProcessCreate(ctx context.Context, dummy domain.Dummy) error {
	p.dummy = dummy
	return p.result(ctx, eventCreate)
}
func (p *fakeProcessor) ProcessUpdate(ctx context.Context, _ string, patch domain.Patch) error {
	p.patch = patch
	return p.result(ctx, eventUpdate)
}
func (p *fakeProcessor) ProcessDelete(ctx context.Context, _ string) error {
	return p.result(ctx, eventDelete)
}
func consumerForTest(t *testing.T, messages []kafka.Message, processor *fakeProcessor) (*Consumer, *fakeReader, context.Context, *fakeIDs) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	trace := []string{}
	reader := &fakeReader{messages: messages, cancel: cancel, trace: &trace}
	processor.trace = &trace
	ids := &fakeIDs{}
	consumer := NewConsumer(reader, processor, &testLog{}, ids)
	return consumer, reader, ctx, ids
}
func kafkaMessage(data string, offset int64) kafka.Message {
	return kafka.Message{Value: []byte(data), Offset: offset, Headers: []kafka.Header{{Key: RequestIDHeader, Value: []byte(incomingID)}}}
}
func TestConsumerDispatch(t *testing.T) {
	processor := &fakeProcessor{}
	consumer, reader, ctx, _ := consumerForTest(t, []kafka.Message{kafkaMessage(createJSON, 1), kafkaMessage(updateJSON, 2), kafkaMessage(deleteJSON, 3)}, processor)
	if err := consumer.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch:1", processCreateCall, commitFirst, "fetch:2", "process:update", "commit:2", "fetch:3", "process:delete", "commit:3", "close"}
	if !reflect.DeepEqual(*reader.trace, want) || reader.closeCalls != 1 {
		t.Fatalf("trace=%v closes=%d", *reader.trace, reader.closeCalls)
	}
	if processor.dummy.ID != testDummyID || processor.patch.Note == nil || *processor.patch.Note != "" {
		t.Fatal("processor received incorrect payloads")
	}
	if !reflect.DeepEqual(processor.ids, []string{incomingID, incomingID, incomingID}) {
		t.Fatal("processor lost request IDs")
	}
}
func TestConsumerRetries(t *testing.T) {
	processor := &fakeProcessor{failures: []error{errUnavailable, nil}}
	message := kafkaMessage(createJSON, 1)
	message.Headers = nil
	consumer, reader, ctx, ids := consumerForTest(t, []kafka.Message{message}, processor)
	reader.fetchFailures, reader.commitFailures = 1, 2
	var delays []time.Duration
	consumer.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	if err := consumer.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch-error", "fetch:1", processCreateCall, processCreateCall, commitFirst, commitFirst, commitFirst, "close"}
	if !reflect.DeepEqual(*reader.trace, want) {
		t.Fatalf("trace=%v", *reader.trace)
	}
	if !reflect.DeepEqual(delays, []time.Duration{initialBackoff, initialBackoff, initialBackoff, 2 * initialBackoff}) {
		t.Fatalf("delays=%v", delays)
	}
	if ids.calls != 1 || !reflect.DeepEqual(processor.ids, []string{generatedID, generatedID}) {
		t.Fatal("request ID was regenerated during retries")
	}
}
func TestConsumerPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		failure error
		calls   int
	}{
		{"malformed", "private-invalid-payload", nil, 0},
		{"unknown", `{"event_type":"unknown","dummy_id":"` + testDummyID + `"}`, nil, 0},
		{"null patch", `{"event_type":"update","dummy_id":"` + testDummyID + `","patch":{"note":null}}`, nil, 0},
		{"invalid command", createJSON, fmt.Errorf("processor: %w", domain.ErrInvalid), 1},
		{"conflict", updateJSON, fmt.Errorf("processor: %w", domain.ErrConflict), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processor := &fakeProcessor{failures: []error{tc.failure}}
			consumer, reader, ctx, _ := consumerForTest(t, []kafka.Message{kafkaMessage(tc.data, 1)}, processor)
			consumer.wait = func(_ context.Context, _ time.Duration) error {
				t.Error("permanent errors must not retry")
				return errUnavailable
			}
			if err := consumer.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if processor.calls != tc.calls || reader.commitCalls != 1 {
				t.Fatalf("processor calls=%d commits=%d", processor.calls, reader.commitCalls)
			}
		})
	}
}
func TestConsumerReplayAfterUncommittedProcessing(t *testing.T) {
	processor := &fakeProcessor{}
	message := kafkaMessage(deleteJSON, 7)
	first, reader, ctx, _ := consumerForTest(t, []kafka.Message{message}, processor)
	reader.commitFailures = 1
	first.wait = func(ctx context.Context, _ time.Duration) error {
		reader.cancel()
		return ctx.Err()
	}
	if err := first.Run(ctx); err != nil {
		t.Fatal(err)
	}
	second, replayReader, replayCtx, _ := consumerForTest(t, []kafka.Message{message}, processor)
	if err := second.Run(replayCtx); err != nil {
		t.Fatal(err)
	}
	if processor.calls != 2 || replayReader.commitCalls != 1 {
		t.Fatal("uncommitted messages must be processed again after restart")
	}
}
func TestCancellationAndLifecycleErrors(t *testing.T) {
	t.Run("fetch cancellation closes reader and exits", func(t *testing.T) {
		consumer, reader, ctx, _ := consumerForTest(t, nil, &fakeProcessor{})
		started := make(chan struct{})
		reader.fetch = func(ctx context.Context) (kafka.Message, error) {
			close(started)
			<-ctx.Done()
			return kafka.Message{}, ctx.Err()
		}
		done := make(chan error, 1)
		go func() { done <- consumer.Run(ctx) }()
		<-started
		reader.cancel()
		select {
		case err := <-done:
			if err != nil || reader.closeCalls != 1 {
				t.Fatalf("error=%v closes=%d", err, reader.closeCalls)
			}
		case <-time.After(time.Second):
			t.Fatal("consumer did not exit")
		}
	})
	t.Run("processing cancellation does not commit", func(t *testing.T) {
		processor := &fakeProcessor{failures: []error{errUnavailable}}
		consumer, reader, ctx, _ := consumerForTest(t, []kafka.Message{kafkaMessage(createJSON, 1)}, processor)
		processor.onProcess = reader.cancel
		if err := consumer.Run(ctx); err != nil || reader.commitCalls != 0 {
			t.Fatal("canceled processing was committed")
		}
	})
	t.Run("correlation failure is returned without commit", func(t *testing.T) {
		message := kafkaMessage(createJSON, 1)
		message.Headers = nil
		consumer, reader, ctx, ids := consumerForTest(t, []kafka.Message{message}, &fakeProcessor{})
		ids.err = errUnavailable
		if err := consumer.Run(ctx); !errors.Is(err, errUnavailable) || reader.commitCalls != 0 || reader.closeCalls != 1 {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("close failure is returned", func(t *testing.T) {
		consumer, reader, ctx, _ := consumerForTest(t, nil, &fakeProcessor{})
		reader.closeErr = errUnavailable
		if err := consumer.Run(ctx); !errors.Is(err, errUnavailable) {
			t.Fatalf("error=%v", err)
		}
	})
}
func TestBackoff(t *testing.T) {
	consumer, _, _, _ := consumerForTest(t, nil, &fakeProcessor{})
	var delays []time.Duration
	consumer.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	attempts := 0
	err := consumer.retry(context.Background(), func() error {
		attempts++
		if attempts == 10 {
			return nil
		}
		return errUnavailable
	}, "retry")
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1600 * time.Millisecond, 3200 * time.Millisecond, maxBackoff, maxBackoff, maxBackoff}
	if err != nil || !reflect.DeepEqual(delays, want) {
		t.Fatalf("error=%v delays=%v", err, delays)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitBackoff(ctx, maxBackoff); !errors.Is(err, context.Canceled) {
		t.Fatalf("backoff cancellation error=%v", err)
	}
}
