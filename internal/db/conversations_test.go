package db

import (
	"fmt"
	"testing"
)

func TestUpsertConversation_InsertAndUpdate(t *testing.T) {
	store := newTestStore(t)

	t.Run("insert new conversation", func(t *testing.T) {
		conv := &Conversation{
			ConversationID: "conv-1",
			Name:           "Alice",
			IsGroup:        false,
			Participants:   `[{"name":"Alice","number":"+15551234567"}]`,
			LastMessageTS:  1000,
			UnreadCount:    3,
		}
		if err := store.UpsertConversation(conv); err != nil {
			t.Fatalf("upsert: %v", err)
		}

		got, err := store.GetConversation("conv-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Name != "Alice" {
			t.Errorf("name: got %q, want %q", got.Name, "Alice")
		}
		if got.IsGroup {
			t.Error("is_group: got true, want false")
		}
		if got.Participants != `[{"name":"Alice","number":"+15551234567"}]` {
			t.Errorf("participants: got %q", got.Participants)
		}
		if got.LastMessageTS != 1000 {
			t.Errorf("last_message_ts: got %d, want 1000", got.LastMessageTS)
		}
		if got.UnreadCount != 3 {
			t.Errorf("unread_count: got %d, want 3", got.UnreadCount)
		}
	})

	t.Run("update existing conversation", func(t *testing.T) {
		conv := &Conversation{
			ConversationID: "conv-1",
			Name:           "Alice Smith",
			IsGroup:        false,
			Participants:   `[{"name":"Alice Smith","number":"+15551234567"}]`,
			LastMessageTS:  2000,
			UnreadCount:    0,
		}
		if err := store.UpsertConversation(conv); err != nil {
			t.Fatalf("upsert update: %v", err)
		}

		got, err := store.GetConversation("conv-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Name != "Alice Smith" {
			t.Errorf("name after update: got %q, want %q", got.Name, "Alice Smith")
		}
		if got.LastMessageTS != 2000 {
			t.Errorf("last_message_ts after update: got %d, want 2000", got.LastMessageTS)
		}
		if got.UnreadCount != 0 {
			t.Errorf("unread_count after update: got %d, want 0", got.UnreadCount)
		}
	})

	t.Run("upsert group conversation", func(t *testing.T) {
		conv := &Conversation{
			ConversationID: "group-1",
			Name:           "Family Chat",
			IsGroup:        true,
			Participants:   `[{"name":"Alice"},{"name":"Bob"},{"name":"Charlie"}]`,
			LastMessageTS:  3000,
			UnreadCount:    5,
		}
		if err := store.UpsertConversation(conv); err != nil {
			t.Fatalf("upsert group: %v", err)
		}

		got, err := store.GetConversation("group-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !got.IsGroup {
			t.Error("is_group: got false, want true")
		}
		if got.Name != "Family Chat" {
			t.Errorf("name: got %q, want %q", got.Name, "Family Chat")
		}
	})
}

func TestGetConversation_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetConversation("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent conversation, got nil")
	}
}

func TestListConversations_Ordering(t *testing.T) {
	store := newTestStore(t)

	// Insert conversations with varying timestamps (out of order).
	conversations := []Conversation{
		{ConversationID: "c-old", Name: "Old", LastMessageTS: 1000},
		{ConversationID: "c-new", Name: "New", LastMessageTS: 5000},
		{ConversationID: "c-mid", Name: "Middle", LastMessageTS: 3000},
		{ConversationID: "c-newest", Name: "Newest", LastMessageTS: 8000},
		{ConversationID: "c-ancient", Name: "Ancient", LastMessageTS: 100},
	}
	for i := range conversations {
		if err := store.UpsertConversation(&conversations[i]); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	t.Run("returns all ordered by last_message_ts DESC", func(t *testing.T) {
		got, err := store.ListConversations(100)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("count: got %d, want 5", len(got))
		}
		expectedOrder := []string{"Newest", "New", "Middle", "Old", "Ancient"}
		for i, name := range expectedOrder {
			if got[i].Name != name {
				t.Errorf("position %d: got %q, want %q", i, got[i].Name, name)
			}
		}
	})

	t.Run("limit constrains results", func(t *testing.T) {
		got, err := store.ListConversations(2)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("count: got %d, want 2", len(got))
		}
		if got[0].Name != "Newest" {
			t.Errorf("first: got %q, want Newest", got[0].Name)
		}
		if got[1].Name != "New" {
			t.Errorf("second: got %q, want New", got[1].Name)
		}
	})

	t.Run("limit zero returns empty", func(t *testing.T) {
		got, err := store.ListConversations(0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("count: got %d, want 0", len(got))
		}
	})
}

func TestListConversations_Empty(t *testing.T) {
	store := newTestStore(t)

	got, err := store.ListConversations(100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("count: got %d, want 0", len(got))
	}
}

func TestUpdateConversationTimestamp(t *testing.T) {
	store := newTestStore(t)

	store.UpsertConversation(&Conversation{
		ConversationID: "conv-ts",
		Name:           "Timestamp Test",
		LastMessageTS:  1000,
	})

	t.Run("updates timestamp", func(t *testing.T) {
		if err := store.UpdateConversationTimestamp("conv-ts", 9999); err != nil {
			t.Fatalf("update ts: %v", err)
		}

		got, err := store.GetConversation("conv-ts")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.LastMessageTS != 9999 {
			t.Errorf("last_message_ts: got %d, want 9999", got.LastMessageTS)
		}
	})

	t.Run("update on nonexistent conversation does not error", func(t *testing.T) {
		// SQLite UPDATE with no matching rows is not an error.
		if err := store.UpdateConversationTimestamp("no-such-id", 5000); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestListConversations_AfterTimestampUpdate(t *testing.T) {
	store := newTestStore(t)

	// Insert two conversations.
	store.UpsertConversation(&Conversation{ConversationID: "c1", Name: "First", LastMessageTS: 1000})
	store.UpsertConversation(&Conversation{ConversationID: "c2", Name: "Second", LastMessageTS: 2000})

	// Verify initial ordering.
	got, _ := store.ListConversations(100)
	if got[0].Name != "Second" {
		t.Fatalf("initial first: got %q, want Second", got[0].Name)
	}

	// Update first conversation to have the latest timestamp.
	store.UpdateConversationTimestamp("c1", 5000)

	got, _ = store.ListConversations(100)
	if got[0].Name != "First" {
		t.Errorf("after update first: got %q, want First", got[0].Name)
	}
}

func TestUpsertConversation_DefaultValues(t *testing.T) {
	store := newTestStore(t)

	// Insert with minimal fields.
	conv := &Conversation{
		ConversationID: "minimal",
	}
	if err := store.UpsertConversation(conv); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetConversation("minimal")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "" {
		t.Errorf("name: got %q, want empty", got.Name)
	}
	if got.IsGroup {
		t.Error("is_group: got true, want false")
	}
	if got.LastMessageTS != 0 {
		t.Errorf("last_message_ts: got %d, want 0", got.LastMessageTS)
	}
	if got.UnreadCount != 0 {
		t.Errorf("unread_count: got %d, want 0", got.UnreadCount)
	}
}

func TestListConversations_ManyEntries(t *testing.T) {
	store := newTestStore(t)

	// Insert many conversations.
	for i := 0; i < 50; i++ {
		store.UpsertConversation(&Conversation{
			ConversationID: fmt.Sprintf("conv-%03d", i),
			Name:           fmt.Sprintf("Conv %d", i),
			LastMessageTS:  int64(i * 100),
		})
	}

	t.Run("limit 10 returns 10", func(t *testing.T) {
		got, err := store.ListConversations(10)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 10 {
			t.Errorf("count: got %d, want 10", len(got))
		}
	})

	t.Run("all 50 returned with high limit", func(t *testing.T) {
		got, err := store.ListConversations(1000)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 50 {
			t.Errorf("count: got %d, want 50", len(got))
		}
	})
}
