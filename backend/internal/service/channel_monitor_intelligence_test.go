//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntelligenceAnswerParsing(t *testing.T) {
	for _, answer := range []string{
		"21",
		" 21\n",
		"最终答案是 21",
		"最终答案：21",
		"最少取出 21 个",
		"\\boxed{21}",
	} {
		require.True(t, validateIntelligenceResponse(answer))
	}
	for _, answer := range []string{
		"",
		"20",
		"29",
		"121",
		"-21",
		"21 or 22",
		"最终答案：29",
		"最少取出 21 个，方法是利用手感取 9 个圆形和 12 个五角星形。",
		"这里有 7、9、8、21 等多个数字",
	} {
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
				require.Equal(t, "medium", data["reasoning"].(map[string]any)["effort"])
			} else {
				messages := data["messages"].([]any)
				require.Equal(t, monitorIntelligencePrompt, messages[0].(map[string]any)["content"])
				require.Equal(t, float64(2048), data["max_tokens"])
				require.Equal(t, "medium", data["reasoning_effort"])
			}
		})
	}
}
