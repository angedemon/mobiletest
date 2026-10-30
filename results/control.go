package results

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/mongodb"
	log "github.com/sirupsen/logrus"
)

// Mode Contrôle (drive test / walk test de l'autorité).
//
// Seuls les comptes listés dans `control_agents` (SPEEDTEST_CONTROL_AGENTS,
// e-mails séparés par des virgules) peuvent envoyer ou lire des mesures de
// contrôle. Elles sont stockées dans une collection séparée, jamais exposée
// par les routes publiques.

var campaignRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// maxControlPayload : une mesure fait quelques centaines d'octets ; 16 Kio
// laisse une large marge sans permettre d'abus.
const maxControlPayload = 16 << 10

// isControlAgent indique si l'e-mail figure dans la liste des agents
// habilités (comparaison insensible à la casse et aux espaces).
func isControlAgent(email, agents string) bool {
	email = mongodb.NormalizeEmail(email)
	if email == "" {
		return false
	}
	for _, a := range strings.Split(agents, ",") {
		if mongodb.NormalizeEmail(a) == email {
			return true
		}
	}
	return false
}

// controlAgent renvoie le compte de l'agent connecté, ou écrit 401 / 403.
func controlAgent(w http.ResponseWriter, r *http.Request) *mongodb.User {
	u := currentUser(w, r)
	if u == nil {
		return nil
	}
	if !isControlAgent(u.Email, config.LoadedConfig().ControlAgents) {
		http.Error(w, "not a control agent", http.StatusForbidden)
		return nil
	}
	return u
}

// ControlMe — GET /api/control/me : le compte connecté est-il agent ?
func ControlMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(w, r)
	if u == nil {
		return
	}
	jsonResponse(w, map[string]any{
		"agent": isControlAgent(u.Email, config.LoadedConfig().ControlAgents),
		"email": u.Email,
	})
}

// controlPayload : champs de la mesure lus par le serveur. Le reste du JSON
// est conservé tel quel dans Payload.
type controlPayload struct {
	Campaign   string  `json:"campaign"`
	DeviceID   string  `json:"deviceId"`
	Seq        int64   `json:"seq"`
	MeasuredAt string  `json:"measuredAt"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	SimOperator string `json:"simOperator"`
	Operator   string  `json:"operator"`
}

// parseControlPayload valide une mesure de contrôle brute.
func parseControlPayload(raw []byte) (*controlPayload, string) {
	var p controlPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "invalid json"
	}
	if !campaignRe.MatchString(p.Campaign) {
		return nil, "invalid campaign"
	}
	if p.DeviceID == "" || len(p.DeviceID) > 80 {
		return nil, "invalid deviceId"
	}
	if p.Seq < 1 {
		return nil, "invalid seq"
	}
	if _, err := time.Parse(time.RFC3339, p.MeasuredAt); err != nil {
		return nil, "invalid measuredAt"
	}
	if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
		return nil, "invalid position"
	}
	return &p, ""
}

// ControlRecord — POST /api/control/measurements : une mesure (JSON).
// 201 si enregistrée, 409 si déjà reçue (le téléphone peut alors la retirer
// de sa file d'attente).
func ControlRecord(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	u := controlAgent(w, r)
	if u == nil {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxControlPayload))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	p, problem := parseControlPayload(raw)
	if p == nil {
		http.Error(w, problem, http.StatusBadRequest)
		return
	}
	measuredAt, _ := time.Parse(time.RFC3339, p.MeasuredAt)
	op := p.SimOperator
	if op == "" {
		op = p.Operator
	}
	c := &mongodb.ControlMeasurement{
		Campaign:   p.Campaign,
		DeviceID:   p.DeviceID,
		Seq:        p.Seq,
		AgentID:    u.ID.Hex(),
		AgentEmail: u.Email,
		MeasuredAt: measuredAt.UTC(),
		Latitude:   p.Latitude,
		Longitude:  p.Longitude,
		Operator:   op,
		Payload:    string(raw),
	}
	switch err := database.MongoDB.InsertControl(c); err {
	case nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"hash": c.Hash})
	case mongodb.ErrSeqReplay:
		http.Error(w, "already recorded", http.StatusConflict)
	default:
		log.Errorf("ControlRecord error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// ControlCampaign — GET /api/control/campaign?id=XXX[&limit=N] : mesures
// d'une campagne et vérification de leur chaîne.
func ControlCampaign(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	if controlAgent(w, r) == nil {
		return
	}
	id := r.URL.Query().Get("id")
	if !campaignRe.MatchString(id) {
		http.Error(w, "invalid campaign", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if limit < 1 || limit > 50000 {
		limit = 50000
	}
	list, err := database.MongoDB.FetchControlCampaign(id, limit)
	if err != nil {
		log.Errorf("ControlCampaign error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	broken := mongodb.VerifyControlChain(list)
	jsonResponse(w, map[string]any{
		"campaign":     id,
		"count":        len(list),
		"chainIntact":  len(broken) == 0,
		"brokenCount":  len(broken),
		"measurements": list,
	})
}
