// Copyright 2026 Brad Knight and contributors. Licensed under Apache-2.0. See LICENSE.
//
// steam_resolve_test.go - offline tests for the store-aware Steam identity path
// and the store-localisation knobs. No network access.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/game-goat/internal/client"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/game-goat/internal/config"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/game-goat/internal/source/steam"
)

func TestParseSteamAppID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"https://store.steampowered.com/app/379720/DOOM/", 379720, true},
		{"https://store.steampowered.com/app/2280/DOOM__DOOM_II/", 2280, true},
		{"https://store.steampowered.com/app/479030", 479030, true},
		{"https://store.steampowered.com/app/379720/DOOM/?cc=de", 379720, true},
		{"https://store.steampowered.com/app/379720/DOOM/#about", 379720, true},
		{"https://www.gog.com/fr/game/doom_2016", 0, false},
		{"https://store.steampowered.com/app/abc/DOOM/", 0, false},
		{"https://store.steampowered.com/app/0/Nothing/", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseSteamAppID(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseSteamAppID(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSteamYearFromRelease(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"2016-05-12", 2016},
		{"2016", 2016},
		{"2016-12-31", 2016},
		{"", 0},
		{"abcd-01-01", 0},
		{"1899-01-01", 0},
	}
	for _, tc := range cases {
		if got := steamYearFromRelease(tc.in); got != tc.want {
			t.Errorf("steamYearFromRelease(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSteamAppIDForRAWGGame(t *testing.T) {
	cases := []struct {
		name    string
		rawgID  int
		body    string
		want    int64
		wantErr bool
	}{
		{
			name:   "steam link among other storefronts",
			rawgID: 2454,
			body:   `{"count":3,"results":[{"store_id":5,"url":"https://www.gog.com/fr/game/doom_2016"},{"store_id":1,"url":"https://store.steampowered.com/app/379720/DOOM/"},{"store_id":3,"url":"https://store.playstation.com/x"}]}`,
			want:   379720,
		}, {
			name:   "1993 classic maps to its own appid",
			rawgID: 52884,
			body:   `{"count":1,"results":[{"store_id":1,"url":"https://store.steampowered.com/app/2280/DOOM__DOOM_II/"}]}`,
			want:   2280,
		}, {
			name:    "no steam link degrades",
			rawgID:  7,
			body:    `{"count":1,"results":[{"store_id":5,"url":"https://www.gog.com/game/x"}]}`,
			wantErr: true,
		}, {
			name:    "steam store row with an unparseable url degrades",
			rawgID:  8,
			body:    `{"count":1,"results":[{"store_id":1,"url":"https://store.steampowered.com/not-an-app/"}]}`,
			wantErr: true,
		}, {
			name:    "missing rawg id",
			rawgID:  0,
			body:    `{"count":0,"results":[]}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != fmt.Sprintf("/games/%d/stores", tc.rawgID) {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := client.New(&config.Config{BaseURL: srv.URL}, 0, 0)
			c.NoCache = true

			got, err := steamAppIDForRAWGGame(context.Background(), c, tc.rawgID)
			if tc.rawgID == 0 {
				if err == nil {
					t.Fatal("a RAWG id is required")
				}
				return
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got appid %d", got)
				}
				if !errors.Is(err, steam.ErrAppNotFound) {
					t.Errorf("error = %v, want ErrAppNotFound so callers degrade", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("steamAppIDForRAWGGame: %v", err)
			}
			if got != tc.want {
				t.Errorf("steamAppIDForRAWGGame = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestResolveSteamCountryPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		flag     string
		steamEnv string
		itadEnv  string
		want     string
		wantErr  bool
	}{
		{"explicit flag wins", "de", "GB", "FR", "DE", false},
		{"STEAM_COUNTRY next", "", "gb", "FR", "GB", false},
		{"ITAD_COUNTRY fallback", "", "", "fr", "FR", false},
		{"US default", "", "", "", steam.DefaultCountry, false},
		{"invalid flag is a usage error", "xyz", "", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STEAM_COUNTRY", tc.steamEnv)
			t.Setenv("ITAD_COUNTRY", tc.itadEnv)
			got, err := resolveSteamCountry(tc.flag)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveSteamCountry(%q) must error, got %q", tc.flag, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSteamCountry: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveSteamCountry = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSteamLanguage(t *testing.T) {
	t.Run("flag wins and is lower-cased", func(t *testing.T) {
		t.Setenv("STEAM_LANG", "french")
		if got := resolveSteamLanguage("German"); got != "german" {
			t.Errorf("resolveSteamLanguage = %q, want german", got)
		}
	})
	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("STEAM_LANG", "French")
		if got := resolveSteamLanguage(""); got != "french" {
			t.Errorf("resolveSteamLanguage = %q, want french", got)
		}
	})
	t.Run("default", func(t *testing.T) {
		t.Setenv("STEAM_LANG", "")
		if got := resolveSteamLanguage(""); got != steam.DefaultLanguage {
			t.Errorf("resolveSteamLanguage = %q, want %q", got, steam.DefaultLanguage)
		}
	})
}

func TestClassifySteamError(t *testing.T) {
	if err := classifySteamError(nil); err != nil {
		t.Errorf("classifySteamError(nil) = %v", err)
	}
	notFound := classifySteamError(fmt.Errorf("wrapped: %w", steam.ErrAppNotFound))
	var coded *cliError
	if !errors.As(notFound, &coded) || coded.code != 3 {
		t.Errorf("ErrAppNotFound should map to exit code 3, got %v", notFound)
	}
	ambiguous := classifySteamError(fmt.Errorf("wrapped: %w", steam.ErrAmbiguousApp))
	if !errors.As(ambiguous, &coded) || coded.code != 2 {
		t.Errorf("ErrAmbiguousApp should map to exit code 2, got %v", ambiguous)
	}
	plain := errors.New("transport failure")
	if got := classifySteamError(plain); got != plain {
		t.Errorf("an untyped failure must pass through, got %v", got)
	}
}
