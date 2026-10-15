package mongodb

import (
	"context"
	"sort"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// resultsFilter construit le filtre commun à la liste paginée et à l'export :
// opérateur (sous-chaîne, insensible à la casse), type de réseau et période
// (dates AAAA-MM-JJ incluses).
func resultsFilter(operator, networkType, from, to string) bson.M {
	filter := withoutPassive(bson.M{})
	if operator != "" {
		filter["operator"] = bson.M{"$regex": regexpQuote(operator), "$options": "i"}
	}
	if networkType != "" {
		filter["network_type"] = networkType
	}
	timeFilter := bson.M{}
	if from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			timeFilter["$gte"] = t
		}
	}
	if to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			timeFilter["$lte"] = t.Add(24 * time.Hour)
		}
	}
	if len(timeFilter) > 0 {
		filter["timestamp"] = timeFilter
	}
	return filter
}

// regexpQuote neutralise les caractères spéciaux d'une saisie utilisateur
// utilisée comme motif $regex (sinon « ( » ou « .* » changent la requête).
func regexpQuote(s string) string {
	const special = `\.+*?()|[]{}^$`
	out := make([]rune, 0, len(s))
	for _, r := range s {
		for _, sp := range special {
			if r == sp {
				out = append(out, '\\')
				break
			}
		}
		out = append(out, r)
	}
	return string(out)
}

// ── Export (IMP-11) ──────────────────────────────────────────────────────────

// ExportRow est une mesure exportée : uniquement des champs non personnels
// (ni IP, ni identifiant d'appareil, ni informations du FAI brutes).
type ExportRow struct {
	Timestamp        string  `json:"timestamp"`
	Operator         string  `json:"operator"`
	NetworkType      string  `json:"networkType"`
	CellularTech     string  `json:"cellularTech"`
	SimOperator      string  `json:"simOperator"`
	Location         string  `json:"location"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	LocationSource   string  `json:"locationSource"`
	DownloadMbps     float64 `json:"downloadMbps"`
	DownloadMin      float64 `json:"downloadMin"`
	DownloadMax      float64 `json:"downloadMax"`
	UploadMbps       float64 `json:"uploadMbps"`
	UploadMin        float64 `json:"uploadMin"`
	UploadMax        float64 `json:"uploadMax"`
	PingMs           float64 `json:"pingMs"`
	PingMin          float64 `json:"pingMin"`
	PingMax          float64 `json:"pingMax"`
	JitterMs         float64 `json:"jitterMs"`
	PacketLoss       float64 `json:"packetLoss"`
	RSRP             int     `json:"rsrp"`
	RSRQ             int     `json:"rsrq"`
	SINR             int     `json:"sinr"`
	SignalDbm        int     `json:"signalDbm"`
	BrowsingScore    float64 `json:"browsingScore"`
	BrowsingLoadMs   float64 `json:"browsingAvgLoadMs"`
	StreamingScore   float64 `json:"streamingScore"`
	StreamingMaxRes  string  `json:"streamingMaxResolution"`
	StreamingStartMs int     `json:"streamingStartupMs"`
	GlobalScore      float64 `json:"globalScore"`
	DataUsedKiB      int     `json:"dataUsedKiB"`
	DeviceModel      string  `json:"deviceModel"`
	QoERating        int     `json:"qoeRating"`
}

// ExportColumns est l'ordre des colonnes du CSV, aligné sur ExportRow.
var ExportColumns = []string{
	"timestamp", "operator", "networkType", "cellularTech", "simOperator",
	"location", "latitude", "longitude", "locationSource",
	"downloadMbps", "downloadMin", "downloadMax",
	"uploadMbps", "uploadMin", "uploadMax",
	"pingMs", "pingMin", "pingMax", "jitterMs", "packetLoss",
	"rsrp", "rsrq", "sinr", "signalDbm",
	"browsingScore", "browsingAvgLoadMs",
	"streamingScore", "streamingMaxResolution", "streamingStartupMs",
	"globalScore", "dataUsedKiB", "deviceModel", "qoeRating",
}

// Values renvoie la ligne sous forme de texte, dans l'ordre d'ExportColumns.
func (r ExportRow) Values() []string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	i := strconv.Itoa
	return []string{
		r.Timestamp, r.Operator, r.NetworkType, r.CellularTech, r.SimOperator,
		r.Location, f(r.Latitude), f(r.Longitude), r.LocationSource,
		f(r.DownloadMbps), f(r.DownloadMin), f(r.DownloadMax),
		f(r.UploadMbps), f(r.UploadMin), f(r.UploadMax),
		f(r.PingMs), f(r.PingMin), f(r.PingMax), f(r.JitterMs), f(r.PacketLoss),
		i(r.RSRP), i(r.RSRQ), i(r.SINR), i(r.SignalDbm),
		f(r.BrowsingScore), f(r.BrowsingLoadMs),
		f(r.StreamingScore), r.StreamingMaxRes, i(r.StreamingStartMs),
		f(r.GlobalScore), i(r.DataUsedKiB), r.DeviceModel, i(r.QoERating),
	}
}

// MaxExportRows borne un export : au-delà, filtrer par période.
const MaxExportRows = 10000

// FetchExport renvoie les mesures filtrées, des plus récentes aux plus
// anciennes, sans aucun champ personnel.
func (m *MongoDB) FetchExport(operator, networkType, from, to string) ([]ExportRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetLimit(MaxExportRows).
		SetProjection(bson.M{"ip_address": 0, "isp_info": 0, "extra_raw": 0,
			"user_agent": 0, "language": 0, "log": 0, "device_id": 0})
	cursor, err := m.collection.Find(ctx, resultsFilter(operator, networkType, from, to), opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []Document
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	rows := make([]ExportRow, 0, len(docs))
	for _, d := range docs {
		dl, _ := strconv.ParseFloat(d.Download, 64)
		ul, _ := strconv.ParseFloat(d.Upload, 64)
		ping, _ := strconv.ParseFloat(d.Ping, 64)
		jit, _ := strconv.ParseFloat(d.Jitter, 64)
		rows = append(rows, ExportRow{
			Timestamp:        d.Timestamp.UTC().Format(time.RFC3339),
			Operator:         d.Operator,
			NetworkType:      d.NetworkType,
			CellularTech:     d.CellularTech,
			SimOperator:      d.SimOperator,
			Location:         d.Location,
			Latitude:         d.Latitude,
			Longitude:        d.Longitude,
			LocationSource:   d.LocationSource,
			DownloadMbps:     dl,
			DownloadMin:      d.DownloadMin,
			DownloadMax:      d.DownloadMax,
			UploadMbps:       ul,
			UploadMin:        d.UploadMin,
			UploadMax:        d.UploadMax,
			PingMs:           ping,
			PingMin:          d.PingMin,
			PingMax:          d.PingMax,
			JitterMs:         jit,
			PacketLoss:       d.PacketLoss,
			RSRP:             d.RSRP,
			RSRQ:             d.RSRQ,
			SINR:             d.SINR,
			SignalDbm:        d.SignalDbm,
			BrowsingScore:    d.BrowsingScore,
			BrowsingLoadMs:   d.BrowsingAvgLoadMs,
			StreamingScore:   d.StreamingScore,
			StreamingMaxRes:  d.StreamingMaxResolution,
			StreamingStartMs: d.StreamingStartupMs,
			GlobalScore:      d.GlobalScore,
			DataUsedKiB:      d.DataUsedKiB,
			DeviceModel:      d.DeviceModel,
			QoERating:        d.QoERating,
		})
	}
	return rows, nil
}

// ── Baromètre (IMP-09) ───────────────────────────────────────────────────────

// Barometer situe un résultat parmi les mesures existantes.
//
// Les rangs sont des centiles : DownloadRank = 80 signifie que le débit
// descendant est meilleur que celui de 80 % des tests. Pour la latence,
// « meilleur » veut dire plus basse.
type Barometer struct {
	SampleSize        int     `json:"sampleSize"`
	DownloadRank      float64 `json:"downloadRank"`
	UploadRank        float64 `json:"uploadRank"`
	PingRank          float64 `json:"pingRank"`
	Operator          string  `json:"operator,omitempty"`
	OperatorSample    int     `json:"operatorSampleSize"`
	OperatorAvgDl     float64 `json:"operatorAvgDownload"`
	OperatorRank      float64 `json:"operatorDownloadRank"`
	NationalAvgDl     float64 `json:"nationalAvgDownload"`
	OperatorsAvgDl    map[string]float64 `json:"operatorsAvgDownload"`
}

// barometerWindow limite le calcul aux mesures récentes : le réseau évolue,
// un test d'il y a un an n'est plus un bon repère.
const barometerWindow = 90 * 24 * time.Hour

// FetchBarometer calcule la position d'un résultat (débits en Mb/s, latence
// en ms) parmi les tests actifs aboutis des 90 derniers jours, au niveau
// national et pour son opérateur.
func (m *MongoDB) FetchBarometer(download, upload, ping float64, operator string) (*Barometer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter := speedOnly(bson.M{"timestamp": bson.M{"$gte": time.Now().Add(-barometerWindow)}})
	opts := options.Find().
		SetProjection(bson.M{"dl": 1, "ul": 1, "ping": 1, "operator": 1}).
		SetLimit(20000)
	cursor, err := m.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []Document
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	var dls, uls, pings, opDls []float64
	sums := map[string][2]float64{} // opérateur → (somme, nombre)
	for _, d := range docs {
		dl, _ := strconv.ParseFloat(d.Download, 64)
		ul, _ := strconv.ParseFloat(d.Upload, 64)
		p, _ := strconv.ParseFloat(d.Ping, 64)
		dls = append(dls, dl)
		if ul > 0 {
			uls = append(uls, ul)
		}
		if p > 0 {
			pings = append(pings, p)
		}
		if d.Operator != "" {
			s := sums[d.Operator]
			sums[d.Operator] = [2]float64{s[0] + dl, s[1] + 1}
			if operator != "" && d.Operator == operator {
				opDls = append(opDls, dl)
			}
		}
	}

	b := &Barometer{
		SampleSize:     len(dls),
		DownloadRank:   rankBelow(dls, download),
		UploadRank:     rankBelow(uls, upload),
		PingRank:       100 - rankBelow(pings, ping),
		Operator:       operator,
		OperatorSample: len(opDls),
		OperatorRank:   rankBelow(opDls, download),
		NationalAvgDl:  round2(mean(dls)),
		OperatorAvgDl:  round2(mean(opDls)),
		OperatorsAvgDl: map[string]float64{},
	}
	for op, s := range sums {
		if s[1] >= 5 { // moyenne affichée seulement au-delà de 5 tests
			b.OperatorsAvgDl[op] = round2(s[0] / s[1])
		}
	}
	return b, nil
}

// rankBelow renvoie le pourcentage des valeurs strictement inférieures à v.
func rankBelow(values []float64, v float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := sort.SearchFloat64s(sorted, v)
	return round2(float64(n) * 100 / float64(len(sorted)))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range values {
		s += v
	}
	return s / float64(len(values))
}

// ── Messages « Contactez-nous » (IMP-02) ─────────────────────────────────────

// Feedback est un message envoyé depuis l'écran d'aide de l'application.
type Feedback struct {
	Timestamp  time.Time `bson:"timestamp"   json:"timestamp"`
	Topic      string    `bson:"topic"       json:"topic"`
	Message    string    `bson:"message"     json:"message"`
	Email      string    `bson:"email"       json:"email"`
	DeviceID   string    `bson:"device_id"   json:"deviceId"`
	AppVersion string    `bson:"app_version" json:"appVersion"`
}

// InsertFeedback enregistre un message dans la collection `feedback`.
func (m *MongoDB) InsertFeedback(f *Feedback) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f.Timestamp = time.Now()
	_, err := m.collection.Database().Collection("feedback").InsertOne(ctx, f)
	return err
}
