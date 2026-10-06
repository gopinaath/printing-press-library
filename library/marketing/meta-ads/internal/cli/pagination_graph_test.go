// Copyright 2026 dhilip-subramanian. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

// graphPaginationClient serves canned bodies to paginatedGet and records each
// request's params; the last body repeats once the list is exhausted.
type graphPaginationClient struct {
	bodies   []string
	requests []map[string]string
}

func (c *graphPaginationClient) GetWithHeaders(_ context.Context, _ string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	request := make(map[string]string, len(params))
	for key, value := range params {
		request[key] = value
	}
	c.requests = append(c.requests, request)
	i := len(c.requests) - 1
	if i >= len(c.bodies) {
		i = len(c.bodies) - 1
	}
	return json.RawMessage(c.bodies[i]), nil
}

// walkAll runs paginatedGet the way the promoted list commands call it: --all,
// no pagination metadata from the generated endpoint.
func walkAll(t *testing.T, client *graphPaginationClient) (json.RawMessage, string) {
	t.Helper()
	var data json.RawMessage
	stderr := captureStderr(t, func() {
		var err error
		data, err = paginatedGet(context.Background(), client, "/act_123/ads", map[string]string{}, nil, true, "", "offset", "limit", "", "")
		if err != nil {
			t.Fatalf("paginatedGet returned error: %v", err)
		}
	})
	return data, stderr
}

// The last Graph page keeps paging.cursors but has no paging.next; following
// cursors alone would ask for a page past the end.
func TestPaginatedGetAutoDetectsMetaGraphCursor(t *testing.T) {
	client := &graphPaginationClient{bodies: []string{
		`{"data":[{"id":"ad-1"},{"id":"ad-2"}],"paging":{"cursors":{"before":"cursor-0","after":"cursor-2"},"next":"https://graph.facebook.test/v19.0/act_123/ads?access_token=from-next-url&limit=2&after=cursor-2"}}`,
		`{"data":[{"id":"ad-3"}],"paging":{"cursors":{"before":"cursor-2","after":"cursor-3"}}}`,
		`{"data":[{"id":"past-last-page"}]}`,
	}}

	data, stderr := walkAll(t, client)

	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (stop when paging.next is absent)", len(items))
	}
	if len(client.requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(client.requests))
	}
	if got := client.requests[1]["after"]; got != "cursor-2" {
		t.Fatalf("second request after cursor = %q, want cursor-2", got)
	}
	for i, request := range client.requests {
		for key, value := range request {
			if strings.Contains(value, "from-next-url") || strings.HasPrefix(value, "http") {
				t.Fatalf("request %d replayed the next URL through %s=%q", i+1, key, value)
			}
		}
	}
	if !strings.Contains(stderr, `"event":"complete","total":3,"pages":2`) {
		t.Fatalf("a whole walk must report complete: %q", stderr)
	}
}

func TestPaginatedGetSinglePageWithCursorsOnlyDoesNotProbe(t *testing.T) {
	client := &graphPaginationClient{bodies: []string{
		`{"data":[{"id":"ad-1"}],"paging":{"cursors":{"before":"cursor-0","after":"cursor-1"}}}`,
		`{"data":[]}`,
	}}

	data, stderr := walkAll(t, client)

	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil || len(items) != 1 {
		t.Fatalf("got %d items (err %v), want 1", len(items), err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("got %d requests, want 1: a last page that keeps cursors must not be probed", len(client.requests))
	}
	if !strings.Contains(stderr, `"event":"complete","total":1,"pages":1`) {
		t.Fatalf("a single-page walk must report complete: %q", stderr)
	}
}

func TestCursorTokenFromMaybeURL(t *testing.T) {
	cases := []struct {
		name, token, param, want string
	}{
		{"plain token passes through", "QVFIUm", "after", "QVFIUm"},
		{"next URL reduced to cursor", "https://graph.facebook.com/v19.0/act_1/ads?access_token=x&limit=25&after=QVFIUm", "after", "QVFIUm"},
		{"next URL without cursor", "https://graph.facebook.com/v19.0/act_1/insights?since=1&until=2", "after", ""},
		{"next URL without cursor param name", "https://graph.facebook.com/v19.0/act_1/ads?after=QVFIUm", "", ""},
	}
	for _, tc := range cases {
		if got := cursorTokenFromMaybeURL(tc.token, tc.param); got != tc.want {
			t.Errorf("%s: cursorTokenFromMaybeURL(%q, %q) = %q, want %q", tc.name, tc.token, tc.param, got, tc.want)
		}
	}
}

func TestPaginatedGetStopsWithWarningWhenNextHasNoCursor(t *testing.T) {
	client := &graphPaginationClient{bodies: []string{
		`{"data":[{"id":"a"}],"paging":{"next":"https://graph.facebook.test/v19.0/act_123/insights?since=1&until=2"}}`,
	}}

	data, stderr := walkAll(t, client)

	if string(data) != `[{"id":"a"}]` {
		t.Fatalf("got %s, want page 1 only", data)
	}
	if len(client.requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(client.requests))
	}
	if !strings.Contains(stderr, `"reason":"pagination_cursor_missing"`) {
		t.Fatalf("stderr lacks the pagination_cursor_missing warning: %q", stderr)
	}
	if strings.Contains(stderr, `"event":"complete"`) {
		t.Fatalf("a truncated walk must not report complete: %q", stderr)
	}
}

func TestPaginatedGetStopsAtMaxPages(t *testing.T) {
	client := &graphPaginationClient{bodies: []string{
		`{"data":[{"id":"x"}],"paging":{"next":"https://graph.facebook.test/v19.0/act_123/ads?after=same"}}`,
	}}

	data, stderr := walkAll(t, client)

	if len(client.requests) != paginatedGetMaxPages {
		t.Fatalf("got %d requests, want the %d-page cap", len(client.requests), paginatedGetMaxPages)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil || len(items) != paginatedGetMaxPages {
		t.Fatalf("got %d items (err %v), want %d", len(items), err, paginatedGetMaxPages)
	}
	if !strings.Contains(stderr, `"reason":"max_pages_cap_hit"`) {
		t.Fatalf("stderr lacks the max_pages_cap_hit warning: %q", stderr)
	}
	if strings.Contains(stderr, `"event":"complete"`) {
		t.Fatalf("a walk cut short by the page cap must not report complete: %q", stderr)
	}
}

// An explicit next-cursor path from endpoint metadata still sends a plain
// token as-is; reducing next-page URLs must not touch it.
func TestPaginatedGetExplicitCursorPathUnchanged(t *testing.T) {
	client := &graphPaginationClient{bodies: []string{
		`{"data":[{"id":"a"}],"meta":{"next_cursor":"c1"}}`,
		`{"data":[{"id":"b"}]}`,
	}}

	var data json.RawMessage
	captureStderr(t, func() {
		var err error
		data, err = paginatedGet(context.Background(), client, "/thing", map[string]string{}, nil, true, "cursor", "cursor", "limit", "meta.next_cursor", "")
		if err != nil {
			t.Fatalf("paginatedGet returned error: %v", err)
		}
	})

	if string(data) != `[{"id":"a"},{"id":"b"}]` {
		t.Fatalf("got %s, want both pages", data)
	}
	if len(client.requests) != 2 || client.requests[1]["cursor"] != "c1" {
		t.Fatalf("requests = %v, want a second request with cursor=c1", client.requests)
	}
}

// TestGraphListAllFollowsPagingNext drives every command that pages a Graph
// edge through paginatedGet, with --all, against a two-page fixture. Page 1
// carries paging.next (a full URL with an embedded access token); page 2 is
// the real Graph last-page shape: cursors still present but no next. --all
// must fetch exactly both pages, send only the after= cursor, and never
// replay the URL's token.
func TestGraphListAllFollowsPagingNext(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		newCmd func(*rootFlags) *cobra.Command
		args   []string
	}{
		{name: "campaigns", path: "/act_1/campaigns", newCmd: newCampaignsPromotedCmd, args: []string{"act_1"}},
		{name: "adsets", path: "/act_1/adsets", newCmd: newAdsetsPromotedCmd, args: []string{"act_1"}},
		{name: "ads", path: "/act_1/ads", newCmd: newAdsPromotedCmd, args: []string{"act_1"}},
		{name: "customaudiences", path: "/act_1/customaudiences", newCmd: newCustomaudiencesPromotedCmd, args: []string{"act_1"}},
		{name: "adcreatives", path: "/ad_1/adcreatives", newCmd: newAdcreativesPromotedCmd, args: []string{"ad_1"}},
		{name: "me", path: "/me/adaccounts", newCmd: newMePromotedCmd},
		{name: "insights get-account", path: "/act_1/insights", newCmd: newInsightsGetAccountCmd, args: []string{"act_1"}},
		{name: "insights get-campaign", path: "/camp_1/insights", newCmd: newInsightsGetCampaignCmd, args: []string{"camp_1"}},
		{name: "insights get-ad-set", path: "/adset_1/insights", newCmd: newInsightsGetAdSetCmd, args: []string{"adset_1"}},
		{name: "insights get-ad", path: "/ad_1/insights", newCmd: newInsightsGetAdCmd, args: []string{"ad_1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var requests []*http.Request
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, r.Clone(r.Context()))
				mu.Unlock()
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				switch r.URL.Query().Get("after") {
				case "":
					next := srv.URL + "/v19.0" + tc.path + "?access_token=from-next-url&limit=2&after=cursor-2"
					fmt.Fprintf(w, `{"data":[{"id":"m1"},{"id":"m2"}],"paging":{"cursors":{"before":"cursor-0","after":"cursor-2"},"next":%q}}`, next)
				case "cursor-2":
					fmt.Fprint(w, `{"data":[{"id":"m3"}],"paging":{"cursors":{"before":"cursor-2","after":"cursor-3"}}}`)
				default:
					fmt.Fprint(w, `{"data":[{"id":"past-last-page"}]}`)
				}
			}))
			defer srv.Close()

			t.Setenv("HOME", t.TempDir())
			t.Setenv("META_ADS_BASE_URL", srv.URL)
			t.Setenv("META_ACCESS_TOKEN", "test-token")

			flags := &rootFlags{asJSON: true, noCache: true, dataSource: "live"}
			cmd := tc.newCmd(flags)
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(append(append([]string{}, tc.args...), "--all"))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("%s --all: %v\nstderr: %s", tc.name, err, stderr.String())
			}

			var envelope struct {
				Results []struct {
					ID string `json:"id"`
				} `json:"results"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("unmarshal output %q: %v", stdout.String(), err)
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
				if strings.Contains(r.URL.RawQuery, "from-next-url") {
					t.Fatalf("request %d replayed the token embedded in paging.next: %s", i+1, r.URL.String())
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Fatalf("request %d Authorization = %q, want Bearer test-token", i+1, got)
				}
			}
		})
	}
}

// captureStderr runs fn with os.Stderr redirected to a file and returns what
// fn wrote there; paginatedGet reports progress and warnings on os.Stderr as
// JSON events unless --human-friendly is set.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	origStderr, origHuman := os.Stderr, humanFriendly
	os.Stderr, humanFriendly = f, false
	defer func() { os.Stderr, humanFriendly = origStderr, origHuman }()
	fn()
	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
