package repository

import (
	"fmt"

	"github.com/gocql/gocql"
)

// tableStatements contains DDL for all tables (no keyspace DDL — that is built
// dynamically in EnsureSchema so the DC name and RF can be injected).
var tableStatements = []string{

	// Primary messages store: partitioned by conversation for range scans.
	// Clustered DESC so newest messages come first without ORDER BY.
	`CREATE TABLE IF NOT EXISTS chat.messages (
		conversation_id          text,
		created_at               timestamp,
		id                       text,
		sender_id                text,
		type                     text,
		content_text             text,
		content_mentions         list<text>,
		media_url                text,
		media_thumbnail_url      text,
		media_mime_type          text,
		media_file_size          bigint,
		media_file_name          text,
		media_width              int,
		media_height             int,
		reply_to_id              text,
		reply_to_sender_id       text,
		reply_to_preview         text,
		forwarded_from_id        text,
		forwarded_from_sender_id text,
		reactions                text,
		edited                   boolean,
		deleted                  boolean,
		PRIMARY KEY ((conversation_id), created_at, id)
	) WITH CLUSTERING ORDER BY (created_at DESC, id DESC)`,

	// Lookup table: resolve message id → (conversation_id, created_at)
	// needed to build the full primary key for the messages table.
	`CREATE TABLE IF NOT EXISTS chat.messages_by_id (
		id              text PRIMARY KEY,
		conversation_id text,
		created_at      timestamp
	)`,

	// Conversation store: participants and last_message serialised as JSON.
	`CREATE TABLE IF NOT EXISTS chat.conversations (
		id                   text PRIMARY KEY,
		type                 text,
		name                 text,
		avatar_url           text,
		created_by           text,
		participants         text,
		last_message         text,
		only_admins_can_send boolean,
		created_at           timestamp,
		updated_at           timestamp
	)`,

	// Forward index: user_id → set of conversation_ids they belong to.
	`CREATE TABLE IF NOT EXISTS chat.user_conversations (
		user_id         text,
		conversation_id text,
		PRIMARY KEY ((user_id), conversation_id)
	)`,

	// Reverse index: conversation_id → set of user_ids.
	// Required to maintain user_conversations when participants change.
	`CREATE TABLE IF NOT EXISTS chat.conversation_participants (
		conversation_id text,
		user_id         text,
		PRIMARY KEY ((conversation_id), user_id)
	)`,

	// Direct-chat deduplication: sorted pair key → conversation_id.
	`CREATE TABLE IF NOT EXISTS chat.direct_conversations (
		lookup_key      text PRIMARY KEY,
		conversation_id text
	)`,
}

// EnsureSchema creates the keyspace and all tables if they do not already exist.
// It must be called against a session that has no default keyspace set.
// datacenter is the ScyllaDB DC name; rf is the replication factor.
func EnsureSchema(session *gocql.Session, datacenter string, rf int) error {
	var keyspaceDDL string
	if rf == 1 {
		// Single-node dev: SimpleStrategy avoids DC-name dependency.
		keyspaceDDL = `CREATE KEYSPACE IF NOT EXISTS chat` +
			` WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}` +
			` AND durable_writes = true`
	} else {
		keyspaceDDL = fmt.Sprintf(
			`CREATE KEYSPACE IF NOT EXISTS chat`+
				` WITH replication = {'class': 'NetworkTopologyStrategy', '%s': %d}`+
				` AND durable_writes = true`,
			datacenter, rf,
		)
	}
	if err := session.Query(keyspaceDDL).Exec(); err != nil {
		return fmt.Errorf("create keyspace: %w", err)
	}
	for _, stmt := range tableStatements {
		if err := session.Query(stmt).Exec(); err != nil {
			return fmt.Errorf("execute schema statement: %w", err)
		}
	}
	return nil
}
