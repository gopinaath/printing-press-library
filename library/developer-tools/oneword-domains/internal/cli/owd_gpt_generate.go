// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored replacement for the DomainsGPT generate endpoint: the site
// streams concatenated JSON objects, which the generated JSON client rejects.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelGptGenerateCmd(flags *rootFlags) *cobra.Command {
	var typ, ctxText, word, position, tld, exclude string
	var minLen, maxLen int
	var availableOnly, raw bool
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate about 20 DomainsGPT names with live availability, parsed from the stream into JSON",
		Long: strings.TrimSpace(`
Generate brandable domain names with DomainsGPT and check each one live.
The site streams concatenated JSON objects; this command parses them into a
real JSON array (or NDJSON with --raw) so agents and scripts can consume it.
Anonymous callers get 10 generations; a signed-in session (auth login --chrome)
or a partner token in ONEWORD_DOMAINS_GPT_TOKEN raises the quota.
Use 'brainstorm' when you also want registration prices and dictionary flags.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli gpt generate --type portmanteau --word open --position prefix --tld com
  oneword-domains-pp-cli gpt generate --type brandable --context "a terminal tool for domain search" --tld ai --available-only --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:endpoint":    "gpt.generate",
			"pp:method":      "POST",
			"pp:path":        "/api/gpt/generate",
			"pp:happy-args":  "--type=brandable;--tld=ai;--min-length=6;--max-length=10",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "gpt generate")
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			req := owdGPTRequest{Type: typ, Context: ctxText, MinLen: minLen, MaxLen: maxLen, Word: word, Position: position, TLD: tld, Exclude: owdParseCSVList(exclude)}
			if err := req.validate(); err != nil {
				_ = cmd.Usage()
				return err
			}
			// One request: the whole-call bound and the request timeout coincide.
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			names, rawBytes, err := owdGenerate(ctx, flags, owdGPTConfig(flags), owdGPTBody(req))
			if err != nil && len(names) == 0 {
				return owdTypedErr(cmd, flags, err)
			}
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v (returning the %d names parsed before the cut)\n", err, len(names))
			}
			if raw {
				_, werr := cmd.OutOrStdout().Write(append(rawBytes, '\n'))
				return werr
			}
			rows := make([]owdGenerated, 0, len(names))
			for _, n := range names {
				if availableOnly && !n.Available {
					continue
				}
				rows = append(rows, n)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "DomainsGPT returned no names matching the filter.")
				return nil
			}
			return owdHumanTable(cmd, flags, rows)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "random", "Name style: portmanteau, combination, brandable, nonenglish, alternate, random")
	cmd.Flags().StringVar(&ctxText, "context", "", "What the business or product does, to steer the names")
	cmd.Flags().IntVar(&minLen, "min-length", 7, "Minimum name length")
	cmd.Flags().IntVar(&maxLen, "max-length", 12, "Maximum name length")
	cmd.Flags().StringVar(&word, "word", "", "A word to include in every name")
	cmd.Flags().StringVar(&position, "position", "prefix", "Where the included word goes: prefix, suffix, anywhere")
	cmd.Flags().StringVar(&tld, "tld", "com", "TLD to append and check")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Previously generated domains to avoid, comma-separated")
	cmd.Flags().BoolVar(&availableOnly, "available-only", false, "Keep only names whose domain is available")
	cmd.Flags().BoolVar(&raw, "raw", false, "Print the untouched DomainsGPT stream instead of parsed JSON")
	return cmd
}
