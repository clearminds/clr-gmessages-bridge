package cmd

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/tools"
	"github.com/maxghenis/openmessage/internal/web"
)

func RunServe(logger zerolog.Logger) error {
	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	// Connect to Google Messages (skip in demo mode)
	if os.Getenv("OPENMESSAGES_DEMO") == "" {
		if err := a.LoadAndConnect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		// Backfill existing conversations and messages
		go func() {
			if err := a.Backfill(); err != nil {
				logger.Warn().Err(err).Msg("Backfill failed")
			}
		}()
	} else {
		logger.Info().Msg("Demo mode — skipping phone connection")
	}

	// Start web server
	port := os.Getenv("OPENMESSAGES_PORT")
	if port == "" {
		port = "7007"
	}

	// Create MCP server
	mcpSrv := mcpserver.NewMCPServer(
		"openmessage",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
	)
	tools.Register(mcpSrv, a)

	// Create SSE transport for MCP, mounted at /mcp/
	sseSrv := mcpserver.NewSSEServer(mcpSrv,
		mcpserver.WithBaseURL(fmt.Sprintf("http://localhost:%s", port)),
		mcpserver.WithStaticBasePath("/mcp"),
	)

	var mediaUploader web.MediaUploader
	if a.Supabase != nil {
		mediaUploader = func(messageID string) (string, error) {
			msg, err := a.Store.GetMessageByID(messageID)
			if err != nil {
				return "", fmt.Errorf("get message: %w", err)
			}
			if msg == nil || msg.MediaID == "" {
				return "", fmt.Errorf("no media for message %s", messageID)
			}
			key, err := hex.DecodeString(msg.DecryptionKey)
			if err != nil {
				return "", fmt.Errorf("decode key: %w", err)
			}
			data, err := a.Client.GM.DownloadMedia(msg.MediaID, key)
			if err != nil {
				return "", fmt.Errorf("download: %w", err)
			}
			path := fmt.Sprintf("%s/%s", msg.ConversationID, messageID)
			if ext := mimeToExt(msg.MimeType); ext != "" {
				path += ext
			}
			return a.Supabase.UploadMedia(path, data, msg.MimeType)
		}
	}

	httpHandler := web.APIHandlerFull(a.Store, a.Client, logger, sseSrv,
		func() bool { return a.Connected.Load() },
		a.Unpair,
		mediaUploader,
		a.DeepBackfill,
	)
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", port, err)
	}
	go func() {
		logger.Info().Str("port", port).Msg("Web UI available at http://localhost:" + port)
		logger.Info().Str("port", port).Msg("MCP SSE available at http://localhost:" + port + "/mcp/sse")
		if err := http.Serve(ln, httpHandler); err != nil {
			logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Block until signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info().Msg("Shutting down")
	return nil
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	default:
		return ""
	}
}

// LogLevel returns the zerolog level based on OPENMESSAGES_LOG_LEVEL env var.
func LogLevel() zerolog.Level {
	switch os.Getenv("OPENMESSAGES_LOG_LEVEL") {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "trace":
		return zerolog.TraceLevel
	default:
		return zerolog.InfoLevel
	}
}
