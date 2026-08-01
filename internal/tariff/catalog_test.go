package tariff

import "testing"

func TestCatalogContainsVerifiedProvidersAndTariffs(t *testing.T) {
	catalog := Catalog()
	if len(catalog.Providers) < 5 {
		t.Fatalf("provider count = %d, want at least 5", len(catalog.Providers))
	}
	seenProviders := map[string]bool{}
	seenTariffs := map[string]bool{}
	for _, provider := range catalog.Providers {
		if provider.ID == "" || provider.Name == "" || provider.SourceURL == "" {
			t.Fatalf("provider metadata incomplete: %#v", provider)
		}
		if seenProviders[provider.ID] {
			t.Fatalf("duplicate provider id %q", provider.ID)
		}
		seenProviders[provider.ID] = true
		if len(provider.Tariffs) == 0 {
			t.Fatalf("provider %q has no tariffs", provider.Name)
		}
		for _, plan := range provider.Tariffs {
			if plan.ID == "" || plan.Name == "" || plan.AdvertisedDownMbps <= 0 {
				t.Fatalf("tariff metadata incomplete: %#v", plan)
			}
			if seenTariffs[plan.ID] {
				t.Fatalf("duplicate tariff id %q", plan.ID)
			}
			seenTariffs[plan.ID] = true
		}
	}
	for _, required := range []string{"telekom", "vodafone", "o2", "1und1", "deutsche-glasfaser"} {
		if !seenProviders[required] {
			t.Errorf("required provider %q missing", required)
		}
	}
}

func TestFindCatalogTariffReturnsProviderAndTemplate(t *testing.T) {
	provider, plan, ok := FindCatalogTariff("dg-giga-1000")
	if !ok {
		t.Fatal("expected DG giga 1000 catalog entry")
	}
	if provider.Name != "Deutsche Glasfaser" || plan.AdvertisedDownMbps != 1000 || plan.AdvertisedUpMbps != 500 {
		t.Fatalf("unexpected lookup result: provider=%#v tariff=%#v", provider, plan)
	}
}

func TestCatalogMarksVariableUploadInsteadOfInventingValue(t *testing.T) {
	_, plan, ok := FindCatalogTariff("o2-home-l")
	if !ok {
		t.Fatal("expected O2 Home L catalog entry")
	}
	if !plan.RequiresUploadInput || plan.AdvertisedUpMbps != 0 {
		t.Fatalf("variable upload must require user input: %#v", plan)
	}
}
