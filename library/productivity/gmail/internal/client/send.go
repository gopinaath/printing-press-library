// Copyright 2026 Derik Parkinson and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

const sendMessagePath = "/gmail/v1/users/me/messages/send"

// SendMessage is the sole opt-in sending path. Generic Post calls remain
// blocked by the cleanup allowlist. Never replay a send, including redirects,
// rate limits and ambiguous network failures: Gmail has no idempotency key.
func (c *Client) SendMessage(ctx context.Context, message []byte) (json.RawMessage, error) {
	sender := *c
	sender.sending = true
	httpClient := *c.HTTPClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	sender.HTTPClient = &httpClient
	data, status, err := sender.Post(ctx, sendMessagePath, map[string]string{
		"raw": base64.RawURLEncoding.EncodeToString(message),
	})
	if err != nil {
		return nil, fmt.Errorf("send failed; delivery may be uncertain; check Sent before retrying: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("send refused HTTP %d (redirects are not followed)", status)
	}
	return data, nil
}
