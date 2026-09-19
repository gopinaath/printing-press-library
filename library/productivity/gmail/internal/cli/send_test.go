// Copyright 2026 Derik Parkinson and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func sendTestArgs() []string {
	return []string{"send", "--from", engineTestEmail, "--to", "recipient@example.invalid",
		"--subject", "Offline test", "--body", "Hello from a local test."}
}

// All HTTP must stay local. A valid-looking future token belongs only to the
// fixture, and every command explicitly uses its temporary auth/home paths.
func TestSendPreviewOffline(t *testing.T) {
	fx := newEngineFixture(t)
	for _, extra := range [][]string{
		nil, {"--agent"}, {"--yes"}, {"--send-now", "--dry-run"},
		{"--account", "does-not-exist", "--config", "/does-not-exist/config.toml"},
	} {
		out, stderr, code := fx.runCLI(t, append(sendTestArgs(), extra...)...)
		if code != 0 {
			t.Fatalf("%v: code=%d stderr=%s", extra, code, stderr)
		}
		result := mustParseJSON(t, out)
		if result["sent"] != false || result["status"] != "preview" || result["mime"] == nil {
			t.Fatalf("incomplete preview: %s", out)
		}
	}
	if requests := fx.fake.reqLog(); len(requests) != 0 {
		t.Fatalf("preview made HTTP requests: %v", requests)
	}
}

func TestSendLiveUsesFakeGmail(t *testing.T) {
	fx := newEngineFixture(t)
	var sends atomic.Int32
	var rawMessage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer engine-test-token" {
			t.Error("wrong fixture token")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /gmail/v1/users/me/profile":
			writeJSON(w, map[string]string{"emailAddress": engineTestEmail})
		case "POST /gmail/v1/users/me/messages/send":
			sends.Add(1)
			var payload struct {
				Raw string `json:"raw"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			decoded, err := base64.RawURLEncoding.DecodeString(payload.Raw)
			if err != nil {
				t.Error(err)
			}
			rawMessage = string(decoded)
			writeJSON(w, map[string]string{"id": "fake-sent-123", "threadId": "fake-thread"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GMAIL_BASE_URL", server.URL)
	out, stderr, code := fx.runCLI(t, append(sendTestArgs(), "--account", "test", "--send-now")...)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	result := mustParseJSON(t, out)
	if result["sent"] != true || result["id"] != "fake-sent-123" || sends.Load() != 1 {
		t.Fatalf("unexpected send result: %s (%d sends)", out, sends.Load())
	}
	msg, err := mail.ReadMessage(strings.NewReader(rawMessage))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Get("Subject") != "Offline test" {
		t.Fatalf("subject: %v", msg.Header)
	}
	addresses, err := msg.Header.AddressList("To")
	if err != nil || len(addresses) != 1 || addresses[0].Address != "recipient@example.invalid" {
		t.Fatalf("recipients: %v %v", addresses, err)
	}
}

func TestSendRefusesWrongAccount(t *testing.T) {
	fx := newEngineFixture(t)
	for _, extra := range [][]string{
		{"--send-now"},
		{"--send-now", "--account", "test", "--from", "wrong@example.invalid"},
	} {
		_, _, code := fx.runCLI(t, append(sendTestArgs(), extra...)...)
		if code == 0 {
			t.Fatalf("expected refusal: %v", extra)
		}
	}
	if len(fx.fake.reqLog()) != 0 {
		t.Fatal("invalid account made a request")
	}
	fx.fake.profileEmail = "other@example.invalid"
	_, _, code := fx.runCLI(t, append(sendTestArgs(), "--send-now", "--account", "test")...)
	if code == 0 {
		t.Fatal("live identity mismatch must refuse")
	}
	if fx.fake.countRequests("POST") != 0 {
		t.Fatal("mismatch sent email")
	}
}

func TestSendMIMEAttachmentsAndUnicode(t *testing.T) {
	dir := t.TempDir()
	attachment := filepath.Join(dir, "résumé.bin")
	data := bytes.Repeat([]byte{0, 1, 2, 255}, 70)
	if err := os.WriteFile(attachment, data, 0600); err != nil {
		t.Fatal(err)
	}
	opts := sendOptions{from: "Sender <sender@example.invalid>",
		to: []string{"\"Last, First\" <to@example.invalid>"}, cc: []string{"cc@example.invalid"},
		bcc: []string{"hidden@example.invalid"}, subject: strings.Repeat("こんにちは ", 25),
		bodyFile: "-", attachments: []string{attachment}}
	raw, err := composeSendMessage(opts, strings.NewReader("Hello café\nSecond line"))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != opts.subject {
		t.Fatalf("subject round trip: %q %v", subject, err)
	}
	for _, name := range []string{"To", "Cc", "Bcc", "From"} {
		addresses, err := msg.Header.AddressList(name)
		if err != nil || len(addresses) != 1 {
			t.Fatalf("%s: %v %v", name, addresses, err)
		}
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(part) // multipart decodes quoted-printable automatically.
	if err != nil || string(body) != "Hello café\r\nSecond line" {
		t.Fatalf("body: %q %v", body, err)
	}
	part, err = reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
	if err != nil || !bytes.Equal(decoded, data) || part.FileName() != "résumé.bin" {
		t.Fatalf("attachment did not round trip: %v %s", err, part.FileName())
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("trailing MIME part: %v", err)
	}
}

func TestSendValidation(t *testing.T) {
	valid := func() sendOptions {
		return sendOptions{from: "a@example.invalid", to: []string{"b@example.invalid"}, body: "hello"}
	}
	for name, change := range map[string]func(*sendOptions){
		"no from":            func(o *sendOptions) { o.from = "" },
		"multiple from":      func(o *sendOptions) { o.from = "a@example.invalid,b@example.invalid" },
		"bad to":             func(o *sendOptions) { o.to = []string{"not-an-address"} },
		"injected to":        func(o *sendOptions) { o.to = []string{"a@example.invalid\r\nBcc: evil@example.invalid"} },
		"injected subject":   func(o *sendOptions) { o.subject = "Hi\nBcc: evil@example.invalid" },
		"no recipients":      func(o *sendOptions) { o.to = nil },
		"missing attachment": func(o *sendOptions) { o.attachments = []string{filepath.Join(t.TempDir(), "absent")} },
		"invalid body":       func(o *sendOptions) { o.body = string([]byte{255}) },
		"oversize body":      func(o *sendOptions) { o.body = strings.Repeat("x", maxSendInputBytes+1) },
	} {
		t.Run(name, func(t *testing.T) {
			opts := valid()
			change(&opts)
			if _, err := composeSendMessage(opts, strings.NewReader("")); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	fx := newEngineFixture(t)
	if _, _, code := fx.runCLI(t, append(sendTestArgs(), "--body-file", "-")...); code == 0 {
		t.Fatal("body and body-file must be mutually exclusive")
	}
}

func TestSendBodyFileAndLongASCIISubject(t *testing.T) {
	file := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(file, []byte("line one\nline two"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := sendOptions{from: "a@example.invalid", bcc: []string{"b@example.invalid"},
		subject: strings.Repeat("subject ", 200), bodyFile: file}
	raw, err := composeSendMessage(opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != opts.subject {
		t.Fatal("long ASCII subject did not round trip")
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil || string(body) != "line one\r\nline two" {
		t.Fatalf("body: %q %v", body, err)
	}
}
