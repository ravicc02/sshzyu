//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestAgentSSEDoesNotRetainTranscript(t *testing.T) {
	w := &agentResponseWriter{headers: make(http.Header)}
	w.headers.Set("Content-Type", "text/event-stream")
	frame := []byte(`data: {"type":"image_generation.partial_image","b64_json":"` + string(bytes.Repeat([]byte("A"), 1<<20)) + `"}` + "\n\n")
	for i := 0; i < 3; i++ {
		if _, err := w.Write(frame); err != nil {
			t.Fatal(err)
		}
	}
	if w.body.Len() != 0 || len(w.pending) != 0 || w.receivedBytes != len(frame)*3 {
		t.Fatal("full SSE transcript retained", w.body.Len(), len(w.pending))
	}
	if _, err := w.Write([]byte("data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AQID\"}\n\n")); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(w.resultBody(), []byte("AQID")) {
		t.Fatal("terminal evidence lost")
	}
	w.receivedBytes = 64 << 20
	if _, err := w.Write([]byte("x")); err != io.ErrShortBuffer || !w.overflow {
		t.Fatal("cumulative response cap bypassed")
	}
}
func TestAgentInvocationErrorsEnterOpsAndProbesDoNot(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)
	ops := service.NewOpsService(&ingressRejectOpsRepo{}, &ingressRejectSettingRepo{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(agentOpsMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		setOpsRequestContext(c, "agent-model", true)
		c.JSON(502, gin.H{"error": gin.H{"type": "api_error", "message": "upstream unavailable"}})
	})
	for _, probe := range []bool{true, false} {
		req := httptest.NewRequest("POST", "/v1/responses", nil)
		ctx := context.WithValue(req.Context(), ctxkey.ClientRequestID, "agent-op-fixture")
		if probe {
			ctx = context.WithValue(ctx, agentProbeKey{}, true)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req.WithContext(ctx))
		if w.Code != 502 {
			t.Fatal(w.Code)
		}
		if probe && OpsErrorLogEnqueuedTotal() != 0 {
			t.Fatal("authorization probe counted as invocation")
		}
	}
	if OpsErrorLogEnqueuedTotal() != 1 {
		t.Fatal("missing or duplicated Agent Ops error", OpsErrorLogEnqueuedTotal())
	}
	job := <-opsErrorLogQueue
	if job.entry.ClientRequestID != "agent-op-fixture" || job.entry.Model != "agent-model" {
		t.Fatal(job.entry)
	}
}

// Opt-in synthetic envelope on a constrained Linux container; no network or billing.
func TestAgentLargeImageMemoryEnvelope(t *testing.T) {
	if os.Getenv("AGENT_MEMORY_STRESS") != "1" {
		t.Skip("explicit resource envelope only")
	}
	var source bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 4096, 2048))); err != nil {
		t.Fatal(err)
	}
	expected := source.Len()
	frame := []byte(`data: {"type":"image_generation.completed","b64_json":"` + base64.StdEncoding.EncodeToString(source.Bytes()) + `"}` + "\n\n")
	source = bytes.Buffer{}
	runtime.GC()
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	var peak uint64
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := &agentResponseWriter{headers: make(http.Header), ctx: context.Background()}
			w.headers.Set("Content-Type", "text/event-stream")
			for offset := 0; offset < len(frame); offset += 64 << 10 {
				end := min(offset+(64<<10), len(frame))
				if _, err := w.Write(frame[offset:end]); err != nil {
					errs <- err
					return
				}
				runtime.Gosched()
			}
			release, err := agent.AcquireImageMemory(context.Background())
			if err != nil {
				errs <- err
				return
			}
			data, err := service.ResolveAgentImageResult(context.Background(), w.resultBody())
			release()
			if err != nil || len(data) != expected {
				errs <- fmt.Errorf("invalid output: %d %v", len(data), err)
				return
			}
			if w.body.Len() != 0 || len(w.pending) != 0 {
				errs <- fmt.Errorf("retained transcript")
				return
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			_ = m
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	peak = m.HeapSys
	t.Logf("12 parallel images, PNG=%d bytes, retained_heap=%d, heap_sys=%d", expected, m.HeapAlloc, peak)
}

func TestAgentURLRecoveryDoesNotWaitOnRasterMemory(t *testing.T) {
	release, err := agent.AcquireImageMemory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	executor := &platformAgentExecutor{}
	_, err = executor.Recover(ctx, agent.Invocation{Kind: "image"}, agent.Outcome{Cost: "0.17000000", Body: []byte(`{"data":[{"url":"http://127.0.0.1/private.png"}]}`)})
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("URL download waited on raster memory slot")
	}
	var invalid *agent.ResultFailure
	if !errors.As(err, &invalid) || invalid.Code != "AGENT_IMAGE_RESULT_INVALID" {
		t.Fatal("SSRF validation changed", err)
	}
}

func TestAgentWriterReleasePreservesAcceptedResponse(t *testing.T) {
	w := &agentResponseWriter{headers: make(http.Header), ctx: context.Background()}
	w.headers.Set("Content-Type", "text/event-stream")
	_, err := w.Write([]byte("data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AQID\"}\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	accepted := w.resultBody()
	w.releaseBuffers()
	if !bytes.Contains(accepted, []byte("AQID")) || len(w.completed) != 0 || w.body.Cap() != 0 || w.pending != nil || w.ctx != nil {
		t.Fatal("pooled writer retains large response or evidence was overwritten")
	}
}
func TestAgentRequestBuffersAreNotKeptByGinPool(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(bytes.Repeat([]byte("x"), 1<<20)))
	c.Set(gin.BodyBytesKey, bytes.Repeat([]byte("y"), 1<<20))
	releaseAgentRequestBuffers(c)
	if c.Request.Body != http.NoBody || c.Request.GetBody != nil || c.Keys != nil || c.Request.MultipartForm != nil {
		t.Fatal("pooled request retains byte buffers")
	}
}
