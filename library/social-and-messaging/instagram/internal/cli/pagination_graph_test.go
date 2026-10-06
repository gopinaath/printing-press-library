// Copyright 2026 Mohammed Al Khamis and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestMediaListAllFollowsGraphPagingNext drives `media list --all` against a
// two-page Graph fixture. Page 1 carries paging.next (a full URL with an
// embedded access token); page 2 is the real Graph last-page shape: cursors
// still present but no next. --all must fetch exactly both pages, send only
// the after= cursor, and never replay the URL's token.
func TestMediaListAllFollowsGraphPagingNext(t *testing.T) {
	var mu sync.Mutex
	var requests []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Clone(r.Context()))
		mu.Unlock()
		switch r.URL.Query().Get("after") {
		case "":
			next := "https://graph.facebook.com/v22.0/1784/media?access_token=from-next-url&fields=id&limit=2&after=cursor-2"
			fmt.Fprintf(w, `{"data":[{"id":"m1"},{"id":"m2"}],"paging":{"cursors":{"before":"cursor-0","after":"cursor-2"},"next":%q}}`, next)
		case "cursor-2":
			fmt.Fprint(w, `{"data":[{"id":"m3"}],"paging":{"cursors":{"before":"cursor-2","after":"cursor-3"}}}`)
		default:
			fmt.Fprint(w, `{"data":[{"id":"past-last-page"}]}`)
		}
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("INSTAGRAM_BASE_URL", srv.URL)
	t.Setenv("INSTAGRAM_ACCESS_TOKEN", "test-token")

	flags := &rootFlags{asJSON: true, noCache: true, dataSource: "live"}
	cmd := newMediaListCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"1784", "--all", "--limit", "2", "--fields", "id"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("media list --all: %v\noutput: %s", err, out.String())
	}

	var envelope struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	var ids []string
	for _, item := range envelope.Results {
		ids = append(ids, item.ID)
	}
	if got := strings.Join(ids, ","); got != "m1,m2,m3" {
		t.Fatalf("got ids %q, want m1,m2,m3", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want 2 (stop when paging.next is absent)", len(requests))
	}
	if got := requests[1].URL.Query().Get("after"); got != "cursor-2" {
		t.Fatalf("second request after = %q, want cursor-2", got)
	}
	for i, r := range requests {
		if strings.Contains(r.URL.RawQuery, "from-next-url") || strings.Contains(r.Header.Get("Authorization"), "from-next-url") {
			t.Fatalf("request %d replayed the token embedded in paging.next: %s", i+1, r.URL.String())
		}
	}
}

// TestMediaListAllCapDoesNotReportComplete drives `media list --all` against a
// Graph fixture that always carries paging.next. The walk must stop at the
// page cap with a max_pages_cap_hit truncation event and must not also emit
// {"event":"complete"}, which would present the partial inventory as whole.
func TestMediaListAllCapDoesNotReportComplete(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		next := fmt.Sprintf("https://graph.facebook.com/v22.0/1784/media?limit=1&after=cursor-%d", n)
		fmt.Fprintf(w, `{"data":[{"id":"m%d"}],"paging":{"cursors":{"after":"cursor-%d"},"next":%q}}`, n, n, next)
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("INSTAGRAM_BASE_URL", srv.URL)
	t.Setenv("INSTAGRAM_ACCESS_TOKEN", "test-token")

	// paginatedGet reports progress on os.Stderr, not the command's writer.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	stderrCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		stderrCh <- buf.String()
	}()

	flags := &rootFlags{asJSON: true, noCache: true, dataSource: "live"}
	cmd := newMediaListCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"1784", "--all", "--limit", "1", "--fields", "id"})
	execErr := cmd.Execute()
	os.Stderr = origStderr
	_ = w.Close()
	stderr := <-stderrCh
	if execErr != nil {
		t.Fatalf("media list --all: %v\noutput: %s", execErr, out.String())
	}

	if got := requests.Load(); got != paginatedGetMaxPages {
		t.Fatalf("got %d requests, want %d (stop at the page cap)", got, paginatedGetMaxPages)
	}
	if !strings.Contains(stderr, `"reason":"max_pages_cap_hit"`) {
		t.Fatalf("stderr lacks the max_pages_cap_hit truncation event:\n%s", stderr)
	}
	if strings.Contains(stderr, `"event":"complete"`) {
		t.Fatalf("capped walk reported complete:\n%s", stderr)
	}
}

func TestCursorTokenFromMaybeURL(t *testing.T) {
	cases := []struct {
		name, token, param, want string
	}{
		{"plain token passes through", "QVFIUm", "after", "QVFIUm"},
		{"next URL reduced to cursor", "https://graph.facebook.com/v22.0/1/media?access_token=x&after=QVFIUm", "after", "QVFIUm"},
		{"next URL without cursor", "https://graph.facebook.com/v22.0/1/insights?since=1&until=2", "after", ""},
		{"next URL without cursor param name", "https://graph.facebook.com/v22.0/1/media?after=QVFIUm", "", ""},
	}
	for _, tc := range cases {
		if got := cursorTokenFromMaybeURL(tc.token, tc.param); got != tc.want {
			t.Errorf("%s: cursorTokenFromMaybeURL(%q, %q) = %q, want %q", tc.name, tc.token, tc.param, got, tc.want)
		}
	}
}
