// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Pure helpers for listings rank: filtering, scoring, ordering, and the local
// listings reader.

package cli

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
)

var owdRankSorts = []string{"popularity", "price-x", "bids-per-day", "ending"}

// owdRankRow is one scored auction.
type owdRankRow struct {
	Domain        string   `json:"domain"`
	TLD           string   `json:"tld"`
	Price         string   `json:"price"`
	BidCount      int      `json:"bid_count"`
	EndDate       string   `json:"end_date"`
	HoursLeft     *float64 `json:"hours_left"`
	MinPrice      string   `json:"min_price"`
	PriceX        *float64 `json:"price_x"`
	TakenOf93     *int     `json:"taken_of_93"`
	PopularityPct *float64 `json:"popularity_pct"`
	BidsPerDay    float64  `json:"bids_per_day"`
	Checked       bool     `json:"checked"`
}

// owdRankOutput wraps the rows with the scan accounting.
type owdRankOutput struct {
	Rows            []owdRankRow `json:"rows"`
	ScannedListings int          `json:"scanned_listings"`
	ScannedPages    int          `json:"scanned_pages"`
	Matched         int          `json:"matched"`
	Checked         int          `json:"checked"`
	FetchFailures   []owdFailure `json:"fetch_failures"`
	Note            string       `json:"note,omitempty"`
}

// owdRankFilter applies the TLD suffix, the ending window (zero for none),
// and the bid ceiling (nil for none).
func owdRankFilter(listings []owdListing, tld string, ending time.Duration, maxBids *int, now time.Time) []owdListing {
	out := make([]owdListing, 0, len(listings))
	for _, l := range listings {
		if tld != "" && !strings.HasSuffix(strings.ToLower(l.Domain), "."+tld) {
			continue
		}
		if ending > 0 && !owdEndsWithin(l.EndDate, ending, now) {
			continue
		}
		if maxBids != nil && l.BidCount > *maxBids {
			continue
		}
		out = append(out, l)
	}
	return out
}

// owdRankHoursLeft returns hours until the auction ends (negative when past), nil if unparseable.
func owdRankHoursLeft(endDate string, now time.Time) *float64 {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(endDate))
	if err != nil {
		return nil
	}
	h := owdRound2(t.Sub(now).Hours())
	return &h
}

// owdRankBidsPerDay divides bids by the days since the listing was first
// seen (nil when never seen), floored at one day.
func owdRankBidsPerDay(bids int, firstSeen *time.Time, now time.Time) float64 {
	days := 1.0
	if firstSeen != nil {
		if d := now.Sub(*firstSeen).Hours() / 24; d > 1 {
			days = d
		}
	}
	return owdRound2(float64(bids) / days)
}

// owdRankPreorder orders candidates so a --max-checks cap scores the most
// relevant listings first for the requested sort.
func owdRankPreorder(listings []owdListing, sortKey string, now time.Time) {
	switch sortKey {
	case "ending":
		owdSortKeyed(listings, func(l owdListing) *float64 { return owdRankHoursLeft(l.EndDate, now) },
			func(x, y owdKeyed[owdListing, *float64]) bool {
				a, b := x.k, y.k
				if (a == nil) != (b == nil) {
					return a != nil
				}
				if a != nil && *a != *b {
					return *a < *b
				}
				return x.v.Domain < y.v.Domain
			})
	case "bids-per-day":
		sort.SliceStable(listings, func(i, j int) bool {
			if listings[i].BidCount != listings[j].BidCount {
				return listings[i].BidCount > listings[j].BidCount
			}
			return listings[i].Domain < listings[j].Domain
		})
	case "price-x":
		owdSortKeyed(listings, func(l owdListing) owdPrice { return owdParsePrice(l.Price) },
			func(x, y owdKeyed[owdListing, owdPrice]) bool {
				a, b := x.k, y.k
				if a.ok != b.ok {
					return a.ok
				}
				if a.ok && a.v != b.v {
					return a.v < b.v
				}
				return x.v.Domain < y.v.Domain
			})
	}
}

// owdRankScore scores one listing. tldCount is the site's count of TLDs
// where the word is still free (nil when unknown, which leaves the
// popularity fields null); knownTLDs supplies the TLD's list min price (a
// missing TLD leaves price_x null); firstSeen is the sighting date (nil when
// new).
func owdRankScore(l owdListing, tldCount *int, knownTLDs map[string]owdTLD, firstSeen *time.Time, now time.Time) owdRankRow {
	_, tld, _ := owdSplitDomain(l.Domain)
	minPrice := knownTLDs[tld].MinPrice
	row := owdRankRow{
		Domain:     strings.ToLower(l.Domain),
		TLD:        tld,
		Price:      l.Price,
		BidCount:   l.BidCount,
		EndDate:    l.EndDate,
		HoursLeft:  owdRankHoursLeft(l.EndDate, now),
		MinPrice:   minPrice,
		BidsPerDay: owdRankBidsPerDay(l.BidCount, firstSeen, now),
		Checked:    tldCount != nil,
	}
	if p, ok := owdPriceFloat(l.Price); ok {
		if m, ok := owdPriceFloat(minPrice); ok && m > 0 {
			row.PriceX = owdFloatPtr(owdRound2(p / m))
		}
	}
	if tldCount != nil {
		taken := owdTLDTotal - *tldCount
		if taken < 0 {
			taken = 0
		}
		if taken > owdTLDTotal {
			taken = owdTLDTotal
		}
		row.TakenOf93 = &taken
		row.PopularityPct = owdFloatPtr(owdRoundPct(owdPopularityPct(*tldCount)))
	}
	return row
}

// owdRankSort orders scored rows; unknown values always sort last.
func owdRankSort(rows []owdRankRow, sortKey string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch sortKey {
		case "ending":
			if (a.HoursLeft == nil) != (b.HoursLeft == nil) {
				return a.HoursLeft != nil
			}
			if a.HoursLeft != nil && *a.HoursLeft != *b.HoursLeft {
				return *a.HoursLeft < *b.HoursLeft
			}
		case "bids-per-day":
			if a.BidsPerDay != b.BidsPerDay {
				return a.BidsPerDay > b.BidsPerDay
			}
		case "price-x":
			if (a.PriceX == nil) != (b.PriceX == nil) {
				return a.PriceX != nil
			}
			if a.PriceX != nil && *a.PriceX != *b.PriceX {
				return *a.PriceX < *b.PriceX
			}
		default: // popularity
			if (a.TakenOf93 == nil) != (b.TakenOf93 == nil) {
				return a.TakenOf93 != nil
			}
			if a.TakenOf93 != nil && *a.TakenOf93 != *b.TakenOf93 {
				return *a.TakenOf93 > *b.TakenOf93
			}
			if (a.PriceX == nil) != (b.PriceX == nil) {
				return a.PriceX != nil
			}
			if a.PriceX != nil && *a.PriceX != *b.PriceX {
				return *a.PriceX < *b.PriceX
			}
		}
		return a.Domain < b.Domain
	})
}

// owdRankLocalListings reads the synced listings table (all rows drained before return).
func owdRankLocalListings(ctx context.Context, db *store.Store) ([]owdListing, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT COALESCE(domain,''), COALESCE(type,''), COALESCE(price,''), COALESCE(bid_count,0), end_date FROM listings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]owdListing, 0)
	for rows.Next() {
		var l owdListing
		var end any
		if err := rows.Scan(&l.Domain, &l.Type, &l.Price, &l.BidCount, &end); err != nil {
			return nil, err
		}
		l.EndDate = owdScanString(end)
		if l.Domain != "" {
			out = append(out, l)
		}
	}
	return out, rows.Err()
}
