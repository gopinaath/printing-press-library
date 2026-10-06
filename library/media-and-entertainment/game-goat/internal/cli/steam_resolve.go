// Copyright 2026 Brad Knight and contributors. Licensed under Apache-2.0. See LICENSE.
// PATCH(amend-2026-10-02: Steam identity comes from RAWG's Steam store link first)
//
// steam_resolve.go — store-aware Steam identity for the multi-source commands.
//
// Title-string resolution is fragile: RAWG names the 2016 reboot "DOOM (2016)"
// while the Steam store calls it plain "DOOM", and remakes share names across
// release years. RAWG already records the exact Steam appid for a game — it is
// in that game's store links — so ratings reads it from there and only falls
// back to a store-aware title search when the link is absent.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/game-goat/internal/client"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/game-goat/internal/source/steam"
)

// steamStoreID is RAWG's store id for Steam (2 Xbox, 3 PlayStation, 5 GOG).
const steamStoreID = 1

// rawgStoreLinkRow is one entry from RAWG's /games/{id}/stores listing.
type rawgStoreLinkRow struct {
	StoreID int    `json:"store_id"`
	URL     string `json:"url"`
}

// parseSteamAppID extracts the appid from a Steam store URL such as
// https://store.steampowered.com/app/379720/DOOM/.
func parseSteamAppID(rawURL string) (int64, bool) {
	const marker = "/app/"
	idx := strings.Index(rawURL, marker)
	if idx < 0 {
		return 0, false
	}
	rest := rawURL[idx+len(marker):]
	if cut := strings.IndexAny(rest, "/?#"); cut >= 0 {
		rest = rest[:cut]
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// steamAppIDForRAWGGame returns the exact Steam appid RAWG records for a game,
// read from its store links. This is the deterministic path: it cannot pick the
// wrong remake, because RAWG already distinguishes them (2454 DOOM (2016) ->
// 379720, 52884 DOOM (1993) -> 2280). A missing link returns the typed
// steam.ErrAppNotFound so callers degrade instead of failing.
func steamAppIDForRAWGGame(ctx context.Context, c *client.Client, rawgID int) (int64, error) {
	if c == nil || rawgID <= 0 {
		return 0, fmt.Errorf("steam: a RAWG game id is required to read its Steam store link")
	}
	data, err := c.Get(ctx, "/games/"+strconv.Itoa(rawgID)+"/stores", nil)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Results []rawgStoreLinkRow `json:"results"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("steam: parsing RAWG store links for game %d: %w", rawgID, err)
	}
	for _, link := range resp.Results {
		if link.StoreID != steamStoreID {
			continue
		}
		if appid, ok := parseSteamAppID(link.URL); ok {
			return appid, nil
		}
	}
	return 0, fmt.Errorf("%w: RAWG game %d lists no Steam store link", steam.ErrAppNotFound, rawgID)
}

// steamYearFromRelease pulls the year out of a RAWG "2016-05-12" release date.
func steamYearFromRelease(released string) int {
	released = strings.TrimSpace(released)
	if len(released) < 4 {
		return 0
	}
	year, err := strconv.Atoi(released[:4])
	if err != nil || year < 1900 || year > 2099 {
		return 0
	}
	return year
}

// steamEnrichmentForGame resolves one RAWG game to a Steam review block.
//
// Ordering: the RAWG Steam store link first (exact), then a store-aware title
// search pinned by the game's release year. It returns how the appid was found
// so --agent output can show the provenance, and it never fails the caller's
// command: the error travels back for sources_missing degradation.
func steamEnrichmentForGame(ctx context.Context, c *client.Client, game rawgGame, title string) (*steam.SteamReview, string, error) {
	appid, linkErr := steamAppIDForRAWGGame(ctx, c, game.ID)
	if linkErr == nil {
		review, rerr := steam.SteamReviewForApp(ctx, appid)
		if rerr == nil {
			return review, "rawg-store-link", nil
		}
		return nil, "", rerr
	}
	review, rerr := steam.SteamReviewForTitleWithYear(ctx, steamLookupName(title, game), steamYearFromRelease(game.Released))
	if rerr != nil {
		return nil, "", fmt.Errorf("%v; title fallback: %w", linkErr, rerr)
	}
	return review, "title", nil
}
