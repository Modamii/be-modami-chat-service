package database

import (
	"context"
	"time"

	"gitlab.com/lifegoeson-libs/pkg-logging/logger"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)
type MongoConfig struct {
	URI        string
	Database   string
	Timeout    time.Duration
	MaxPool    uint64
	MinPool    uint64
	EnableLogging bool
}
type MongoDB struct {
	Client   *mongo.Client
	Database *mongo.Database
}
type MongoLogSink struct {
	ctx context.Context
}

func NewMongoLogSink(ctx context.Context) *MongoLogSink {
	return &MongoLogSink{ctx: ctx}
}

func (m *MongoLogSink) Info(level int, message string, keysAndValues ...interface{}) {
	// l := logger.FromContext(m.ctx)
	// l.Info("[MongoDB] " + message)
	// if len(keysAndValues) > 0 {
	// 	l.Info("[MongoDB] Additional data", logging.String("data", fmt.Sprintf("%+v", keysAndValues)))
	// }
}
func (m *MongoLogSink) Error(err error, message string, keysAndValues ...interface{}) {
	// l := logger.FromContext(m.ctx)
	// l.Error("[MongoDB] Error: "+message, err)
	// if len(keysAndValues) > 0 {
	// 	l.Error("[MongoDB] Additional data", fmt.Errorf("%+v", keysAndValues))
	// }
}
func NewMongoDB(config MongoConfig) (*MongoDB, error) {
	ctx := context.Background()
	l := logger.FromContext(ctx)

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxPool == 0 {
		config.MaxPool = 100
	}
	if config.MinPool == 0 {
		config.MinPool = 10
	}
	var loggerOpts *options.LoggerOptions
	if config.EnableLogging {
		loggerOpts = &options.LoggerOptions{
			ComponentLevels: map[options.LogComponent]options.LogLevel{
				options.LogComponentCommand:         options.LogLevelDebug, // Show all commands
				options.LogComponentTopology:       options.LogLevelInfo,  // Show topology changes
				options.LogComponentServerSelection: options.LogLevelInfo,  // Show server selection
				options.LogComponentConnection:     options.LogLevelInfo,  // Show connection events
			},
			Sink:              NewMongoLogSink(ctx), // Use our custom log sink
			MaxDocumentLength: 1000,                 // Limit document length in logs
		}
	} else {
		loggerOpts = &options.LoggerOptions{
			ComponentLevels: map[options.LogComponent]options.LogLevel{},
		}
	}
	clientOpts := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(config.MaxPool).
		SetMinPoolSize(config.MinPool).
		SetMaxConnIdleTime(5 * time.Minute).
		SetServerSelectionTimeout(config.Timeout).
		SetLoggerOptions(loggerOpts)
	connectCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	client, err := mongo.Connect(connectCtx, clientOpts)
	if err != nil {
		l.Error("Failed to connect to MongoDB", err)
		return nil, err
	}
	if err := client.Ping(connectCtx, readpref.Primary()); err != nil {
		l.Error("Failed to ping MongoDB", err)
		return nil, err
	}
	l.Info("Successfully connected to MongoDB")
	return &MongoDB{
		Client:   client,
		Database: client.Database(config.Database),
	}, nil
}
func (m *MongoDB) Close(ctx context.Context) error {
	l := logger.FromContext(ctx)
	if err := m.Client.Disconnect(ctx); err != nil {
		l.Error("Failed to disconnect from MongoDB", err)
		return err
	}
	l.Info("Disconnected from MongoDB")
	return nil
}
func (m *MongoDB) Ping(ctx context.Context) error {
	return m.Client.Ping(ctx, readpref.Primary())
}
func (m *MongoDB) GetCollection(name string) *mongo.Collection {
	return m.Database.Collection(name)
}
func (m *MongoDB) CreateIndexes(ctx context.Context) error {
	userCollection := m.GetCollection("users")
	userIndexes := []mongo.IndexModel{
		{
			Keys:    map[string]int{"email": 1},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    map[string]int{"username": 1},
			Options: options.Index().SetUnique(true),
		},
	}
	l := logger.FromContext(ctx)
	_, err := userCollection.Indexes().CreateMany(ctx, userIndexes)
	if err != nil {
		l.Error("Failed to create user indexes", err)
		return err
	}
	articleCollection := m.GetCollection("articles")
	articleIndexes := []mongo.IndexModel{
		{
			Keys:    map[string]int{"slug": 1},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: map[string]interface{}{"title": "text", "content": "text"},
		},
		{
			Keys: map[string]int{"author_id": 1},
		},
		{
			Keys: map[string]int{"category_id": 1},
		},
		{
			Keys: map[string]int{"status": 1},
		},
		{
			Keys: map[string]int{"created_at": -1},
		},
	}
	_, err = articleCollection.Indexes().CreateMany(ctx, articleIndexes)
	if err != nil {
		l.Error("Failed to create article indexes", err)
		return err
	}
	categoryCollection := m.GetCollection("categories")
	categoryIndexes := []mongo.IndexModel{
		{
			Keys:    map[string]int{"slug": 1},
			Options: options.Index().SetUnique(true),
		},
	}
	_, err = categoryCollection.Indexes().CreateMany(ctx, categoryIndexes)
	if err != nil {
		l.Error("Failed to create category indexes", err)
		return err
	}
	l.Info("Successfully created MongoDB indexes")
	return nil
}
