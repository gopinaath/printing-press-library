// Copyright 2026 Derik Parkinson and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/productivity/gmail/internal/cliutil"
	"github.com/spf13/cobra"
)

type sendOptions struct {
	from, subject, body, bodyFile string
	to, cc, bcc, attachments      []string
	sendNow                       bool
}

func newSendCmd(flags *rootFlags) *cobra.Command {
	var opts sendOptions
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Preview an email locally; deliver only with --send-now",
		Long: `Build and preview a MIME email without credentials or network access.
Delivery requires --send-now and an explicit --account; --from must match that
account's verified primary address. --dry-run always prevents delivery, including
with --send-now. --agent and --yes never enable delivery. Sends cannot be undone
and are never automatically retried. Replies/threading and drafts are unsupported.`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, flags, opts)
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "Sender's primary email address (required, also for offline previews)")
	cmd.Flags().StringArrayVar(&opts.to, "to", nil, "Recipient address or address list (repeatable)")
	cmd.Flags().StringArrayVar(&opts.cc, "cc", nil, "Cc address or address list (repeatable)")
	cmd.Flags().StringArrayVar(&opts.bcc, "bcc", nil, "Bcc address or address list (repeatable)")
	cmd.Flags().StringVar(&opts.subject, "subject", "", "Email subject")
	cmd.Flags().StringVar(&opts.body, "body", "", "Plain-text UTF-8 body")
	cmd.Flags().StringVar(&opts.bodyFile, "body-file", "", "Read plain-text body from a file, or - for stdin")
	cmd.Flags().StringArrayVar(&opts.attachments, "attach", nil, "Attachment file (repeatable)")
	cmd.Flags().BoolVar(&opts.sendNow, "send-now", false, "Deliver the email to Gmail (otherwise preview only)")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")
	return cmd
}

func runSend(cmd *cobra.Command, flags *rootFlags, opts sendOptions) error {
	message, err := composeSendMessage(opts, cmd.InOrStdin())
	if err != nil {
		return usageErr(err)
	}
	if !opts.sendNow || flags.dryRun || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv()) {
		// Deliberately bypass config, OAuth and the transport, even with --account.
		// Keep full preview content under --agent (its --compact would drop it).
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"status": "preview", "sent": false, "account": flags.account,
			"from": opts.from, "to": opts.to, "cc": opts.cc, "bcc": opts.bcc,
			"subject": opts.subject, "mime": string(message),
			"hint": "No Gmail request was made. Delivery requires --send-now and --account.",
		})
	}
	return deliverSendMessage(cmd, flags, opts, message)
}

func deliverSendMessage(cmd *cobra.Command, flags *rootFlags, opts sendOptions, message []byte) error {
	if flags.account == "" {
		return usageErr(fmt.Errorf("--send-now requires an explicit --account"))
	}
	profile, err := gauthProfile(flags, flags.account)
	if err != nil {
		return err
	}
	from, _ := parseSendAddresses([]string{opts.from}) // Already validated by compose.
	if !strings.EqualFold(from[0].Address, profile.Email) {
		return refusedErr(fmt.Errorf("--from must match account %q (%s)", flags.account, profile.Email))
	}
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	if err := verifyLiveIdentity(cmd.Context(), c, flags, flags.account); err != nil {
		return err
	}
	data, err := c.SendMessage(cmd.Context(), message)
	if err != nil {
		return err
	}
	var result struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.ID == "" {
		return apiErr(fmt.Errorf("send response did not contain a message ID; check Sent before retrying"))
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
		"status": "sent", "sent": true, "id": result.ID,
		"threadId": result.ThreadID, "account": flags.account,
	})
}
