// Copyright 2026 Derik Parkinson and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/productivity/gmail/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/productivity/gmail/internal/config"
)

func TestSendMessageSingleAttempt(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "")
	for _, status := range []int{200, 401, 429, 500, 503, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.Method != "POST" || r.URL.Path != sendMessagePath {
					t.Error("wrong endpoint")
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				raw, err := base64.RawURLEncoding.DecodeString(payload["raw"])
				if err != nil || string(raw) != "Subject: hello\r\n\r\nbody" {
					t.Error("wrong MIME payload")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"id":"fake-sent-id"}`))
			}))
			t.Cleanup(server.Close)
			c := New(&config.Config{BaseURL: server.URL, AuthHeaderVal: "Bearer send-test-token"}, time.Second, 0)
			data, err := c.SendMessage(context.Background(), []byte("Subject: hello\r\n\r\nbody"))
			if (err == nil) != (status == 200) {
				t.Fatalf("status=%d data=%s err=%v", status, data, err)
			}
			if hits.Load() != 1 {
				t.Fatalf("send was replayed: %d attempts", hits.Load())
			}
			// Permission to send must not leak onto the original client.
			if _, _, err := c.Post(context.Background(), sendMessagePath, nil); err == nil {
				t.Fatal("generic POST was allowed after SendMessage")
			}
		})
	}
}

type sendBrokenTransport struct{ attempts atomic.Int32 }

func (tr *sendBrokenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	tr.attempts.Add(1)
	return nil, errors.New("connection lost after request write")
}

func TestSendMessageNetworkFailureNotRetried(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "")
	c := New(&config.Config{BaseURL: "https://gmail.invalid", AuthHeaderVal: "Bearer send-test-token"}, time.Second, 0)
	tr := &sendBrokenTransport{}
	c.HTTPClient.Transport = tr
	_, err := c.SendMessage(context.Background(), []byte("message"))
	if err == nil || !strings.Contains(err.Error(), "check Sent before retrying") {
		t.Fatalf("error: %v", err)
	}
	if tr.attempts.Load() != 1 {
		t.Fatalf("send was replayed %d times", tr.attempts.Load())
	}
}

func TestSendMessageVerifyModeDoesNotDial(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "1")
	t.Setenv(cliutil.VerifyLiveHTTPEnvVar, "")
	c := New(&config.Config{BaseURL: "https://gmail.invalid"}, time.Second, 0)
	tr := &sendBrokenTransport{}
	c.HTTPClient.Transport = tr
	if _, err := c.SendMessage(context.Background(), []byte("message")); err != nil {
		t.Fatal(err)
	}
	if tr.attempts.Load() != 0 {
		t.Fatal("verify mode made a network request")
	}
}
