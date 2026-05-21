package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ConnectMongo dials the given URI and pings to verify connectivity.
func ConnectMongo(ctx context.Context, uri string) (*mongo.Client, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pctx, nil); err != nil {
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client, nil
}
