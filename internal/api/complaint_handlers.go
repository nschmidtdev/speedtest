package api

import (
	"net/http"
	"strconv"
	"time"

	"speedtest/internal/storage"
	"speedtest/internal/tariff"
)

// complaintRow ist eine tabellenfertige Zeile für das Template.
type complaintRow struct {
	Date      string
	Weekday   string
	DownAvg   float64
	UpAvg     float64
	DownPct   float64 // % vom Normalwert
	UpPct     float64
	DownBelow bool
	UpBelow   bool
}

type complaintData struct {
	GeneratedAt time.Time
	Provider    string
	TariffName  string
	AccessTech  string
	DownMbps    float64
	UpMbps      float64
	NormalDown  float64
	NormalUp    float64
	NormalIsEst bool // true wenn 90%-Schätzwert
	WindowDays  int
	// Absender-Adresse aus Settings
	SenderName   string
	SenderStreet string
	SenderCity   string
	SenderPhone  string
	SenderEmail  string
	// Empfänger-Adresse (Anbieter-Sitz) aus Settings
	ProviderStreet string
	ProviderCity   string
	CurrentStreak  int
	TotalBelow     int
	Rows           []complaintRow
}

// ComplaintHandler — GET /api/complaint?profile_id=1&days=30
// Generiert ein HTML-Beschwerdeschreiben nach §41 TKG.
// Liefert HTTP 409 Conflict, wenn die gesetzliche Schwelle
// (mindestens 2 aufeinanderfolgende Werktage Unterschreitung) nicht erfüllt ist.
func (s *AppState) ComplaintHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	profileID := int64(parseQueryInt(r, "profile_id", 0))
	if profileID <= 0 {
		writeError(w, http.StatusBadRequest, "profile_id is required")
		return
	}
	days := parseQueryInt(r, "days", 30)
	if days < 7 || days > 90 {
		days = 30
	}

	profile, err := storage.GetProfile(s.DB, profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	plans, err := storage.ListActiveTariffs(s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var plan *tariff.Tariff
	for i := range plans {
		if plans[i].ProfileID == profileID {
			plan = &plans[i]
			break
		}
	}
	if plan == nil {
		writeError(w, http.StatusBadRequest, "kein aktiver Tarif für dieses Profil")
		return
	}

	streak := storage.ComputeStreak(s.DB, profileID,
		plan.NormalDownMbps, plan.NormalUpMbps,
		plan.AdvertisedDownMbps, plan.AdvertisedUpMbps, days)

	if !streak.ReadyToComplain {
		writeError(w, http.StatusConflict,
			"Die gesetzliche Schwelle für eine Mängelmeldung ist nicht erfüllt. "+
				"Erforderlich: mindestens 2 aufeinanderfolgende Werktage mit Unterschreitung. "+
				"Aktuelle Strähne: "+strconv.Itoa(streak.CurrentStreak)+" Werktag(e).")
		return
	}

	// Normalwerte für Prozentberechnung (mit Fallback)
	normalDown := plan.NormalDownMbps
	if normalDown == 0 {
		normalDown = plan.AdvertisedDownMbps * 0.9
	}
	normalUp := plan.NormalUpMbps
	if normalUp == 0 {
		normalUp = plan.AdvertisedUpMbps * 0.9
	}

	// Template-Daten aufbereiten — keine Mathematik im Template
	var rows []complaintRow
	for _, d := range streak.Days {
		if !d.Below {
			continue
		}
		row := complaintRow{
			Date:      d.Date.Format("02.01.2006"),
			Weekday:   weekdayShort(d.Date),
			DownAvg:   d.DownAvg,
			UpAvg:     d.UpAvg,
			DownBelow: d.DownBelow,
			UpBelow:   d.UpBelow,
		}
		if normalDown > 0 {
			row.DownPct = d.DownAvg / normalDown * 100
		}
		if normalUp > 0 {
			row.UpPct = d.UpAvg / normalUp * 100
		}
		rows = append(rows, row)
	}

	addr, _ := storage.GetComplaintAddress(s.DB)
	providerStreet, providerCity := resolveProviderAddress(plan.Provider, addr)
	_ = profile // Profil geladen für Authorization-Check, nicht im Template verwendet

	data := complaintData{
		GeneratedAt:    time.Now(),
		Provider:       plan.Provider,
		TariffName:     plan.Name,
		AccessTech:     plan.AccessTechnology,
		DownMbps:       plan.AdvertisedDownMbps,
		UpMbps:         plan.AdvertisedUpMbps,
		NormalDown:     normalDown,
		NormalUp:       normalUp,
		NormalIsEst:    plan.NormalDownMbps == 0,
		WindowDays:     streak.WindowDays,
		SenderName:     addr.FullName,
		SenderStreet:   addr.Street,
		SenderCity:     addr.City,
		SenderPhone:    addr.Phone,
		SenderEmail:    addr.Email,
		ProviderStreet: providerStreet,
		ProviderCity:   providerCity,
		CurrentStreak:  streak.CurrentStreak,
		TotalBelow:     streak.TotalBelowDays,
		Rows:           rows,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderPartial(w, "complaint.html", data)
}

func resolveProviderAddress(providerName string, fallback storage.ComplaintAddress) (street, city string) {
	street, city = fallback.ProviderStreet, fallback.ProviderCity
	if catalogProvider, ok := tariff.FindCatalogProvider(providerName); ok {
		// Katalogadresse gewinnt für bekannte Anbieter; manuelle Settings
		// bleiben als Fallback für unvollständige/alte Katalogeinträge erhalten.
		if catalogProvider.AddressStreet != "" {
			street = catalogProvider.AddressStreet
		}
		if catalogProvider.AddressCity != "" {
			city = catalogProvider.AddressCity
		}
	}
	return street, city
}

var weekdayNames = []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

func weekdayShort(t time.Time) string {
	if int(t.Weekday()) >= 0 && int(t.Weekday()) < len(weekdayNames) {
		return weekdayNames[t.Weekday()]
	}
	return ""
}
