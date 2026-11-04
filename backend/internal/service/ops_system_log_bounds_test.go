package service

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOpsSystemLogPayloadBound(t *testing.T) {
	input := &OpsInsertSystemLogInput{Message: strings.Repeat("大字段", 100000), ExtraJSON: strings.Repeat("x", 100000), Host: strings.Repeat("h", 1000), Component: strings.Repeat("c", 1000), RequestID: strings.Repeat("r", 1000), Model: strings.Repeat("m", 1000)}
	out := BoundOpsSystemLog(input)
	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), MaxOpsSystemLogPayloadBytes)
	require.True(t, utf8.ValidString(out.Message))
	require.True(t, json.Valid([]byte(out.ExtraJSON)))
	require.Greater(t, len(input.Message), 100000, "caller input must not be mutated")
}

func TestOpsSystemLogPayloadEscapingCannotExceedBound(t *testing.T) {
	value := strings.Repeat("\x00", 1000)
	out := BoundOpsSystemLog(&OpsInsertSystemLogInput{Message: value, Host: value, Level: value, Component: value, RequestID: value, ClientRequestID: value, Platform: value, Model: value, ExtraJSON: `{"x":"ok"}`})
	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), MaxOpsSystemLogPayloadBytes)
}
