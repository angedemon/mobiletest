package results

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/mongodb"
	log "github.com/sirupsen/logrus"
)

// DashboardExport exporte les mesures filtrées (IMP-11).
// GET /api/dashboard/export?format=csv|json&operator=&network=&from=AAAA-MM-JJ&to=AAAA-MM-JJ
//
// Mêmes filtres que /api/dashboard/results. Au plus 10 000 lignes, des plus
// récentes aux plus anciennes ; aucun champ personnel (IP, identifiant
// d'appareil) n'est exporté.
func DashboardExport(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	q := r.URL.Query()
	rows, err := database.MongoDB.FetchExport(q.Get("operator"), q.Get("network"), q.Get("from"), q.Get("to"))
	if err != nil {
		log.Errorf("DashboardExport error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	name := "yele-mesures-" + time.Now().Format("2006-01-02")
	if q.Get("format") == "json" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.json"`)
		jsonResponse(w, rows)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	// BOM UTF-8 : sans lui, Excel affiche mal les accents (« Télécel »).
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	cw.Comma = ';' // séparateur attendu par Excel en français
	_ = cw.Write(mongodb.ExportColumns)
	for _, row := range rows {
		_ = cw.Write(row.Values())
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Errorf("DashboardExport csv error: %s", err)
	}
}

// DashboardBarometer situe un résultat parmi les autres tests (IMP-09).
// GET /api/dashboard/barometer?download=12.4&upload=2.1&ping=45&operator=Orange%20Burkina%20Faso
func DashboardBarometer(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	q := r.URL.Query()
	num := func(k string) float64 {
		v, _ := strconv.ParseFloat(q.Get(k), 64)
		return v
	}
	res, err := database.MongoDB.FetchBarometer(num("download"), num("upload"), num("ping"), q.Get("operator"))
	if err != nil {
		log.Errorf("DashboardBarometer error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, res)
}
