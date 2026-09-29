package domain

// KeyGroupProvider buckets group platforms into provider labels used by the
// admin user-list filter (api_key_provider). The classification MUST stay in
// sync with the user-facing frontend mapping in
// frontend/src/utils/keyGroupProviders.ts (PROVIDER_BY_PLATFORM).
const (
	KeyGroupProviderAnthropic = "anthropic"
	KeyGroupProviderOpenAI    = "openai"
	KeyGroupProviderDomestic  = "domestic"
	KeyGroupProviderOther     = "other"
)

// keyGroupProviderInclude is the explicit platform whitelist per provider,
// mirroring PROVIDER_BY_PLATFORM on the frontend.
var keyGroupProviderInclude = map[string][]string{
	KeyGroupProviderAnthropic: {PlatformAnthropic},
	KeyGroupProviderOpenAI:    {PlatformOpenAI},
	KeyGroupProviderDomestic:  {PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax},
}

// KeyGroupProviderExcludePlatforms returns every platform that is explicitly
// classified into a non-"other" provider. "other" is defined by exclusion:
// any platform NOT in this list counts as "other". This mirrors the frontend
// fallback (`PROVIDER_BY_PLATFORM[platform] ?? 'other'`), so newly added
// platforms are classified as "other" on both sides without code changes.
func KeyGroupProviderExcludePlatforms() []string {
	known := make(map[string]struct{})
	for _, platforms := range keyGroupProviderInclude {
		for _, p := range platforms {
			known[p] = struct{}{}
		}
	}
	// Deterministic order for stable SQL and tests.
	ordered := []string{
		PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity,
		PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax,
		PlatformOpenCodeGo, PlatformComposite,
	}
	out := make([]string, 0, len(ordered))
	for _, p := range ordered {
		if _, ok := known[p]; ok {
			out = append(out, p)
		}
	}
	return out
}

// KeyGroupProviderPlatforms resolves a provider label into a platform filter:
//   - include non-empty: match platforms IN include;
//   - excludeAllKnown true: match platforms NOT IN KeyGroupProviderExcludePlatforms()
//     (the "other" bucket, defined by exclusion);
//   - ok false: unknown provider label, caller should ignore the filter.
func KeyGroupProviderPlatforms(provider string) (include []string, excludeAllKnown bool, ok bool) {
	if platforms, found := keyGroupProviderInclude[provider]; found {
		return platforms, false, true
	}
	if provider == KeyGroupProviderOther {
		return nil, true, true
	}
	return nil, false, false
}
