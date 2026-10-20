package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/pkg/agentlimits"
	"image"
	"mime"
	"strings"
)

var ErrAgentImageResultTooLarge = errors.New("AGENT_IMAGE_RESULT_TOO_LARGE")
var ErrAgentImageResultInvalid = errors.New("AGENT_IMAGE_RESULT_INVALID")

// Decode one accepted Agent result, independently of the public batch JSONL
// envelope. URL downloads retain the same redirect, SSRF and credential rules.
func ResolveAgentImageResult(ctx context.Context, raw json.RawMessage) ([]byte, error) {
	data, _, err := ResolveAgentImageResultWithSource(ctx, raw)
	return data, err
}
func ResolveAgentImageResultWithSource(ctx context.Context, raw json.RawMessage) ([]byte, bool, error) {
	if len(raw) > agentlimits.ImageResponseBytes {
		return nil, false, ErrAgentImageResultTooLarge
	}
	var response openAIBatchImageResponse
	if err := json.Unmarshal(raw, &response); err != nil || response.Error != nil || len(response.Data) != 1 {
		return nil, false, ErrAgentImageResultInvalid
	}
	entry := response.Data[0]
	var data []byte
	var err error
	declared := ""
	if encoded := strings.TrimSpace(entry.B64JSON); encoded != "" {
		if len(encoded) > base64.StdEncoding.EncodedLen(agentlimits.GeneratedImageBytes) {
			return nil, false, ErrAgentImageResultTooLarge
		}
		data, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, false, ErrAgentImageResultInvalid
		}
	} else if strings.TrimSpace(entry.URL) != "" {
		p := &OpenAIBatchImageProvider{}
		data, declared, err = p.downloadOpenAIImageWithLimit(ctx, entry.URL, agentlimits.GeneratedImageBytes)
		if err != nil {
			switch infraerrors.Reason(err) {
			case "OPENAI_BATCH_IMAGE_TOO_LARGE":
				return nil, false, ErrAgentImageResultTooLarge
			case "OPENAI_BATCH_IMAGE_URL_DENIED", "OPENAI_BATCH_IMAGE_REDIRECT_INVALID", "OPENAI_BATCH_IMAGE_REDIRECT_LIMIT":
				return nil, false, ErrAgentImageResultInvalid
			}
			return nil, false, err
		}
	} else {
		return nil, false, ErrAgentImageResultInvalid
	}
	if len(data) > agentlimits.GeneratedImageBytes {
		return nil, false, ErrAgentImageResultTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > agentlimits.ImageSide || cfg.Height > agentlimits.ImageSide || int64(cfg.Width)*int64(cfg.Height) > agentlimits.ImagePixels {
		return nil, false, ErrAgentImageResultInvalid
	}
	if format != "png" && format != "jpeg" && format != "webp" {
		return nil, false, ErrAgentImageResultInvalid
	}
	if declared != "" {
		mediaType, _, parseErr := mime.ParseMediaType(declared)
		if parseErr != nil {
			return nil, false, ErrAgentImageResultInvalid
		}
		declared = mediaType
	}
	if declared != "" && declared != "image/"+format && !(format == "jpeg" && declared == "image/jpg") && declared != "application/octet-stream" {
		return nil, false, ErrAgentImageResultInvalid
	}
	return data, strings.TrimSpace(entry.B64JSON) != "", nil
}
