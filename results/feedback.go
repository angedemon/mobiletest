package results

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/mongodb"
	log "github.com/sirupsen/logrus"
)

// Messages « Contactez-nous » : au plus 10 par IP et par heure.
var feedbackLimiter = newRateLimiter(10, time.Hour)

// Feedback reçoit un message de l'écran d'aide de l'application (IMP-02).
// POST /api/feedback  {topic, message, email, deviceId, appVersion}
func Feedback(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	if !feedbackLimiter.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many messages", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var f mongodb.Feedback
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	f.Message = strings.TrimSpace(f.Message)
	if n := utf8.RuneCountInString(f.Message); n < 5 || n > 2000 {
		http.Error(w, "message length must be 5-2000", http.StatusBadRequest)
		return
	}
	if len(f.Topic) > 40 || len(f.Email) > 120 || len(f.DeviceID) > 64 || len(f.AppVersion) > 20 {
		http.Error(w, "field too long", http.StatusBadRequest)
		return
	}
	if err := database.MongoDB.InsertFeedback(&f); err != nil {
		log.Errorf("Feedback insert error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
