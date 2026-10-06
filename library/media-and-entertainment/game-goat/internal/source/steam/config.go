// config.go — Steam source configuration.
//
// Steam is a KEYLESS source: every endpoint this client calls
// (storesearch, appreviews, appdetails) and every store service it enumerates
// (IStoreQueryService, IStoreBrowseService, IStoreService) needs no credential,
// so the config carries rate-limit policy plus the store locale — no credential
// fields by design.

package steam

const (
	// DefaultRateLimit is the sustained requests/sec the Steam client paces at.
	DefaultRateLimit = 3.0
	// DefaultBurst is how many requests may fire back-to-back before the
	// sustained rate takes over (token-bucket capacity).
	DefaultBurst = 5
	// DefaultCountry is the storefront region used when neither the flag nor
	// STEAM_COUNTRY/ITAD_COUNTRY selects one. Store prices come back in this
	// region's currency, like the IsThereAnyDeal path.
	DefaultCountry = "US"
	// DefaultLanguage is the store locale used when --lang is not given.
	DefaultLanguage = "english"
)

// Config is the Steam source configuration. Steam needs no credential, so the
// fields are pacing plus the store locale.
type Config struct {
	// RateLimit is the sustained requests per second (token-bucket refill).
	RateLimit float64
	// Country is the ISO 3166-1 alpha-2 storefront region. Empty means
	// DefaultCountry; store prices and availability follow it.
	Country string
	// Language is the store locale (e.g. "english", "german"). Empty means
	// DefaultLanguage.
	Language string
}

// NewConfig returns the default Steam configuration: 3 requests/sec
// sustained, burst 5, adaptive backoff on HTTP 429.
func NewConfig() *Config {
	return &Config{RateLimit: DefaultRateLimit, Country: DefaultCountry, Language: DefaultLanguage}
}
