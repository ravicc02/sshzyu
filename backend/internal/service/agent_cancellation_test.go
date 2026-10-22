package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentGatewayPreservesCancellationAndDeadline(t *testing.T) {
	for _, adapter := range []struct {
		name   string
		detach func(context.Context) (context.Context, context.CancelFunc)
	}{
		{"generic-stream", func(ctx context.Context) (context.Context, context.CancelFunc) {
			return detachStreamUpstreamContext(ctx, true)
		}},
		{"generic-json", func(ctx context.Context) (context.Context, context.CancelFunc) {
			return detachStreamUpstreamContext(ctx, false)
		}},
		{"openai", detachUpstreamContext},
	} {
		t.Run(adapter.name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			trace := &AgentInvocationTrace{}
			in := WithAgentInvocation(parent, trace)
			out, release := adapter.detach(in)
			defer release()
			if _, ok := out.Deadline(); !ok {
				t.Fatal("lost server timeout")
			}
			cancel()
			if !errors.Is(out.Err(), context.Canceled) {
				t.Fatal("lost agent stop", out.Err())
			}
			// Accounting survives stopping the model, using its own bounded context.
			billing, stop := detachedBillingContext(out)
			defer stop()
			if billing.Err() != nil {
				t.Fatal("billing cancelled with model")
			}
			RecordAgentCharge(billing, 0.17)
			if cost, ok := trace.Charge(); !ok || cost != 0.17 {
				t.Fatal("charge lost", cost, ok)
			}
		})
	}
}

func TestOrdinaryGatewayStreamsStillDetachClientCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	stream, release := detachStreamUpstreamContext(parent, true)
	defer release()
	cancel()
	if stream.Err() != nil {
		t.Fatal("changed ordinary client disconnect behavior")
	}
}

func TestAgentGatewayActuallyClosesBlockedUpstreamRequest(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "openai", true: "generic-stream"}[stream], func(t *testing.T) {
			arrived, aborted := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(arrived)
				<-r.Context().Done()
				close(aborted)
			}))
			defer server.Close()
			parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			input := WithAgentInvocation(parent, &AgentInvocationTrace{})
			upstream, release := detachUpstreamContext(input)
			if stream {
				upstream, release = detachStreamUpstreamContext(input, true)
			}
			defer release()
			req, err := http.NewRequestWithContext(upstream, http.MethodPost, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			returned := make(chan error, 1)
			go func() {
				response, err := http.DefaultClient.Do(req)
				if response != nil {
					response.Body.Close()
				}
				returned <- err
			}()
			select {
			case <-arrived:
			case <-parent.Done():
				t.Fatal("request not started")
			}
			cancel()
			select {
			case err = <-returned:
				if !errors.Is(err, context.Canceled) {
					t.Fatal("request not cancelled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("upstream transport ignored stop")
			}
			select {
			case <-aborted:
			case <-time.After(time.Second):
				t.Fatal("upstream connection remains open")
			}
		})
	}
}
