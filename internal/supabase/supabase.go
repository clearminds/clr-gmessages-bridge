package supabase

import (
	"bytes"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Writer handles real-time writes to Supabase via REST APIs.
//
// Data writes use PostgREST RPC functions (/rest/v1/rpc/...).
// Media uploads use Supabase Storage API (/storage/v1/object/...).
// Auto-migration uses direct PostgreSQL (optional, via SUPABASE_DB_URL).
type Writer struct {
	url    string
	key    string
	client *http.Client
}

// NewWriter creates a Supabase writer using REST APIs.
// Requires SUPABASE_URL and SUPABASE_KEY env vars.
// Optionally runs migrations if SUPABASE_DB_URL is also set.
// Returns nil (no error) if SUPABASE_URL/KEY are not set.
func NewWriter() (*Writer, error) {
	url := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_KEY")
	if url == "" || key == "" {
		log.Println("SUPABASE_URL/SUPABASE_KEY not set — Supabase sync disabled")
		return nil, nil
	}

	sw := &Writer{
		url:    strings.TrimRight(url, "/"),
		key:    key,
		client: &http.Client{Timeout: 30 * time.Second},
	}

	// Optional: auto-migrate if SUPABASE_DB_URL is set
	dbURL := os.Getenv("SUPABASE_DB_URL")
	if dbURL != "" {
		if err := sw.runMigrations(dbURL); err != nil {
			return nil, fmt.Errorf("migrations failed: %w", err)
		}
		log.Println("Supabase migrations applied")
	} else {
		log.Println("SUPABASE_DB_URL not set — skipping auto-migration (run SQL manually in Supabase dashboard)")
	}

	// Ensure storage bucket exists
	sw.ensureStorageBucket()

	log.Println("Supabase writer initialized (REST API mode)")
	return sw, nil
}

// Close is a no-op for the REST-based writer (no persistent connections).
func (sw *Writer) Close() error {
	return nil
}

// --- PostgREST RPC Calls ---

// rpc calls a Supabase PostgREST RPC function.
func (sw *Writer) rpc(funcName string, params map[string]interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal RPC params: %w", err)
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/rest/v1/rpc/%s", sw.url, funcName),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", sw.key)
	req.Header.Set("Authorization", "Bearer "+sw.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := sw.client.Do(req)
	if err != nil {
		return fmt.Errorf("RPC %s: %w", funcName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("RPC %s returned %d: %s", funcName, resp.StatusCode, string(respBody))
	}
	return nil
}

// UpsertConversation upserts a conversation via PostgREST RPC.
func (sw *Writer) UpsertConversation(convID, name string, lastMessageTime time.Time, isGroup bool, lastPreview string) error {
	return sw.rpc("upsert_conversation", map[string]interface{}{
		"p_conversation_id":      convID,
		"p_name":                 name,
		"p_last_message_time":    lastMessageTime.Format(time.RFC3339),
		"p_is_group":             isGroup,
		"p_last_message_preview": lastPreview,
	})
}

// UpsertMessage upserts a message via PostgREST RPC.
func (sw *Writer) UpsertMessage(id, conversationID, senderName, senderNumber, content string, timestamp time.Time, isFromMe bool, mediaType, mediaURL string) error {
	if content == "" && mediaType == "" {
		return nil
	}
	return sw.rpc("upsert_message", map[string]interface{}{
		"p_id":              id,
		"p_conversation_id": conversationID,
		"p_sender_name":     senderName,
		"p_sender_number":   senderNumber,
		"p_content":         content,
		"p_timestamp":       timestamp.Format(time.RFC3339),
		"p_is_from_me":      isFromMe,
		"p_media_type":      mediaType,
		"p_media_url":       mediaURL,
	})
}

// UpsertContact upserts a contact via PostgREST RPC.
func (sw *Writer) UpsertContact(number, name string) error {
	return sw.rpc("upsert_contact", map[string]interface{}{
		"p_number": number,
		"p_name":   name,
	})
}

// --- Supabase Storage ---

const storageBucket = "gmessages-media"

// ensureStorageBucket creates the gmessages-media bucket if it doesn't exist.
func (sw *Writer) ensureStorageBucket() {
	body, _ := json.Marshal(map[string]interface{}{
		"id":     storageBucket,
		"name":   storageBucket,
		"public": true,
	})

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/storage/v1/bucket", sw.url),
		bytes.NewReader(body),
	)
	if err != nil {
		log.Printf("Storage bucket request failed: %v", err)
		return
	}
	req.Header.Set("apikey", sw.key)
	req.Header.Set("Authorization", "Bearer "+sw.key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := sw.client.Do(req)
	if err != nil {
		log.Printf("Storage bucket creation failed: %v", err)
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		log.Println("Created gmessages-media storage bucket")
	case 409:
		log.Println("gmessages-media storage bucket exists")
	default:
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("Storage bucket returned %d: %s", resp.StatusCode, string(respBody))
	}
}

// UploadMedia uploads a file to Supabase Storage and returns its public URL.
// path is relative within the bucket (e.g., "conversationid/filename.jpg").
func (sw *Writer) UploadMedia(path string, data []byte, contentType string) (string, error) {
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/storage/v1/object/%s/%s", sw.url, storageBucket, path),
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("apikey", sw.key)
	req.Header.Set("Authorization", "Bearer "+sw.key)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	resp, err := sw.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("media upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("media upload returned %d: %s", resp.StatusCode, string(respBody))
	}

	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", sw.url, storageBucket, path)
	return publicURL, nil
}

// --- Migration Runner (direct PostgreSQL, optional) ---

// runMigrations connects to PostgreSQL directly and applies pending migrations.
func (sw *Writer) runMigrations(dbURL string) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("open DB: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping DB: %w", err)
	}

	db.SetMaxOpenConns(1)

	// Create migrations tracking table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			version INTEGER PRIMARY KEY,
			filename TEXT NOT NULL,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create _migrations table: %w", err)
	}

	// Read embedded migration files
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var version int
		fmt.Sscanf(entry.Name(), "%d", &version)
		if version == 0 {
			log.Printf("Skipping migration with invalid version: %s", entry.Name())
			continue
		}

		var applied bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM _migrations WHERE version = $1)", version).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if applied {
			continue
		}

		content, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		log.Printf("Applying migration %d: %s", version, entry.Name())

		_, err = db.Exec(string(content))
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}

		_, err = db.Exec(
			"INSERT INTO _migrations (version, filename) VALUES ($1, $2)",
			version, entry.Name(),
		)
		if err != nil {
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}

		log.Printf("Migration %d applied", version)
	}

	return nil
}
