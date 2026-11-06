package mongodb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Mode Contrôle : mesures de l'autorité (drive test / walk test).
//
// Elles vivent dans une collection À PART (`control_measurements`) : aucune
// requête publique (carte, baromètre, statistiques, export) ne lit cette
// collection, et les mesures citoyennes n'y entrent jamais. C'est la règle de
// séparation de la note de projet « Yélé Contrôle ».
//
// Chaque mesure est chaînée à la précédente du même téléphone dans la même
// campagne : Hash = SHA-256(PrevHash | Payload). Supprimer, insérer ou
// modifier une mesure après coup casse la chaîne, ce que VerifyControlChain
// détecte.

// ErrSeqReplay : numéro de cycle déjà reçu pour ce téléphone et cette
// campagne (renvoi d'une mesure déjà enregistrée).
var ErrSeqReplay = errors.New("mesure déjà enregistrée")

// ControlMeasurement est une mesure de contrôle telle que stockée.
type ControlMeasurement struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Campaign   string             `bson:"campaign"      json:"campaign"`
	DeviceID   string             `bson:"device_id"     json:"deviceId"`
	Seq        int64              `bson:"seq"           json:"seq"`
	AgentID    string             `bson:"agent_id"      json:"agentId"`
	AgentEmail string             `bson:"agent_email"   json:"agentEmail"`
	ReceivedAt time.Time          `bson:"received_at"   json:"receivedAt"`
	MeasuredAt time.Time          `bson:"measured_at"   json:"measuredAt"`
	Latitude   float64            `bson:"latitude"      json:"latitude"`
	Longitude  float64            `bson:"longitude"     json:"longitude"`
	Operator   string             `bson:"operator"      json:"operator"`
	// Payload : la mesure complète, exactement telle qu'envoyée par le
	// téléphone (JSON). C'est elle qui est chaînée.
	Payload  string `bson:"payload"   json:"payload"`
	PrevHash string `bson:"prev_hash" json:"prevHash"`
	Hash     string `bson:"hash"      json:"hash"`
}

func (m *MongoDB) controls() *mongo.Collection {
	return m.collection.Database().Collection("control_measurements")
}

// EnsureControlIndexes : un seul enregistrement par (campagne, téléphone,
// cycle), et lecture rapide d'une campagne.
func (m *MongoDB) EnsureControlIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := m.controls().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "campaign", Value: 1}, {Key: "device_id", Value: 1}, {Key: "seq", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "campaign", Value: 1}, {Key: "measured_at", Value: 1}}},
	})
	return err
}

// ChainHash calcule l'empreinte d'une mesure à partir de celle de la
// précédente. La première mesure d'une chaîne a un PrevHash vide.
func ChainHash(prevHash, payload string) string {
	sum := sha256.Sum256([]byte(prevHash + "|" + payload))
	return hex.EncodeToString(sum[:])
}

// InsertControl enregistre une mesure de contrôle et la chaîne à la
// dernière mesure du même téléphone dans la même campagne.
func (m *MongoDB) InsertControl(c *ControlMeasurement) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var last ControlMeasurement
	err := m.controls().FindOne(ctx,
		bson.M{"campaign": c.Campaign, "device_id": c.DeviceID},
		options.FindOne().SetSort(bson.D{{Key: "seq", Value: -1}}),
	).Decode(&last)
	switch {
	case err == mongo.ErrNoDocuments:
		c.PrevHash = ""
	case err != nil:
		return err
	case c.Seq <= last.Seq:
		return ErrSeqReplay
	default:
		c.PrevHash = last.Hash
	}
	c.Hash = ChainHash(c.PrevHash, c.Payload)
	c.ReceivedAt = time.Now().UTC()

	_, err = m.controls().InsertOne(ctx, c)
	if mongo.IsDuplicateKeyError(err) {
		return ErrSeqReplay
	}
	return err
}

// FetchControlCampaign renvoie les mesures d'une campagne, dans l'ordre de
// mesure.
func (m *MongoDB) FetchControlCampaign(campaign string, limit int64) ([]ControlMeasurement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cursor, err := m.controls().Find(ctx, bson.M{"campaign": campaign},
		options.Find().
			SetSort(bson.D{{Key: "device_id", Value: 1}, {Key: "seq", Value: 1}}).
			SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := []ControlMeasurement{}
	err = cursor.All(ctx, &out)
	return out, err
}

// VerifyControlChain vérifie, téléphone par téléphone, que chaque mesure
// est bien chaînée à la précédente. Les mesures doivent être triées par
// téléphone puis par cycle (ordre de FetchControlCampaign). Renvoie la liste
// des mesures dont l'empreinte ne correspond pas.
func VerifyControlChain(list []ControlMeasurement) []ControlMeasurement {
	var broken []ControlMeasurement
	prev := map[string]string{}
	for _, c := range list {
		if c.PrevHash != prev[c.DeviceID] || c.Hash != ChainHash(c.PrevHash, c.Payload) {
			broken = append(broken, c)
		}
		prev[c.DeviceID] = c.Hash
	}
	return broken
}

// ControlCampaignSummary résume une campagne pour la liste du tableau de bord.
type ControlCampaignSummary struct {
	Campaign     string    `bson:"_id"           json:"campaign"`
	Count        int64     `bson:"count"         json:"count"`
	Devices      []string  `bson:"devices"       json:"devices"`
	Operators    []string  `bson:"operators"     json:"operators"`
	Agents       []string  `bson:"agents"        json:"agents"`
	FirstAt      time.Time `bson:"first_at"      json:"firstAt"`
	LastAt       time.Time `bson:"last_at"       json:"lastAt"`
	LastReceived time.Time `bson:"last_received" json:"lastReceived"`
}

// FetchControlCampaigns liste les campagnes, la plus récente en premier.
func (m *MongoDB) FetchControlCampaigns() ([]ControlCampaignSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":           "$campaign",
			"count":         bson.M{"$sum": 1},
			"devices":       bson.M{"$addToSet": "$device_id"},
			"operators":     bson.M{"$addToSet": "$operator"},
			"agents":        bson.M{"$addToSet": "$agent_email"},
			"first_at":      bson.M{"$min": "$measured_at"},
			"last_at":       bson.M{"$max": "$measured_at"},
			"last_received": bson.M{"$max": "$received_at"},
		}}},
		{{Key: "$sort", Value: bson.M{"last_at": -1}}},
		{{Key: "$limit", Value: 200}},
	}
	cursor, err := m.controls().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := []ControlCampaignSummary{}
	err = cursor.All(ctx, &out)
	return out, err
}
