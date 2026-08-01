package tariff

// CatalogData is the locally embedded tariff-template catalog.
type CatalogData struct {
	VerifiedAt string            `json:"verified_at"`
	Note       string            `json:"note"`
	Providers  []CatalogProvider `json:"providers"`
}

type CatalogProvider struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	SourceURL string          `json:"source_url"`
	Tariffs   []CatalogTariff `json:"tariffs"`
}

type CatalogTariff struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	AccessTechnology    string  `json:"access_technology,omitempty"`
	AdvertisedDownMbps  float64 `json:"advertised_down_mbps"`
	AdvertisedUpMbps    float64 `json:"advertised_up_mbps,omitempty"`
	RequiresUploadInput bool    `json:"requires_upload_input,omitempty"`
}

// Catalog returns a fresh copy of the curated German provider catalog.
// Values marked RequiresUploadInput depend on address/technology and are
// deliberately not guessed.
func Catalog() CatalogData {
	return CatalogData{
		VerifiedAt: "2026-08-01",
		Note:       "Tarifvorlagen aus offiziellen Anbieterseiten. Verfügbarkeit und vertragliche Werte bitte mit dem Produktinformationsblatt abgleichen.",
		Providers: []CatalogProvider{
			{
				ID: "telekom", Name: "Deutsche Telekom",
				SourceURL: "https://www.telekom.de/shop/tarife/internet-tarife",
				Tariffs: []CatalogTariff{
					{ID: "telekom-mz-s", Name: "MagentaZuhause S", AccessTechnology: "DSL", AdvertisedDownMbps: 16, AdvertisedUpMbps: 2.4},
					{ID: "telekom-mz-m", Name: "MagentaZuhause M", AccessTechnology: "DSL", AdvertisedDownMbps: 50, AdvertisedUpMbps: 10},
					{ID: "telekom-mz-l", Name: "MagentaZuhause L", AccessTechnology: "DSL", AdvertisedDownMbps: 100, AdvertisedUpMbps: 40},
					{ID: "telekom-mz-xl", Name: "MagentaZuhause XL", AccessTechnology: "DSL", AdvertisedDownMbps: 250, AdvertisedUpMbps: 40},
					{ID: "telekom-glasfaser-150", Name: "Glasfaser 150", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 150, AdvertisedUpMbps: 75},
					{ID: "telekom-glasfaser-300", Name: "Glasfaser 300", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 300, AdvertisedUpMbps: 150},
					{ID: "telekom-glasfaser-600", Name: "Glasfaser 600", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 600, AdvertisedUpMbps: 300},
					{ID: "telekom-glasfaser-1000", Name: "Glasfaser 1000", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 1000, AdvertisedUpMbps: 500},
				},
			},
			{
				ID: "vodafone", Name: "Vodafone",
				SourceURL: "https://www.vodafone.de/privat/internet.html",
				Tariffs: []CatalogTariff{
					variableUpload("vodafone-internet-50", "Internet 50", 50),
					variableUpload("vodafone-internet-150", "Internet 150", 150),
					variableUpload("vodafone-internet-300", "Internet 300", 300),
					variableUpload("vodafone-internet-600", "Internet 600", 600),
					variableUpload("vodafone-internet-1000", "Internet 1000", 1000),
				},
			},
			{
				ID: "o2", Name: "O2 Telefónica",
				SourceURL: "https://www.o2online.de/internet-festnetz/",
				Tariffs: []CatalogTariff{
					variableUpload("o2-home-s", "O2 Home S", 50),
					variableUpload("o2-home-m", "O2 Home M", 150),
					variableUpload("o2-home-l", "O2 Home L", 300),
					variableUpload("o2-home-xl", "O2 Home XL", 600),
					variableUpload("o2-home-xxl", "O2 Home XXL", 1000),
				},
			},
			{
				ID: "1und1", Name: "1&1",
				SourceURL: "https://dsl.1und1.de/",
				Tariffs: []CatalogTariff{
					{ID: "1und1-dsl-16", Name: "1&1 DSL 16", AccessTechnology: "DSL", AdvertisedDownMbps: 16, AdvertisedUpMbps: 1},
					{ID: "1und1-dsl-50", Name: "1&1 DSL 50", AccessTechnology: "DSL", AdvertisedDownMbps: 50, AdvertisedUpMbps: 20},
					{ID: "1und1-dsl-100", Name: "1&1 DSL 100", AccessTechnology: "DSL", AdvertisedDownMbps: 100, AdvertisedUpMbps: 40},
					{ID: "1und1-dsl-250", Name: "1&1 DSL 250", AccessTechnology: "DSL", AdvertisedDownMbps: 250, AdvertisedUpMbps: 40},
				},
			},
			{
				ID: "deutsche-glasfaser", Name: "Deutsche Glasfaser",
				SourceURL: "https://www.deutsche-glasfaser.de/tarife",
				Tariffs: []CatalogTariff{
					{ID: "dg-basic-100", Name: "DG basic 100", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 100, AdvertisedUpMbps: 50},
					{ID: "dg-classic-300", Name: "DG classic 300", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 300, AdvertisedUpMbps: 150},
					{ID: "dg-premium-500", Name: "DG premium 500", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 500, AdvertisedUpMbps: 250},
					{ID: "dg-giga-1000", Name: "DG giga 1000", AccessTechnology: "Glasfaser", AdvertisedDownMbps: 1000, AdvertisedUpMbps: 500},
				},
			},
		},
	}
}

func variableUpload(id, name string, download float64) CatalogTariff {
	return CatalogTariff{ID: id, Name: name, AdvertisedDownMbps: download, RequiresUploadInput: true}
}

func FindCatalogTariff(id string) (CatalogProvider, CatalogTariff, bool) {
	for _, provider := range Catalog().Providers {
		for _, plan := range provider.Tariffs {
			if plan.ID == id {
				return provider, plan, true
			}
		}
	}
	return CatalogProvider{}, CatalogTariff{}, false
}
