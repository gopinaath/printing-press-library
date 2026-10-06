// Copyright 2026 Brad Knight and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/game-goat/internal/source/steam"
)

func TestSteamBrowseTagFlagAcceptsCommaSeparated(t *testing.T) {
	cmd := newSteamBrowseCmd(&rootFlags{})
	if err := cmd.ParseFlags([]string{"--tag=Roguelike,Metroidvania", "--tag", "4X"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	got, err := cmd.Flags().GetStringSlice("tag")
	if err != nil {
		t.Fatalf("GetStringSlice: %v", err)
	}
	want := []string{"Roguelike", "Metroidvania", "4X"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

func TestRenderSteamReviewLine(t *testing.T) {
	tests := []struct {
		name   string
		review *steam.ReviewSummary
		want   string
	}{
		{
			name:   "unavailable",
			review: nil,
			want:   "Reviews: unavailable (sources_missing: steam_reviews)\n",
		},
		{
			name:   "total set",
			review: &steam.ReviewSummary{Desc: "Very Positive", Positive: 90, Negative: 10, Total: 100},
			want:   "Reviews: Very Positive (90% positive of 100)\n",
		},
		{
			name:   "total derived from positive plus negative",
			review: &steam.ReviewSummary{Desc: "", Positive: 3, Negative: 1, Total: 0},
			want:   "Reviews: - (75% positive of 4)\n",
		},
		{
			name:   "no reviews",
			review: &steam.ReviewSummary{Desc: "No user reviews"},
			want:   "Reviews: No user reviews (no reviews yet)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderSteamReviewLine(&buf, tt.review)
			if got := buf.String(); got != tt.want {
				t.Fatalf("renderSteamReviewLine = %q, want %q", got, tt.want)
			}
		})
	}
}
