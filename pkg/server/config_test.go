package server

import (
	"testing"
	"time"
)

func TestParseConfigMetadataCacheTTL(t *testing.T) {
	config, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.MetadataCacheTTL != 10*time.Second {
		t.Fatalf("unexpected default metadata cache TTL: %s", config.MetadataCacheTTL)
	}
	config, err = ParseConfig([]string{"--metadata-cache-ttl=750ms"})
	if err != nil {
		t.Fatal(err)
	}
	if config.MetadataCacheTTL != 750*time.Millisecond {
		t.Fatalf("unexpected metadata cache TTL: %s", config.MetadataCacheTTL)
	}
}
