//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntelligenceAnswerIsExactAndSeparate(t *testing.T) {
	for _, answer := range []string{"21", " 21\n"} {
		require.True(t, validateIntelligenceResponse(answer))
	}
	for _, answer := range []string{"", "20", "29", "121", "-21", "Answer: 21", "21 or 22"} {
		require.False(t, validateIntelligenceResponse(answer))
	}
}

func TestIntelligenceProbeEligibility(t *testing.T) {
	require.True(t, shouldRunIntelligenceProbe(&ChannelMonitor{Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeProbe, IntelligenceEnabled: true}))
	require.True(t, shouldRunIntelligenceProbe(&ChannelMonitor{Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeQuotaProbe, IntelligenceEnabled: true}))
	require.False(t, shouldRunIntelligenceProbe(&ChannelMonitor{Provider: MonitorProviderAnthropic}))
	require.False(t, shouldRunIntelligenceProbe(&ChannelMonitor{Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeQuota}))
	require.False(t, shouldRunIntelligenceProbe(&ChannelMonitor{Provider: MonitorProviderOpenAI, BodyOverrideMode: MonitorBodyOverrideModeReplace}))
	require.False(t, shouldRunIntelligenceProbe(&ChannelMonitor{Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeProbe, IntelligenceEnabled: false}))
}

func TestIntelligenceRequestBodyPreservesQuestionAndEnoughOutputTokens(t *testing.T) {
	for _, mode := range []string{MonitorAPIModeChatCompletions, MonitorAPIModeResponses} {
		t.Run(mode, func(t *testing.T) {
			adapter, _, ok := providerAdapterFor(MonitorProviderOpenAI, mode)
			require.True(t, ok)
			body, err := buildRequestBody(adapter, MonitorProviderOpenAI, mode, "test-model", monitorIntelligencePrompt, &CheckOptions{
				ReasoningProbe: true, APIMode: mode, BodyOverrideMode: MonitorBodyOverrideModeMerge,
				BodyOverride: map[string]any{"model": "not-test-model", "max_output_tokens": 1},
			})
			require.NoError(t, err)
			var data map[string]any
			require.NoError(t, json.Unmarshal(body, &data))
			require.Equal(t, "test-model", data["model"])
			if mode == MonitorAPIModeResponses {
				require.Equal(t, monitorIntelligencePrompt, data["input"])
				require.Equal(t, float64(2048), data["max_output_tokens"])
			} else {
				messages := data["messages"].([]any)
				require.Equal(t, monitorIntelligencePrompt, messages[0].(map[string]any)["content"])
				require.Equal(t, float64(2048), data["max_tokens"])
			}
		})
	}
}
