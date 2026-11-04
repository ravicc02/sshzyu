package service

import (
	"encoding/json"
	"unicode/utf8"
)

const MaxOpsSystemLogPayloadBytes = 4096

func boundSystemLogText(value string, limit int) string {
	encoded, _ := json.Marshal(value)
	if len(value) <= limit && len(encoded) <= limit {
		return value
	}
	const suffix = "...[truncated]"
	end := min(limit-len(suffix), len(value))
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	out := value[:end] + suffix
	for {
		encoded, _ := json.Marshal(out)
		if len(encoded) <= limit {
			return out
		}
		end /= 2
		for end > 0 && !utf8.ValidString(value[:end]) {
			end--
		}
		out = value[:end] + suffix
	}
}

// BoundOpsSystemLog caps logical row payload bytes after redaction. PostgreSQL
// tuple/index overhead is separate. Copy the input so callers can safely reuse it.
func BoundOpsSystemLog(input *OpsInsertSystemLogInput) *OpsInsertSystemLogInput {
	if input == nil {
		return nil
	}
	out := *input
	out.Host = boundSystemLogText(out.Host, 255)
	out.Level = boundSystemLogText(out.Level, 16)
	out.Component = boundSystemLogText(out.Component, 64)
	out.Message = boundSystemLogText(out.Message, 1536)
	out.RequestID = boundSystemLogText(out.RequestID, 128)
	out.ClientRequestID = boundSystemLogText(out.ClientRequestID, 128)
	out.Platform = boundSystemLogText(out.Platform, 32)
	out.Model = boundSystemLogText(out.Model, 128)
	if len(out.ExtraJSON) > 1536 || !json.Valid([]byte(out.ExtraJSON)) {
		out.ExtraJSON = `{"truncated":true}`
	}
	if encoded, err := json.Marshal(out); err != nil || len(encoded) > MaxOpsSystemLogPayloadBytes {
		out.ExtraJSON = `{"truncated":true}`
		// JSON escaping can expand a control character to six bytes.
		out.Message = boundSystemLogText(out.Message, 256)
	}
	return &out
}
