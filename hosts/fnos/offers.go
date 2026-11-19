package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const plusOfferURL = "https://license.yiling.win/api/v1/public/apps/air2mi/offers/plus"

var offerCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func validPlusOffer(value map[string]any) bool {
	if value["schema_version"] != float64(1) || value["app_id"] != "air2mi" ||
		(value["product_id"] != "plus" && value["product_id"] != "air2mi_plus") {
		return false
	}
	if _, ok := value["enabled"].(bool); !ok {
		return false
	}
	if _, ok := value["purchasable"].(bool); !ok {
		return false
	}
	currency, ok := value["currency"].(string)
	if !ok || !offerCurrencyPattern.MatchString(currency) {
		return false
	}
	for _, key := range []string{"title", "description", "button_text", "purchase_url", "badge", "updated_at"} {
		if value[key] != nil {
			if _, ok := value[key].(string); !ok {
				return false
			}
		}
	}
	for _, key := range []string{"price_minor", "original_price_minor"} {
		if value[key] != nil {
			number, ok := value[key].(float64)
			if !ok || number < 0 || number > 9007199254740991 || math.Trunc(number) != number {
				return false
			}
		}
	}
	if value["features"] != nil {
		features, ok := value["features"].([]any)
		if !ok {
			return false
		}
		for _, item := range features {
			feature, ok := item.(map[string]any)
			if !ok {
				return false
			}
			if _, ok := feature["id"].(string); !ok {
				return false
			}
			if _, ok := feature["title"].(string); !ok {
				return false
			}
			if feature["description"] != nil {
				if _, ok := feature["description"].(string); !ok {
					return false
				}
			}
		}
	}
	return true
}

type offerSource struct {
	mu      sync.Mutex
	client  *http.Client
	now     func() time.Time
	value   map[string]any
	status  int
	expires time.Time
	etag    string
}

func newOfferSource(client *http.Client) *offerSource {
	return &offerSource{client: client, now: time.Now}
}

var publicPlusOffers = newOfferSource(&http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})

// Public commercial metadata is independent of Engine state and never grants entitlements.
func (o *offerSource) get(ctx context.Context) (int, map[string]any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.now()
	if o.value != nil && now.Before(o.expires) {
		return o.status, o.value
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, plusOfferURL, nil)
	if err != nil {
		return 502, map[string]any{"error": "OFFER_UNAVAILABLE"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Air2Mi-fnOS/"+version)
	if o.etag != "" {
		req.Header.Set("If-None-Match", o.etag)
	}
	response, err := o.client.Do(req)
	if err != nil {
		return 502, map[string]any{"error": "OFFER_UNAVAILABLE"}
	}
	defer response.Body.Close()
	if response.StatusCode == 304 && o.value != nil && o.status == 200 {
		o.expires = now.Add(time.Minute)
		return o.status, o.value
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return 502, map[string]any{"error": "OFFER_INVALID_RESPONSE"}
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return 502, map[string]any{"error": "OFFER_INVALID_RESPONSE"}
	}
	if response.StatusCode != 200 {
		code, _ := value["code"].(string)
		if code == "OFFER_NOT_CONFIGURED" && response.StatusCode == 404 {
			o.status = 404
			o.value = map[string]any{"error": code}
			o.expires = now.Add(time.Minute)
			o.etag = ""
			return o.status, o.value
		}
		if response.StatusCode == 429 {
			seconds, err := strconv.Atoi(response.Header.Get("Retry-After"))
			if err != nil || seconds < 1 {
				seconds = 60
			}
			if seconds > 86400 {
				seconds = 86400
			}
			o.status = 429
			o.value = map[string]any{"error": "RATE_LIMITED", "retry_after": seconds}
			o.expires = now.Add(time.Duration(seconds) * time.Second)
			o.etag = ""
			return o.status, o.value
		}
		return 502, map[string]any{"error": "OFFER_UNAVAILABLE"}
	}
	if !validPlusOffer(value) {
		return 502, map[string]any{"error": "OFFER_INVALID_RESPONSE"}
	}
	o.value = value
	o.status = 200
	o.expires = now.Add(time.Minute)
	o.etag = response.Header.Get("ETag")
	return 200, value
}
