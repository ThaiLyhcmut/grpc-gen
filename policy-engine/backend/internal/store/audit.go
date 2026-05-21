package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// AuditEntry records a single policy mutation.
// ID is bson.ObjectID so the mongo driver writes/reads it natively; JSON
// marshalling emits the 24-char hex string the UI expects.
type AuditEntry struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Timestamp time.Time     `bson:"timestamp" json:"timestamp"`
	Actor     string        `bson:"actor" json:"actor"`   // admin user id / email
	Action    string        `bson:"action" json:"action"` // add | remove | update
	OldPolicy *PolicyTuple  `bson:"old_policy,omitempty" json:"old_policy,omitempty"`
	NewPolicy *PolicyTuple  `bson:"new_policy,omitempty" json:"new_policy,omitempty"`
}

// PolicyTuple is the 4-field key Casbin uses.
type PolicyTuple struct {
	Sub       string `bson:"sub" json:"sub"`
	Obj       string `bson:"obj" json:"obj"`
	Act       string `bson:"act" json:"act"`
	Condition string `bson:"condition" json:"condition"`
}

// AuditStore writes/reads audit entries from MongoDB.
type AuditStore struct {
	col *mongo.Collection
}

func NewAuditStore(db *mongo.Database, collection string) *AuditStore {
	if collection == "" {
		collection = "policy_audit"
	}
	return &AuditStore{col: db.Collection(collection)}
}

func (a *AuditStore) Log(ctx context.Context, entry AuditEntry) error {
	entry.Timestamp = time.Now().UTC()
	_, err := a.col.InsertOne(ctx, entry)
	return err
}

func (a *AuditStore) List(ctx context.Context, limit int64) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	cursor, err := a.col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var out []AuditEntry
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
