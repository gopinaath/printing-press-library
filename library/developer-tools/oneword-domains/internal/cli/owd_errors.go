// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored error mapping, failure reporting and small value helpers
// shared by every One Word Domains hand-built command.

package cli

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/spf13/cobra"
)

// owdFailure is one per-item failure from a fan-out. Failures are kept out
// of every aggregate and listed under fetch_failures instead.
type owdFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

// owdFailures converts fan-out errors into JSON-friendly rows (source order).
func owdFailures(errs []cliutil.FanoutError) []owdFailure {
	out := make([]owdFailure, 0, len(errs))
	for _, e := range errs {
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		out = append(out, owdFailure{Source: e.Source, Error: msg})
	}
	return out
}

// owdWarnFailuresListed reports partial fan-out failures on stderr with the
// denominator for commands whose envelope carries fetch_failures, so the
// text points there instead of repeating each failure.
func owdWarnFailuresListed(w io.Writer, errs []cliutil.FanoutError, total int, what string) {
	if len(errs) == 0 || w == nil {
		return
	}
	fmt.Fprintf(w, "warning: %d of %d %s failed; they are listed under fetch_failures and excluded from the results\n", len(errs), total, what)
}

// owdWarnFailuresInline reports partial fan-out failures on stderr with the
// denominator and one "warn:" line per failure, for commands whose output
// has no fetch_failures key.
func owdWarnFailuresInline(w io.Writer, errs []cliutil.FanoutError, total int, what string) {
	if len(errs) == 0 || w == nil {
		return
	}
	fmt.Fprintf(w, "warning: %d of %d %s failed\n", len(errs), total, what)
	cliutil.FanoutReportErrors(w, errs)
}

// owdSourceErr rejects a --data-source value the command cannot honor:
// live-only commands refuse "local", local-only commands refuse "live". The
// strategy is the command's pp:data-source annotation (the same metadata the
// agent envelope reports) and the name in the message is its command path
// without the root. The usage error is also written as the JSON error
// envelope under --json.
func owdSourceErr(cmd *cobra.Command, flags *rootFlags) error {
	if flags == nil || cmd == nil {
		return nil
	}
	strategy := commandDataSourceAnnotation(cmd)
	command := strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
	var err error
	switch {
	case strategy == "live" && flags.dataSource == "local":
		err = usageErr(fmt.Errorf("%s reads the live API only; --data-source local is not supported", command))
	case strategy == "local" && flags.dataSource == "live":
		err = usageErr(fmt.Errorf("%s reads the local store only; --data-source live is not supported", command))
	default:
		return nil
	}
	writeAPIErrorEnvelope(cmd.OutOrStdout(), flags, err, ExitCode(err))
	return err
}

// owdAPIErr maps a raw client error to a typed CLI error and, under --json
// (or --agent), writes the standard {"error","code"} envelope to stdout so
// machine callers see the failure on the stream they parse. Typed errors
// pass through untouched (the command already shaped them), the site's
// lifetime-pass 401/403 becomes the session auth error with its fix, and
// everything else goes through the generated classifier. Route-specific
// mapping (the availability route's 500 for an unknown word) lives in the
// route helper, owdCheckDomain, whose typed error passes through here.
func owdAPIErr(cmd *cobra.Command, flags *rootFlags, err error) error {
	if err == nil {
		return nil
	}
	var typed *cliError
	if errors.As(err, &typed) {
		return err
	}
	classified := err
	if e, ok := owdIsSessionError(err); ok {
		classified = e
	}
	classified = classifyAPIErrorOnly(classified)
	if cmd != nil {
		writeAPIErrorEnvelope(cmd.OutOrStdout(), flags, classified, ExitCode(classified))
	}
	return classified
}

// owdTypedErr returns an error a command already shaped (the session auth
// error, a not-found, rate-limit or usage error, or a plain local-store
// failure) unchanged and, under --json (or --agent), also writes the standard
// {"error","code"} envelope to stdout the way owdAPIErr does for raw client
// errors, so machine callers see every failure on the stream they parse.
// Untyped errors carry exit code 1.
func owdTypedErr(cmd *cobra.Command, flags *rootFlags, err error) error {
	if err == nil {
		return nil
	}
	if cmd != nil {
		writeAPIErrorEnvelope(cmd.OutOrStdout(), flags, err, ExitCode(err))
	}
	return err
}

// owdScanString normalizes a scanned SQLite value (TEXT, BLOB, or a
// driver-decoded DATETIME) into a string.
func owdScanString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(x)
	}
}

// owdRound2 rounds a ratio to two decimals so output is stable across runs.
func owdRound2(f float64) float64 {
	return math.Round(f*100) / 100
}

// owdRoundPct rounds a percentage to one decimal; every popularity_pct and
// delta_pct field goes through it.
func owdRoundPct(f float64) float64 {
	return math.Round(f*10) / 10
}

// owdFloatPtr boxes a float for nullable JSON fields.
func owdFloatPtr(f float64) *float64 { return &f }
