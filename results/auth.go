package results

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/mongodb"
	log "github.com/sirupsen/logrus"
)

// Comptes utilisateurs (IMP-03) : inscription, connexion, déconnexion,
// profil et mesures du compte. Le jeton de session circule dans l'en-tête
// « Authorization: Bearer <jeton> ».

// 20 tentatives de connexion ou d'inscription par IP et par 10 minutes :
// freine les essais de mots de passe en série.
var authLimiter = newRateLimiter(20, 10*time.Minute)

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func clientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if i := strings.IndexByte(ip, ','); i >= 0 {
		ip = ip[:i]
	}
	if ip = strings.TrimSpace(ip); ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	return ip
}

type authBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type authResponse struct {
	Token string        `json:"token"`
	User  *mongodb.User `json:"user"`
}

func readAuthBody(w http.ResponseWriter, r *http.Request) (*authBody, bool) {
	if !authLimiter.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var b authBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return nil, false
	}
	return &b, true
}

func validEmail(e string) bool {
	e = mongodb.NormalizeEmail(e)
	at := strings.IndexByte(e, '@')
	return len(e) <= 120 && at > 0 && strings.Contains(e[at:], ".") &&
		!strings.ContainsAny(e, " <>")
}

// Register — POST /api/auth/register {email, password, name}
func Register(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	b, ok := readAuthBody(w, r)
	if !ok {
		return
	}
	if !validEmail(b.Email) {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}
	if n := utf8.RuneCountInString(b.Password); n < 8 || n > 128 {
		http.Error(w, "password must be 8-128 characters", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(b.Name) > 60 {
		http.Error(w, "name too long", http.StatusBadRequest)
		return
	}
	u, token, err := database.MongoDB.CreateUser(b.Email, b.Password, b.Name)
	if err == mongodb.ErrEmailTaken {
		http.Error(w, "email already registered", http.StatusConflict)
		return
	}
	if err != nil {
		log.Errorf("Register error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(authResponse{Token: token, User: u})
}

// Login — POST /api/auth/login {email, password}
func Login(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	b, ok := readAuthBody(w, r)
	if !ok {
		return
	}
	u, token, err := database.MongoDB.Login(b.Email, b.Password)
	if err == mongodb.ErrBadLogin {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Errorf("Login error: %s", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, authResponse{Token: token, User: u})
}

// currentUser renvoie le compte du jeton, ou écrit 401.
func currentUser(w http.ResponseWriter, r *http.Request) *mongodb.User {
	if !mongoRequired(w) {
		return nil
	}
	u, err := database.MongoDB.UserForToken(bearerToken(r))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}
	return u
}

// Logout — POST /api/auth/logout
func Logout(w http.ResponseWriter, r *http.Request) {
	if !mongoRequired(w) {
		return
	}
	_ = database.MongoDB.Logout(bearerToken(r))
	w.WriteHeader(http.StatusNoContent)
}

// Me — GET (profil), PUT {name} (modification), DELETE (suppression du compte)
func Me(w http.ResponseWriter, r *http.Request) {
	u := currentUser(w, r)
	if u == nil {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonResponse(w, u)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		var b authBody
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil ||
			utf8.RuneCountInString(b.Name) > 60 {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := database.MongoDB.UpdateProfile(u.ID, b.Name); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		u.Name = strings.TrimSpace(b.Name)
		jsonResponse(w, u)
	case http.MethodDelete:
		if err := database.MongoDB.DeleteUser(u.ID); err != nil {
			log.Errorf("DeleteUser error: %s", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// MyResults — GET /api/me/results?page=1&limit=20 : mesures du compte seul.
func MyResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(w, r)
	if u == nil {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	res, err := database.MongoDB.UserResults(u.ID.Hex(), page, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, res)
}
