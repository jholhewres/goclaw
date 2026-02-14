// Package copilot – media_tools.go registers tools for image understanding
// (describe_image) and audio transcription (transcribe_audio).
// Also used internally when processing incoming media messages.
package copilot

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
)

// RegisterMediaTools registers describe_image and transcribe_audio tools
// when the LLM client and config support them.
func RegisterMediaTools(executor *ToolExecutor, llmClient *LLMClient, cfg *Config, logger *slog.Logger) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	media := cfg.Media.Effective()

	if media.VisionEnabled && llmClient != nil {
		registerDescribeImageTool(executor, llmClient, media, logger)
	}

	if media.TranscriptionEnabled && llmClient != nil {
		registerTranscribeAudioTool(executor, llmClient, media, logger)
	}
}

func registerDescribeImageTool(executor *ToolExecutor, llm *LLMClient, media MediaConfig, logger *slog.Logger) {
	executor.Register(
		MakeToolDefinition("describe_image", "Describe or analyze an image. Takes a base64-encoded image and returns a description. Use when the user shares an image or when you need to understand visual content.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image_base64": map[string]any{
					"type":        "string",
					"description": "Base64-encoded image data (without data URL prefix)",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Optional question or instruction about the image (e.g. 'What is in this image?', 'Extract the text')",
				},
				"detail": map[string]any{
					"type":        "string",
					"description": "Vision detail level: 'auto', 'low', or 'high'. Default: auto",
					"enum":        []string{"auto", "low", "high"},
				},
				"mime_type": map[string]any{
					"type":        "string",
					"description": "MIME type of the image (e.g. image/jpeg, image/png). Default: image/jpeg",
				},
			},
			"required": []string{"image_base64"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			imageBase64, _ := args["image_base64"].(string)
			if imageBase64 == "" {
				return nil, fmt.Errorf("image_base64 is required")
			}

			// Decode to verify and check size.
			decoded, err := base64.StdEncoding.DecodeString(imageBase64)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 image: %w", err)
			}

			if int64(len(decoded)) > media.MaxImageSize {
				return nil, fmt.Errorf("image too large: %d bytes (max %d)", len(decoded), media.MaxImageSize)
			}

			prompt, _ := args["prompt"].(string)
			if prompt == "" {
				prompt = "Describe this image in detail. Include any text visible in the image."
			}

			detail, _ := args["detail"].(string)
			if detail == "" {
				detail = media.VisionDetail
			}
			if detail == "" {
				detail = "auto"
			}

			mimeType, _ := args["mime_type"].(string)
			if mimeType == "" {
				mimeType = "image/jpeg"
			}

			logger.Debug("describing image",
				"size_bytes", len(decoded),
				"prompt", truncate(prompt, 50),
			)

			desc, err := llm.CompleteWithVision(ctx, "", imageBase64, mimeType, prompt, detail)
			if err != nil {
				logger.Error("vision API failed", "error", err)
				return nil, fmt.Errorf("vision API: %w", err)
			}

			return desc, nil
		},
	)
	logger.Debug("registered describe_image tool")
}

func registerTranscribeAudioTool(executor *ToolExecutor, llm *LLMClient, media MediaConfig, logger *slog.Logger) {
	executor.Register(
		MakeToolDefinition("transcribe_audio", "Transcribe audio/voice to text using the Whisper API. Takes base64-encoded audio data. Use when the user shares voice notes or audio files.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"audio_base64": map[string]any{
					"type":        "string",
					"description": "Base64-encoded audio data",
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Filename hint for format (e.g. audio.ogg, voice.mp3, recording.webm). Helps the API detect format.",
				},
			},
			"required": []string{"audio_base64"},
		}),
		func(ctx context.Context, args map[string]any) (any, error) {
			audioBase64, _ := args["audio_base64"].(string)
			if audioBase64 == "" {
				return nil, fmt.Errorf("audio_base64 is required")
			}

			decoded, err := base64.StdEncoding.DecodeString(audioBase64)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 audio: %w", err)
			}

			if int64(len(decoded)) > media.MaxAudioSize {
				return nil, fmt.Errorf("audio too large: %d bytes (max %d)", len(decoded), media.MaxAudioSize)
			}

			filename, _ := args["filename"].(string)
			if filename == "" {
				filename = "audio.webm"
			}

			logger.Debug("transcribing audio",
				"size_bytes", len(decoded),
				"filename", filename,
			)

			transcript, err := llm.TranscribeAudio(ctx, decoded, filename, media.TranscriptionModel, media)
			if err != nil {
				logger.Error("transcription failed", "error", err)
				return nil, fmt.Errorf("transcription: %w", err)
			}

			return transcript, nil
		},
	)
	logger.Debug("registered transcribe_audio tool")
}
