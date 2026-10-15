package mongodb

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Comptes utilisateurs (IMP-03).
//
// Mots de passe : PBKDF2-SHA256, 600 000 itérations, sel aléatoire par
// compte (recommandation OWASP 2023). Sessions : jeton aléatoire de 32
// octets remis à l'app ; seule son empreinte SHA-256 est stockée, de sorte
// qu'une fuite de la base ne livre aucun jeton utilisable.

var (
	ErrEmailTaken = errors.New("email déjà utilisé")
	ErrBadLogin   = errors.New("email ou mot de passe incorrect")
	ErrNoSession  = errors.New("session invalide ou expirée")
)

const (
	pbkdf2Iterations = 600_000
	sessionLifetime  = 90 * 24 * time.Hour
)

// User est un compte. Le hachage et le sel ne sortent jamais en JSON.
type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email        string             `bson:"email"         json:"email"`
	Name         string             `bson:"name"          json:"name"`
	PasswordHash string             `bson:"password_hash" json:"-"`
	// Identifiant Google (« sub ») du compte, s'il s'est connecté avec Google.
	GoogleSub string `bson:"google_sub,omitempty" json:"-"`
	Salt         string             `bson:"salt"          json:"-"`
	CreatedAt    time.Time          `bson:"created_at"    json:"createdAt"`
}

type session struct {
	TokenHash string             `bson:"token_hash"`
	UserID    primitive.ObjectID `bson:"user_id"`
	CreatedAt time.Time          `bson:"created_at"`
	ExpiresAt time.Time          `bson:"expires_at"`
}

func (m *MongoDB) users() *mongo.Collection    { return m.collection.Database().Collection("users") }
func (m *MongoDB) sessions() *mongo.Collection { return m.collection.Database().Collection("sessions") }

// EnsureAccountIndexes crée les index des comptes (e-mail unique, sessions
// expirées supprimées automatiquement par MongoDB).
func (m *MongoDB) EnsureAccountIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.users().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "google_sub", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true)},
	}); err != nil {
		return err
	}
	_, err := m.sessions().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
	return err
}

func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func hashPassword(password string, salt []byte) (string, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateUser crée un compte et ouvre une session ; renvoie le jeton.
func (m *MongoDB) CreateUser(email, password, name string) (*User, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	saltHex, err := randomHex(16)
	if err != nil {
		return nil, "", err
	}
	salt, _ := hex.DecodeString(saltHex)
	hash, err := hashPassword(password, salt)
	if err != nil {
		return nil, "", err
	}
	u := &User{
		Email: NormalizeEmail(email), Name: strings.TrimSpace(name),
		PasswordHash: hash, Salt: saltHex, CreatedAt: time.Now(),
	}
	res, err := m.users().InsertOne(ctx, u)
	if mongo.IsDuplicateKeyError(err) {
		return nil, "", ErrEmailTaken
	}
	if err != nil {
		return nil, "", err
	}
	u.ID = res.InsertedID.(primitive.ObjectID)
	token, err := m.openSession(ctx, u.ID)
	return u, token, err
}

// GoogleUser retrouve (ou crée) le compte d'une identité Google vérifiée et
// ouvre une session. Ordre de recherche : identifiant Google, puis e-mail —
// un compte créé avec e-mail et mot de passe est alors relié à Google, ce
// qui est sûr puisque Google a vérifié la propriété de l'adresse.
func (m *MongoDB) GoogleUser(sub, email, name string) (*User, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var u User
	err := m.users().FindOne(ctx, bson.M{"google_sub": sub}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		err = m.users().FindOne(ctx, bson.M{"email": NormalizeEmail(email)}).Decode(&u)
		if err == nil {
			_, err = m.users().UpdateByID(ctx, u.ID, bson.M{"$set": bson.M{"google_sub": sub}})
		} else if err == mongo.ErrNoDocuments {
			u = User{
				Email: NormalizeEmail(email), Name: strings.TrimSpace(name),
				GoogleSub: sub, CreatedAt: time.Now(),
			}
			var res *mongo.InsertOneResult
			res, err = m.users().InsertOne(ctx, &u)
			if err == nil {
				u.ID = res.InsertedID.(primitive.ObjectID)
			}
		}
	}
	if err != nil {
		return nil, "", err
	}
	token, err := m.openSession(ctx, u.ID)
	return &u, token, err
}

// Login vérifie le mot de passe et ouvre une session.
func (m *MongoDB) Login(email, password string) (*User, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var u User
	err := m.users().FindOne(ctx, bson.M{"email": NormalizeEmail(email)}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		// Calcul factice : même durée de réponse qu'un mauvais mot de passe,
		// pour ne pas révéler quels e-mails ont un compte.
		_, _ = hashPassword(password, []byte("yele-dummy-salt!"))
		return nil, "", ErrBadLogin
	}
	if err != nil {
		return nil, "", err
	}
	if u.PasswordHash == "" { // compte créé avec Google, sans mot de passe
		return nil, "", ErrBadLogin
	}
	salt, _ := hex.DecodeString(u.Salt)
	hash, err := hashPassword(password, salt)
	if err != nil {
		return nil, "", err
	}
	if subtle.ConstantTimeCompare([]byte(hash), []byte(u.PasswordHash)) != 1 {
		return nil, "", ErrBadLogin
	}
	token, err := m.openSession(ctx, u.ID)
	return &u, token, err
}

func (m *MongoDB) openSession(ctx context.Context, userID primitive.ObjectID) (string, error) {
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	now := time.Now()
	_, err = m.sessions().InsertOne(ctx, session{
		TokenHash: tokenHash(token), UserID: userID,
		CreatedAt: now, ExpiresAt: now.Add(sessionLifetime),
	})
	return token, err
}

// UserForToken renvoie le compte d'une session valide.
func (m *MongoDB) UserForToken(token string) (*User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var s session
	err := m.sessions().FindOne(ctx, bson.M{
		"token_hash": tokenHash(token),
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&s)
	if err != nil {
		return nil, ErrNoSession
	}
	var u User
	if err := m.users().FindOne(ctx, bson.M{"_id": s.UserID}).Decode(&u); err != nil {
		return nil, ErrNoSession
	}
	return &u, nil
}

// Logout ferme la session du jeton.
func (m *MongoDB) Logout(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := m.sessions().DeleteOne(ctx, bson.M{"token_hash": tokenHash(token)})
	return err
}

// UpdateProfile modifie le nom affiché.
func (m *MongoDB) UpdateProfile(id primitive.ObjectID, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := m.users().UpdateByID(ctx, id, bson.M{"$set": bson.M{"name": strings.TrimSpace(name)}})
	return err
}

// DeleteUser supprime le compte et ses sessions. Les mesures restent, mais
// détachées du compte : elles deviennent anonymes.
func (m *MongoDB) DeleteUser(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := m.sessions().DeleteMany(ctx, bson.M{"user_id": id}); err != nil {
		return err
	}
	if _, err := m.collection.UpdateMany(ctx, bson.M{"user_id": id.Hex()},
		bson.M{"$unset": bson.M{"user_id": ""}}); err != nil {
		return err
	}
	_, err := m.users().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// UserResults renvoie les mesures d'un compte, des plus récentes aux plus
// anciennes : chaque compte ne voit que les siennes.
func (m *MongoDB) UserResults(userID string, page, limit int) (*ResultsPage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := bson.M{"user_id": userID}
	total, _ := m.collection.CountDocuments(ctx, filter)
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).SetLimit(int64(limit))
	cursor, err := m.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []Document
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	return &ResultsPage{Data: docs, Total: total, Page: page, Limit: limit}, nil
}
