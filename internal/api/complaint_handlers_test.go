package api

import (
	"testing"

	"speedtest/internal/storage"
)

func TestResolveProviderAddressUsesCatalogForKnownProvider(t *testing.T) {
	street, city := resolveProviderAddress("Vodafone", storage.ComplaintAddress{
		ProviderStreet: "Manuelle Straße 1",
		ProviderCity:   "00000 Fallbackstadt",
	})
	if street != "Ferdinand-Braun-Platz 1" || city != "40549 Düsseldorf" {
		t.Fatalf("catalog address not preferred: street=%q city=%q", street, city)
	}
}

func TestResolveProviderAddressUsesSettingsForUnknownProvider(t *testing.T) {
	street, city := resolveProviderAddress("UnknownNet", storage.ComplaintAddress{
		ProviderStreet: "Manuelle Straße 1",
		ProviderCity:   "00000 Fallbackstadt",
	})
	if street != "Manuelle Straße 1" || city != "00000 Fallbackstadt" {
		t.Fatalf("manual fallback not used: street=%q city=%q", street, city)
	}
}
