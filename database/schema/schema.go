package schema

import (
	"time"
)

type TelemetryData struct {
	Timestamp time.Time
	IPAddress string
	ISPInfo   string
	Extra     string
	UserAgent string
	Language  string
	Download  string
	Upload    string
	Ping      string
	Jitter    string
	Log       string
	UUID      string
	// Compte de l'utilisateur connecté (IMP-03), vide pour un test anonyme.
	// Seul le backend MongoDB l'enregistre.
	UserID string
}
