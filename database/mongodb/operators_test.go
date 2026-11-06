package mongodb

import (
	"testing"

	"github.com/librespeed/speedtest-go/database/schema"
)

func TestNormalizeOperator(t *testing.T) {
	cas := map[string]string{
		// Variantes réellement présentes en base.
		"ANPTIC": "ANPTIC",
		"Agence Nationale de Promotion des TIC (ANPTIC)":          "ANPTIC",
		"ONATEL (Office National des Telecommunications, PTT)":    "ONATEL / Telmob",
		"Office National des Telecommunications":                  "ONATEL / Telmob",
		"Telmob":                                                  "ONATEL / Telmob",
		"ONATEL / Telmob":                                         "ONATEL / Telmob",
		"onatel":                                                  "ONATEL / Telmob",
		"Orange Burkina Faso":                                     "Orange Burkina Faso",
		"ORANGE BURKINA":                                          "Orange Burkina Faso",
		"Autorite de Regulation des Communications Electroniques": "ARCEP Burkina",
		"Telecel Faso SA":                                         "Telecel Faso",
		"Moov Africa Burkina":                                     "Moov Africa",
		// Nom inconnu : seulement débarrassé des espaces.
		"  Wayodi  ": "Wayodi",
		"":           "",
	}
	for entree, attendu := range cas {
		if got := NormalizeOperator(entree); got != attendu {
			t.Errorf("NormalizeOperator(%q) = %q, attendu %q", entree, got, attendu)
		}
	}
}

func TestToDocumentNormalizesOperators(t *testing.T) {
	m := &MongoDB{}
	doc := m.toDocument(&schema.TelemetryData{
		UUID:  "test-uuid",
		Extra: `{"operator": "Agence Nationale de Promotion des TIC (ANPTIC)", "simOperator": "Telmob"}`,
	})
	if doc.Operator != "ANPTIC" {
		t.Errorf("Operator = %q, attendu ANPTIC", doc.Operator)
	}
	if doc.SimOperator != "ONATEL / Telmob" {
		t.Errorf("SimOperator = %q, attendu ONATEL / Telmob", doc.SimOperator)
	}

	// Test web sans `extra` : l'opérateur vient de l'organisation IP.
	doc = m.toDocument(&schema.TelemetryData{
		UUID:    "test-uuid-web",
		ISPInfo: `{"rawIspInfo": {"org": "AS37577 ONATEL (Office National des Telecommunications, PTT)"}}`,
	})
	if doc.Operator != "ONATEL / Telmob" {
		t.Errorf("Operator (IP) = %q, attendu ONATEL / Telmob", doc.Operator)
	}
}
