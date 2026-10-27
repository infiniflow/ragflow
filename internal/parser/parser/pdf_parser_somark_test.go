package parser

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPDFParser_ParseWithResult_SoMarkJSONIntegration(t *testing.T) {
	withSSRFBypass(t)
	var submitSeen bool
	var pollSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse/async":
			submitSeen = true
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				return
			}
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				body, _ := io.ReadAll(part)
				fields[part.FormName()] = string(body)
			}
			if fields["api_key"] != "somark-secret" {
				t.Errorf("api_key = %q, want somark-secret", fields["api_key"])
				return
			}
			if !strings.Contains(fields["element_formats"], "image") {
				t.Errorf("element_formats = %q", fields["element_formats"])
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
		case "/parse/async_check":
			pollSeen = true
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "task_id=task-1") {
				t.Errorf("poll body = %q, want task_id", string(body))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"title","content":"SoMark Title","title_level":2},{"type":"figure","content":"Figure caption"},{"type":"table","content":"<table><tr><td>x</td></tr></table>"}]}]}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{
		"parse_method":    "SoMark",
		"somark_base_url": server.URL,
		"somark_api_key":  "somark-secret",
	})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !submitSeen || !pollSeen {
		t.Fatalf("submit/poll seen = %v/%v, want true/true", submitSeen, pollSeen)
	}
	if len(res.JSON) < 3 {
		t.Fatalf("JSON len = %d, want >=3", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_SoMarkRequiresBaseURL(t *testing.T) {
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "SoMark"})
	pdf.SoMarkBaseURL = ""
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "sample.pdf", []byte("%PDF-1.4\nmock"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "somark_base_url") {
		t.Fatalf("error = %v, want somark_base_url context", res.Err)
	}
}

func TestSoMarkBlockToItem_DropsHeaderByDefault(t *testing.T) {
	if item := soMarkBlockToItem(map[string]any{"type": "header", "content": "x"}, false); item != nil {
		t.Fatalf("item = %#v, want nil", item)
	}
}

func TestSoMarkSubmitMultipartShape(t *testing.T) {
	withSSRFBypass(t)
	var form multipart.Form
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = *r.MultipartForm
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-3"}}`))
	}))
	defer server.Close()
	taskID, err := soMarkSubmit(t.Context(), server.URL, "sample.pdf", []byte("%PDF"), NewPDFParser(), "key")
	if err != nil {
		t.Fatalf("soMarkSubmit: %v", err)
	}
	if taskID != "task-3" {
		t.Fatalf("taskID = %q, want task-3", taskID)
	}
	if got := form.Value["api_key"][0]; got != "key" {
		t.Fatalf("api_key = %q, want key", got)
	}
}

// TestSoMarkPoll_PollsUntilSuccess is the regression test for the missing
// poll loop. The async_check endpoint returns PENDING twice before SUCCESS;
// the old single-shot implementation returned an error on the first PENDING,
// so any task that needed processing time failed. The poll loop must keep
// asking until SUCCESS.
func TestSoMarkPoll_PollsUntilSuccess(t *testing.T) {
	withSSRFBypass(t)
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := polls.Add(1)
		if n < 3 {
			_, _ = w.Write([]byte(`{"code":0,"data":{"status":"PENDING"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"text","content":"polled body"}]}]}}}}}`))
	}))
	defer server.Close()

	result, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err != nil {
		t.Fatalf("soMarkPoll: %v", err)
	}
	if got := polls.Load(); got < 3 {
		t.Fatalf("polls = %d, want >=3 (must retry past PENDING)", got)
	}
	if r, _ := result["outputs"].(map[string]any); r == nil {
		t.Fatalf("result = %#v, want outputs", result)
	}
}

// TestSoMarkPoll_TerminalFailureStopsEarly confirms a failure status is not
// polled until the deadline.
func TestSoMarkPoll_TerminalFailureStopsEarly(t *testing.T) {
	withSSRFBypass(t)
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		polls.Add(1)
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"FAILED"}}`))
	}))
	defer server.Close()

	_, err := soMarkPoll(t.Context(), server.URL, "task-1", "")
	if err == nil || !strings.Contains(err.Error(), "failed status=FAILED") {
		t.Fatalf("err = %v, want failed status=FAILED", err)
	}
	if got := polls.Load(); got != 1 {
		t.Fatalf("polls = %d, want 1 (terminal failure must not be retried)", got)
	}
}

// TestSoMarkPoll_DeadlineExpires confirms a perpetually-running task stops at
// the caller's deadline rather than spinning forever.
func TestSoMarkPoll_DeadlineExpires(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"RUNNING"}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, err := soMarkPoll(ctx, server.URL, "task-1", "")
	if err == nil {
		t.Fatal("soMarkPoll: want error for perpetual RUNNING under a short deadline, got nil")
	}
	// Both "timed out" (our own deadline check) and "cancelled: context
	// deadline exceeded" (ctx.Err() during the inter-poll sleep) are correct
	// stop outcomes; what matters is that a never-finishing task does not run
	// for somarkPollTimeout.
	if msg := err.Error(); !strings.Contains(msg, "timed out") && !strings.Contains(msg, "deadline exceeded") {
		t.Fatalf("err = %v, want timed out or deadline exceeded", err)
	}
}

// TestSoMarkPoll_CtxCancelledMidPoll confirms a cancelled context aborts the
// wait between polls instead of sleeping the full interval.
func TestSoMarkPoll_CtxCancelledMidPoll(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"PENDING"}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	// Cancel shortly after the first poll returns, so the select on ctx.Done
	// during the sleep is the path that fires.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := soMarkPoll(ctx, server.URL, "task-1", "")
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("err = %v, want cancelled", err)
	}
	// somarkPollInterval is 3s; a cancelled-during-sleep exit must return well
	// before one full interval, proving the select respects cancellation.
	if elapsed > time.Second {
		t.Fatalf("elapsed = %v, want < 1s (cancel must preempt the poll interval)", elapsed)
	}
}

// TestIsSoMarkTerminalFailure pins the failure vs in-progress classification so
// the poll loop's stop condition is documented and guarded.
func TestIsSoMarkTerminalFailure(t *testing.T) {
	for _, s := range []string{"FAILED", "failed", "Failure", "ERROR", "error", "CANCELLED", "cancelled"} {
		if !isSoMarkTerminalFailure(s) {
			t.Errorf("isSoMarkTerminalFailure(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "PENDING", "RUNNING", "PROCESSING", "STARTED", "QUEUED", "SUCCESS"} {
		if isSoMarkTerminalFailure(s) {
			t.Errorf("isSoMarkTerminalFailure(%q) = true, want false", s)
		}
	}
}
