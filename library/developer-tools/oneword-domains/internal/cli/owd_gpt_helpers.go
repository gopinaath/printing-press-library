// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored DomainsGPT request shape shared by `gpt generate` and
// `brainstorm`.

package cli

import (
	"fmt"
	"slices"
	"strings"
)

// owdGPTTypes and owdGPTPositions are the name styles and word placements
// DomainsGPT accepts.
var (
	owdGPTTypes     = []string{"portmanteau", "combination", "brandable", "nonenglish", "alternate", "random"}
	owdGPTPositions = []string{"prefix", "suffix", "anywhere"}
)

// owdGPTNamesPerCall is how many names one DomainsGPT call returns.
const owdGPTNamesPerCall = 20

// owdGPTRequest is one DomainsGPT generation as the command flags describe it.
type owdGPTRequest struct {
	Type     string
	Context  string
	MinLen   int
	MaxLen   int
	Word     string
	Position string
	TLD      string
	Exclude  []string
}

// validate applies the flag checks shared by `gpt generate` and `brainstorm`,
// returning the same usage errors (exit 2) both commands report.
func (r owdGPTRequest) validate() error {
	if !slices.Contains(owdGPTTypes, r.Type) {
		return usageErr(fmt.Errorf("--type must be one of %s", strings.Join(owdGPTTypes, ", ")))
	}
	if r.Word != "" && !slices.Contains(owdGPTPositions, r.Position) {
		return usageErr(fmt.Errorf("--position must be one of %s", strings.Join(owdGPTPositions, ", ")))
	}
	if r.MinLen <= 0 || r.MaxLen < r.MinLen {
		return usageErr(fmt.Errorf("--min-length must be positive and --max-length at least --min-length"))
	}
	return nil
}

// owdGPTBody builds the DomainsGPT request body: context, word/position and
// the exclusion list are sent only when set.
func owdGPTBody(r owdGPTRequest) map[string]any {
	body := map[string]any{"type": r.Type, "minLength": r.MinLen, "maxLength": r.MaxLen, "tld": owdNormTLD(r.TLD)}
	if r.Context != "" {
		body["context"] = r.Context
	}
	if r.Word != "" {
		body["word"] = strings.ToLower(r.Word)
		body["position"] = r.Position
	}
	if len(r.Exclude) > 0 {
		body["domains"] = r.Exclude
	}
	return body
}
