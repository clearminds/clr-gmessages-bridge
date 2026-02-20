package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/client"
	"github.com/maxghenis/openmessage/internal/db"
)

// Backfill fetches existing conversations and recent messages from
// Google Messages and stores them in the local database.
func (a *App) Backfill() error {
	if a.Client == nil {
		return fmt.Errorf("client not connected")
	}

	a.Logger.Info().Msg("Starting backfill of conversations and messages")

	resp, err := a.Client.GM.ListConversations(100, gmproto.ListConversationsRequest_INBOX)
	if err != nil {
		return fmt.Errorf("list conversations: %w", err)
	}

	convos := resp.GetConversations()
	a.Logger.Info().Int("count", len(convos)).Msg("Fetched conversations")

	for _, conv := range convos {
		if err := a.storeConversation(conv); err != nil {
			a.Logger.Error().Err(err).Str("conv_id", conv.GetConversationID()).Msg("Failed to store conversation")
			continue
		}

		// Fetch recent messages for each conversation
		msgResp, err := a.Client.GM.FetchMessages(conv.GetConversationID(), 20, nil)
		if err != nil {
			a.Logger.Warn().Err(err).Str("conv_id", conv.GetConversationID()).Msg("Failed to fetch messages")
			continue
		}

		for _, msg := range msgResp.GetMessages() {
			a.storeMessage(msg)
		}
	}

	a.Logger.Info().Int("conversations", len(convos)).Msg("Backfill complete")
	return nil
}

// DeepBackfill fetches ALL conversations and ALL messages with pagination.
// Runs in the background and logs progress.
func (a *App) DeepBackfill() {
	if a.Client == nil {
		a.Logger.Error().Msg("Deep backfill: client not connected")
		return
	}

	a.Logger.Info().Msg("Starting deep backfill of all messages")

	totalConvos := 0
	totalMsgs := 0

	// Paginate through all conversations
	var cursor *gmproto.Cursor
	for {
		resp, err := a.Client.GM.ListConversations(100, gmproto.ListConversationsRequest_INBOX)
		if err != nil {
			a.Logger.Error().Err(err).Msg("Deep backfill: list conversations failed")
			break
		}

		convos := resp.GetConversations()
		if len(convos) == 0 {
			break
		}

		for _, conv := range convos {
			if err := a.storeConversation(conv); err != nil {
				a.Logger.Error().Err(err).Str("conv_id", conv.GetConversationID()).Msg("Deep backfill: store conversation failed")
				continue
			}
			totalConvos++

			// Paginate through all messages in this conversation
			n := a.deepBackfillConversation(conv.GetConversationID())
			totalMsgs += n
		}

		cursor = resp.GetCursor()
		if cursor == nil {
			break
		}
		// ListConversations doesn't take a cursor param in the current API,
		// so we only get the first page. Break after first batch.
		break
	}

	a.Logger.Info().
		Int("conversations", totalConvos).
		Int("messages", totalMsgs).
		Msg("Deep backfill complete")
}

// deepBackfillConversation fetches all messages in a conversation using cursor pagination.
func (a *App) deepBackfillConversation(convID string) int {
	total := 0
	var cursor *gmproto.Cursor

	for {
		resp, err := a.Client.GM.FetchMessages(convID, 50, cursor)
		if err != nil {
			a.Logger.Warn().Err(err).Str("conv_id", convID).Msg("Deep backfill: fetch messages failed")
			break
		}

		msgs := resp.GetMessages()
		if len(msgs) == 0 {
			break
		}

		for _, msg := range msgs {
			a.storeMessage(msg)
			total++
		}

		cursor = resp.GetCursor()
		if cursor == nil {
			break
		}

		a.Logger.Debug().
			Str("conv_id", convID).
			Int("batch", len(msgs)).
			Int("total_so_far", total).
			Msg("Deep backfill: fetched message batch")
	}

	if total > 0 {
		a.Logger.Info().
			Str("conv_id", convID).
			Int("messages", total).
			Msg("Deep backfill: conversation complete")
	}

	return total
}

func (a *App) storeConversation(conv *gmproto.Conversation) error {
	participantsJSON := "[]"
	if ps := conv.GetParticipants(); len(ps) > 0 {
		type pInfo struct {
			Name   string `json:"name"`
			Number string `json:"number"`
			IsMe   bool   `json:"is_me,omitempty"`
		}
		var infos []pInfo
		for _, p := range ps {
			info := pInfo{
				Name: p.GetFullName(),
				IsMe: p.GetIsMe(),
			}
			if id := p.GetID(); id != nil {
				info.Number = id.GetNumber()
			}
			if info.Number == "" {
				info.Number = p.GetFormattedNumber()
			}
			infos = append(infos, info)
		}
		if b, err := json.Marshal(infos); err == nil {
			participantsJSON = string(b)
		}
	}

	unread := 0
	if conv.GetUnread() {
		unread = 1
	}

	if err := a.Store.UpsertConversation(&db.Conversation{
		ConversationID: conv.GetConversationID(),
		Name:           conv.GetName(),
		IsGroup:        conv.GetIsGroupChat(),
		Participants:   participantsJSON,
		LastMessageTS:  conv.GetLastMessageTimestamp() / 1000,
		UnreadCount:    unread,
	}); err != nil {
		return err
	}

	if a.Supabase != nil {
		ts := time.UnixMilli(conv.GetLastMessageTimestamp() / 1000)
		go func() {
			if err := a.Supabase.UpsertConversation(
				conv.GetConversationID(), conv.GetName(),
				ts, conv.GetIsGroupChat(), "",
			); err != nil {
				a.Logger.Warn().Err(err).Msg("Supabase backfill conversation sync failed")
			}
		}()
	}

	return nil
}

func (a *App) storeMessage(msg *gmproto.Message) {
	body := client.ExtractMessageBody(msg)
	senderName, senderNumber := client.ExtractSenderInfo(msg)

	status := "unknown"
	if ms := msg.GetMessageStatus(); ms != nil {
		status = ms.GetStatus().String()
	}

	dbMsg := &db.Message{
		MessageID:      msg.GetMessageID(),
		ConversationID: msg.GetConversationID(),
		SenderName:     senderName,
		SenderNumber:   senderNumber,
		Body:           body,
		TimestampMS:    msg.GetTimestamp() / 1000,
		Status:         status,
		IsFromMe:       msg.GetSenderParticipant() != nil && msg.GetSenderParticipant().GetIsMe(),
	}

	if media := client.ExtractMediaInfo(msg); media != nil {
		dbMsg.MediaID = media.MediaID
		dbMsg.MimeType = media.MimeType
		dbMsg.DecryptionKey = hex.EncodeToString(media.DecryptionKey)
	}

	if reactions := client.ExtractReactions(msg); reactions != nil {
		if b, err := json.Marshal(reactions); err == nil {
			dbMsg.Reactions = string(b)
		}
	}
	dbMsg.ReplyToID = client.ExtractReplyToID(msg)

	if err := a.Store.UpsertMessage(dbMsg); err != nil {
		a.Logger.Error().Err(err).Str("msg_id", dbMsg.MessageID).Msg("Failed to store backfill message")
		return
	}

	if a.Supabase != nil {
		ts := time.UnixMilli(dbMsg.TimestampMS)
		go func() {
			if err := a.Supabase.UpsertMessage(
				dbMsg.MessageID, dbMsg.ConversationID,
				dbMsg.SenderName, dbMsg.SenderNumber,
				dbMsg.Body, ts, dbMsg.IsFromMe,
				dbMsg.MimeType, "",
			); err != nil {
				a.Logger.Warn().Err(err).Msg("Supabase backfill message sync failed")
			}
		}()
	}
}
