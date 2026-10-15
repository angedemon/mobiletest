package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	log "github.com/sirupsen/logrus"
)

// Connexion avec Google (IMP-03).
//
// L'application obtient de Google un « ID token » (JWT signé par Google) et
// l'envoie ici. Le serveur le fait vérifier par Google (point d'accès
// tokeninfo : signature, expiration), puis contrôle lui-même que le jeton a
// bien été émis POUR Yélé (audience = identifiant client OAuth configuré
// dans SPEEDTEST_GOOGLE_CLIENT_ID) et que l'adresse e-mail est vérifiée.
// Sans cette vérification d'audience, un jeton émis pour n'importe quelle
// autre application ouvrirait un compte Yélé.

// GoogleIdentity est l'identité extraite d'un jeton Google vérifié.
type GoogleIdentity struct {
	Sub   string
	Email string
	Name  string
}

var errGoogleToken = errors.New("jeton Google invalide")

// googleTokenInfo est remplaçable dans les tests (pas d'appel réseau).
var googleTokenInfo = fetchGoogleTokenInfo

var googleHTTP = &http.Client{Timeout: 10 * time.Second}

func fetchGoogleTokenInfo(idToken string) (map[string]any, error) {
	resp, err := googleHTTP.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" +
		url.QueryEscape(idToken))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errGoogleToken // jeton expiré, falsifié ou mal formé
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return info, nil
}

// verifyGoogleToken valide un ID token pour les identifiants clients
// autorisés (liste séparée par des virgules).
func verifyGoogleToken(idToken, allowedClientIDs string, now time.Time) (*GoogleIdentity, error) {
	if idToken == "" || allowedClientIDs == "" {
		return nil, errGoogleToken
	}
	info, err := googleTokenInfo(idToken)
	if err != nil {
		return nil, err
	}
	str := func(k string) string { s, _ := info[k].(string); return s }

	audOK := false
	for _, id := range strings.Split(allowedClientIDs, ",") {
		if id = strings.TrimSpace(id); id != "" && id == str("aud") {
			audOK = true
		}
	}
	if !audOK {
		return nil, fmt.Errorf("%w : audience inattendue", errGoogleToken)
	}
	if iss := str("iss"); iss != "accounts.google.com" && iss != "https://accounts.google.com" {
		return nil, fmt.Errorf("%w : émetteur inattendu", errGoogleToken)
	}
	if exp, _ := strconv.ParseInt(str("exp"), 10, 64); exp == 0 || now.Unix() >= exp {
		return nil, fmt.Errorf("%w : jeton expiré", errGoogleToken)
	}
	if str("email_verified") != "true" || str("email") == "" || str("sub") == "" {
		return nil, fmt.Errorf("%w : e-mail non vérifié", errGoogleToken)
	}
	return &GoogleIdentity{Sub: str("sub"), Email: str("email"), Name: str("name")}, nil
}

// GoogleLogin — POST /api/auth/google {idToken}
// Crée le compte au premier passage, le retrouve ensuite ; renvoie une
// session Yélé, comme /api/auth/login.
func GoogleLogin(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	clientIDs := config.LoadedConfig().GoogleClientID
	if clientIDs == "" {
		http.Error(w, "google sign-in not configured", http.StatusServiceUnavailable)
		return
	}
	if !authLimiter.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var b struct {
		IDToken string `json:"idToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	id, err := verifyGoogleToken(b.IDToken, clientIDs, time.Now())
	if err != nil {
		log.Warnf("Google sign-in refusé : %s", err)
		http.Error(w, "invalid google token", http.StatusUnauthorized)
		return
	}
	u, token, err := database.MongoDB.GoogleUser(id.Sub, id.Email, id.Name)
	if err != nil {
		log.Errorf("GoogleUser error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, authResponse{Token: token, User: u})
}
