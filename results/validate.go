package results

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Bornes de plausibilité des mesures reçues sur /results/telemetry.
//
// L'endpoint est public : n'importe qui peut y poster. Ces bornes ne prouvent
// pas qu'une mesure est vraie, mais elles écartent les valeurs physiquement
// impossibles (débit négatif, 50 Gb/s sur un mobile, coordonnées hors du
// globe) qui suffiraient à fausser la carte et les moyennes par opérateur.
const (
	maxSpeedMbps  = 10000  // 10 Gb/s : au-delà de toute connexion grand public
	maxLatencyMs  = 60000  // 60 s
	maxStartupMs  = 600000 // 10 min
	maxPageLoadMs = 600000
	maxPagesCount = 100
)

// telemetryExtra reprend les champs numériques de `extra` à contrôler. Les
// champs absents valent 0, qui est toujours dans les bornes.
type telemetryExtra struct {
	Type                   string  `json:"type"`
	Latitude               float64 `json:"latitude"`
	Longitude              float64 `json:"longitude"`
	QoERating              int     `json:"qoeRating"`
	StreamingStartupMs     float64 `json:"streamingStartupMs"`
	StreamingRebufferCount float64 `json:"streamingRebufferCount"`
	StreamingRebufferRatio float64 `json:"streamingRebufferRatio"`
	StreamingScore         float64 `json:"streamingScore"`
	BrowsingAvgLoadMs      float64 `json:"browsingAvgLoadMs"`
	BrowsingSuccessRate    float64 `json:"browsingSuccessRate"`
	BrowsingPagesTested    float64 `json:"browsingPagesTested"`
	BrowsingScore          float64 `json:"browsingScore"`
	SignalDbm              float64 `json:"signalDbm"`
	SignalLevel            float64 `json:"signalLevel"`
	RSRP                   float64 `json:"rsrp"`
	RSRQ                   float64 `json:"rsrq"`
	SINR                   float64 `json:"sinr"`
	SimSlot                float64 `json:"simSlot"`
	DownloadMin            float64 `json:"downloadMin"`
	DownloadMax            float64 `json:"downloadMax"`
	UploadMin              float64 `json:"uploadMin"`
	UploadMax              float64 `json:"uploadMax"`
	PingMin                float64 `json:"pingMin"`
	PingMax                float64 `json:"pingMax"`
	PacketLoss             float64 `json:"packetLoss"`
	GlobalScore            float64 `json:"globalScore"`
	DataUsedKiB            float64 `json:"dataUsedKiB"`
	TestID                 string  `json:"testId"`
	DeviceID               string  `json:"deviceId"`
}

// validateTelemetry renvoie une erreur décrivant le premier champ hors bornes,
// ou nil si la mesure est plausible. Les champs vides sont acceptés : le
// frontend web de LibreSpeed les envoie ainsi quand un test est interrompu.
func validateTelemetry(dl, ul, ping, jitter, extra string) error {
	measures := []struct {
		name  string
		value string
		max   float64
	}{
		{"dl", dl, maxSpeedMbps},
		{"ul", ul, maxSpeedMbps},
		{"ping", ping, maxLatencyMs},
		{"jitter", jitter, maxLatencyMs},
	}
	for _, m := range measures {
		if err := checkNumber(m.name, m.value, m.max); err != nil {
			return err
		}
	}

	if strings.TrimSpace(extra) == "" {
		return nil
	}
	var e telemetryExtra
	if err := json.Unmarshal([]byte(extra), &e); err != nil {
		return fmt.Errorf("extra: JSON invalide")
	}

	switch e.Type {
	case "", "active", "passive":
	default:
		return fmt.Errorf("extra.type: valeur inconnue %q", e.Type)
	}

	ranges := []struct {
		name     string
		value    float64
		min, max float64
	}{
		{"latitude", e.Latitude, -90, 90},
		{"longitude", e.Longitude, -180, 180},
		{"qoeRating", float64(e.QoERating), 0, 5},
		{"streamingStartupMs", e.StreamingStartupMs, 0, maxStartupMs},
		{"streamingRebufferCount", e.StreamingRebufferCount, 0, 10000},
		{"streamingRebufferRatio", e.StreamingRebufferRatio, 0, 1},
		{"streamingScore", e.StreamingScore, 0, 100},
		{"browsingAvgLoadMs", e.BrowsingAvgLoadMs, 0, maxPageLoadMs},
		{"browsingSuccessRate", e.BrowsingSuccessRate, 0, 1},
		{"browsingPagesTested", e.BrowsingPagesTested, 0, maxPagesCount},
		{"browsingScore", e.BrowsingScore, 0, 100},
		// 0 = signal non relevé ; sinon une puissance radio est négative.
		{"signalDbm", e.SignalDbm, -160, 0},
		{"signalLevel", e.SignalLevel, 0, 4},
		// Plages 3GPP (LTE et NR), 0 = non mesuré.
		{"rsrp", e.RSRP, -156, 0},
		{"rsrq", e.RSRQ, -43, 20},
		{"sinr", e.SINR, -23, 40},
		{"simSlot", e.SimSlot, 0, 4},
		{"downloadMin", e.DownloadMin, 0, maxSpeedMbps},
		{"downloadMax", e.DownloadMax, 0, maxSpeedMbps},
		{"uploadMin", e.UploadMin, 0, maxSpeedMbps},
		{"uploadMax", e.UploadMax, 0, maxSpeedMbps},
		{"pingMin", e.PingMin, 0, maxLatencyMs},
		{"pingMax", e.PingMax, 0, maxLatencyMs},
		// -1 = perte non mesurée.
		{"packetLoss", e.PacketLoss, -1, 100},
		{"globalScore", e.GlobalScore, 0, 100},
		// -1 = non mesuré ; 50 Go maximum pour un test.
		{"dataUsedKiB", e.DataUsedKiB, -1, 50 * 1024 * 1024},
	}
	if len(e.TestID) > 64 || len(e.DeviceID) > 64 {
		return fmt.Errorf("extra: identifiant trop long")
	}
	for _, r := range ranges {
		if r.value < r.min || r.value > r.max {
			return fmt.Errorf("extra.%s: %v hors de [%v, %v]", r.name, r.value, r.min, r.max)
		}
	}
	return nil
}

func checkNumber(name, value string, max float64) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%s: %q n'est pas un nombre", name, value)
	}
	if v < 0 || v > max || v != v { // v != v : NaN
		return fmt.Errorf("%s: %v hors de [0, %v]", name, v, max)
	}
	return nil
}
