package region

import (
	"context"
	"strings"
	"testing"
)

func TestOverride(t *testing.T) {
	t.Setenv("RE_REGION", "cn")
	if Override() != CN {
		t.Fatalf("want cn")
	}
	t.Setenv("RE_REGION", "intl")
	if Override() != Intl {
		t.Fatalf("want intl")
	}
}

func TestGetnodeBaseEnvOverride(t *testing.T) {
	t.Setenv("RE_REGISTRAR_URL", "https://example.test/")
	if got := GetnodeBase(context.Background()); got != "https://example.test" {
		t.Fatalf("got %s", got)
	}
}

func TestSeedsURLEnvOverride(t *testing.T) {
	t.Setenv("RE_SEEDS_URL", "https://example.test/seeds.json")
	urls := SeedsURLs(context.Background())
	if len(urls) != 1 || urls[0] != "https://example.test/seeds.json" {
		t.Fatalf("got %#v", urls)
	}
}

func TestConstants(t *testing.T) {
	if !strings.HasPrefix(GetnodeCN, "https://getnode.intentcomputing.cn") {
		t.Fatal(GetnodeCN)
	}
	if !strings.HasPrefix(GetnodeIntl, "https://getnode.intentcomputing.net") {
		t.Fatal(GetnodeIntl)
	}
}

func TestGeoURL(t *testing.T) {
	t.Setenv("RE_GEO_URL", "")
	SetGeoURLOverride("")
	if GeoURL() != DefaultGeoURL {
		t.Fatalf("default got %s", GeoURL())
	}
	SetGeoURLOverride("https://geo.example/json")
	defer SetGeoURLOverride("")
	if GeoURL() != "https://geo.example/json" {
		t.Fatalf("override got %s", GeoURL())
	}
	t.Setenv("RE_GEO_URL", "https://env-geo.example/json")
	if GeoURL() != "https://env-geo.example/json" {
		t.Fatalf("env should win: %s", GeoURL())
	}
}
