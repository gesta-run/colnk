package configvalidate

import "testing"

func TestValidateAddress(t *testing.T) {
	for _, value := range []string{"localhost:7443", "agent.example.test:7443", "127.0.0.1:53", "[::1]:53"} {
		if err := ValidateAddress(value, false); err != nil {
			t.Fatalf("valid address %q was rejected: %v", value, err)
		}
	}
	if err := ValidateAddress(":7443", true); err != nil {
		t.Fatalf("wildcard listen address was rejected: %v", err)
	}
}

func TestValidateAddressRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"https://agent.example.test", "bad host:7443", "-bad.example:7443", "example..test:7443", "localhost:0"} {
		if err := ValidateAddress(value, false); err == nil {
			t.Fatalf("invalid address %q was accepted", value)
		}
	}
	if err := ValidateAddress(":7443", false); err == nil {
		t.Fatal("empty host was accepted")
	}
}
