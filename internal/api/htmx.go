package api

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"speedtest/internal/storage"
	"speedtest/internal/tariff"
	"speedtest/web"
)

// Template-Engine einmal initialisieren.
var partialTemplates *template.Template

// templateFuncs stelltGo-Template-Helper bereit.
var templateFuncs = template.FuncMap{
	"formatTime": formatTimeTmpl,
	"eq":         func(a, b any) bool { return a == b },
}

// formatTimeTmpl formatiert Zeitwerte für die Template-Ausgabe.
func formatTimeTmpl(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.Format("02.01.2006 15:04")
	case *time.Time:
		if t == nil {
			return "—"
		}
		return t.Format("02.01.2006 15:04")
	case string:
		return t
	case nil:
		return "—"
	default:
		return "—"
	}
}

func init() {
	partialTemplates = template.New("partials").Funcs(templateFuncs)
	// Dev-Mode: live von Festplatte lesen (kein Neubau nötig).
	if os.Getenv("SPEEDTEST_DEV") != "" {
		if _, err := partialTemplates.ParseGlob("web/partials/*.html"); err != nil {
			log.Fatalf("parse partials from disk: %v", err)
		}
		return
	}
	// Prod: aus embed.FS
	partialsFS, err := fs.Sub(web.Files, "partials")
	if err != nil {
		log.Fatalf("partials sub-FS: %v", err)
	}
	if _, err := partialTemplates.ParseFS(partialsFS, "*.html"); err != nil {
		log.Fatalf("parse partials: %v", err)
	}
}

// renderPartial führt ein Go-Template aus und schreibt es als HTML in den Response.
func renderPartial(w http.ResponseWriter, name string, data any) {
	// Dev-Mode: Templates bei jedem Request neu von Festplatte parsen (Live-Edit)
	if os.Getenv("SPEEDTEST_DEV") != "" {
		t, err := template.New("partials").Funcs(templateFuncs).ParseGlob("web/partials/*.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := t.ExecuteTemplate(w, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partialTemplates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// renderHTMXError sendet eine kompakte Fehlermeldung für HTMX-Clients.
func renderHTMXError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("HX-Retarget", "#form-status")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(`<span class="form-error">` + msg + "</span>"))
}

// === Tarif HTMX Handler ===

// TariffListPartial — GET /api/tariffs (HTMX: HTML-Liste, sonst JSON)
func (s *AppState) TariffListPartial(w http.ResponseWriter, r *http.Request) {
	plans, err := storage.ListActiveTariffs(s.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if plans == nil {
		plans = []tariff.Tariff{}
	}
	renderPartial(w, "tariff_list.html", plans)
}

// TariffProvidersPartial — GET /api/tariff/providers (HTMX: HTML options)
func (s *AppState) TariffProvidersPartial(w http.ResponseWriter, r *http.Request) {
	renderPartial(w, "tariff_providers.html", tariff.Catalog().Providers)
}

// TariffOptionsPartial — GET /api/tariff/options (HTMX: HTML options)
func (s *AppState) TariffOptionsPartial(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider_id")
	var options []tariff.CatalogTariff
	for _, p := range tariff.Catalog().Providers {
		if p.ID == providerID {
			options = p.Tariffs
			break
		}
	}
	data := struct {
		ProviderID string
		Tariffs    []tariff.CatalogTariff
	}{ProviderID: providerID, Tariffs: options}
	renderPartial(w, "tariff_options.html", data)
}

// ProfileListPartial — GET /api/profiles (HTMX: HTML-Liste, sonst JSON)
func (s *AppState) ProfileListPartial(w http.ResponseWriter, r *http.Request) {
	profiles, err := storage.ListProfiles(s.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderPartial(w, "profile_list.html", profiles)
}

// ProfileOptionsPartial — GET /api/profile/options (HTMX: HTML options für Select)
func (s *AppState) ProfileOptionsPartial(w http.ResponseWriter, r *http.Request) {
	profiles, err := storage.ListProfiles(s.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderPartial(w, "profile_options.html", profiles)
}