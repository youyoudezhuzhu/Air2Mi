package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func offerResponse(status int, body string, headers http.Header) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers}
}

const testOffer = `{"schema_version":1,"app_id":"air2mi","product_id":"plus","enabled":true,"purchasable":true,"price_minor":1990,"currency":"CNY","purchase_url":"https://example.com/buy","button_text":"购买"}`

func TestOfferCacheAndETagRevalidation(t *testing.T) {
	now := time.Now()
	calls := 0
	source := newOfferSource(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != plusOfferURL {
			t.Fatal("unexpected endpoint")
		}
		if calls == 1 {
			return offerResponse(200, testOffer, http.Header{"Etag": []string{"v1"}}), nil
		}
		if r.Header.Get("If-None-Match") != "v1" {
			t.Fatal("ETag omitted")
		}
		return offerResponse(304, "", http.Header{}), nil
	})})
	source.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		status, v := source.get(context.Background())
		if status != 200 || v["price_minor"] != float64(1990) {
			t.Fatal(status, v)
		}
	}
	if calls != 1 {
		t.Fatal("fresh offer fetched twice")
	}
	now = now.Add(61 * time.Second)
	status, _ := source.get(context.Background())
	if status != 200 || calls != 2 {
		t.Fatal(status, calls)
	}
}
func TestOfferNotConfiguredIsDistinctAndCached(t *testing.T) {
	calls := 0
	source := newOfferSource(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return offerResponse(404, `{"code":"OFFER_NOT_CONFIGURED"}`, http.Header{}), nil
	})})
	for i := 0; i < 2; i++ {
		status, v := source.get(context.Background())
		if status != 404 || v["error"] != "OFFER_NOT_CONFIGURED" {
			t.Fatal(status, v)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestOfferErrorsCannotMasqueradeAsNotConfigured(t *testing.T) {
	for _, body := range []string{`<html>WAF</html>`, `{"app_id":"other","product_id":"plus"}`, strings.Repeat("x", 65537)} {
		source := newOfferSource(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return offerResponse(200, body, http.Header{}), nil })})
		status, v := source.get(context.Background())
		if status != 502 || v["error"] == "OFFER_NOT_CONFIGURED" {
			t.Fatal(status, v)
		}
	}
}

func TestMalformedOffersAreRejectedWithoutPoisoningCache(t *testing.T) {
	for _, change := range []struct{ old, replacement string }{
		{`"schema_version":1`, `"schema_version":2`},
		{`"enabled":true`, `"enabled":"true"`},
		{`"currency":"CNY"`, `"currency":"invalid"`},
		{`"price_minor":1990`, `"price_minor":-1`},
		{`"button_text":"购买"`, `"button_text":123`},
		{`"button_text":"购买"`, `"button_text":"购买","features":{}`},
		{`"button_text":"购买"`, `"button_text":"购买","features":[{"id":"permanent","title":{}}]`},
	} {
		t.Run(change.replacement, func(t *testing.T) {
			calls := 0
			source := newOfferSource(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				body := testOffer
				if calls == 1 {
					body = strings.Replace(testOffer, change.old, change.replacement, 1)
				}
				return offerResponse(200, body, http.Header{}), nil
			})})
			status, value := source.get(context.Background())
			if status != 502 || value["error"] != "OFFER_INVALID_RESPONSE" {
				t.Fatal("malformed offer accepted", status, value)
			}
			status, _ = source.get(context.Background())
			if status != 200 || calls != 2 {
				t.Fatal("valid offer did not recover", status, calls)
			}
		})
	}
}
func TestExpiredOfferNetworkFailureDoesNotReturnFreshPrice(t *testing.T) {
	now := time.Now()
	calls := 0
	source := newOfferSource(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return offerResponse(200, testOffer, http.Header{}), nil
		}
		return nil, errors.New("offline")
	})})
	source.now = func() time.Time { return now }
	source.get(context.Background())
	now = now.Add(61 * time.Second)
	status, v := source.get(context.Background())
	if status != 502 || v["price_minor"] != nil {
		t.Fatal(status, v)
	}
}
func TestOfferRateLimitHonorsRetryAfter(t *testing.T) {
	calls := 0
	source := newOfferSource(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return offerResponse(429, `{"code":"RATE_LIMITED"}`, http.Header{"Retry-After": []string{"60"}}), nil
	})})
	for i := 0; i < 2; i++ {
		status, v := source.get(context.Background())
		if status != 429 || v["retry_after"] != 60 {
			t.Fatal(status, v)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestOfferRedirectIsNotFollowed(t *testing.T) {
	calls := 0
	source := newOfferSource(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return offerResponse(302, `{}`, http.Header{"Location": []string{"https://untrusted.example/"}}), nil
	})})
	status, _ := source.get(context.Background())
	if status != 502 || calls != 1 {
		t.Fatal(status, calls)
	}
}
func TestOffersRequireHostAuthentication(t *testing.T) {
	s := &Server{base: "/app/air2mi", devToken: "test"}
	r := httptest.NewRequest("GET", "/app/air2mi/api/plus/offers", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
