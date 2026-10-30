package service

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

// Keep reasoning results separate from availability checks.
const monitorIntelligenceModel = "__reasoning_probe__"

// The 21-candy construction is the minimum guarantee; 29 is the common wrong
// answer produced when the shape constraint is ignored.
const monitorIntelligenceExpected = "21"
const monitorIntelligencePrompt = `Provide only the numerical answer. The use of internet searches, command execution, or code files is not permitted. Problem: A black bag contains candies of three different flavors, with each flavor coming in two distinct shapes (circular and star-shaped; the shapes can be distinguished by touch). The distribution of these candies by flavor and shape is provided in the table below. Before beginning, a participant must decide how many candies to draw from the bag. What is the minimum number of candies that must be drawn to guarantee that the participant holds both an apple-flavored candy and a peach-flavored candy, with the two candies being of different shapes? (The condition is satisfied if the participant holds, for example, a circular apple-flavored candy and a star-shaped peach-flavored candy, or a circular peach-flavored candy and a star-shaped apple-flavored candy.) Apple-flavored | Peach-flavored | Watermelon-flavored | Circular: 7, 9, 8 | Star-shaped: 7, 6, 4`

func validateIntelligenceResponse(response string) bool {
	return strings.TrimSpace(response) == monitorIntelligenceExpected
}

// monitorChallengePromptTemplate 1:1 复刻 BingZi-233/check-cx 的 few-shot 模板。
const monitorChallengePromptTemplate = `Calculate and respond with ONLY the number, nothing else.

Q: 3 + 5 = ?
A: 8

Q: 12 - 7 = ?
A: 5

Q: %d %s %d = ?
A:`

// monitorChallengeNumberRegex 提取响应中的所有整数（含负号）。
var monitorChallengeNumberRegex = regexp.MustCompile(`-?\d+`)

// monitorChallenge 一次 challenge 的 prompt + 期望答案。
type monitorChallenge struct {
	Prompt   string
	Expected string
}

// generateChallenge 生成一次随机算术 challenge：
//   - 随机两个 [monitorChallengeMin, monitorChallengeMax] 整数
//   - 50% 加 / 50% 减；减法用 max - min 保证非负
//   - 渲染 few-shot 模板
//
// 不强求加密随机：math/rand/v2 足够分散，避免 crypto/rand 的开销。
func generateChallenge() monitorChallenge {
	a := randIntInRange(monitorChallengeMin, monitorChallengeMax)
	b := randIntInRange(monitorChallengeMin, monitorChallengeMax)

	if rand.IntN(2) == 0 { //nolint:gosec // 仅用于生成测试问题，无安全影响
		// 加法
		return monitorChallenge{
			Prompt:   fmt.Sprintf(monitorChallengePromptTemplate, a, "+", b),
			Expected: strconv.Itoa(a + b),
		}
	}

	// 减法，保证非负
	hi, lo := a, b
	if lo > hi {
		hi, lo = lo, hi
	}
	return monitorChallenge{
		Prompt:   fmt.Sprintf(monitorChallengePromptTemplate, hi, "-", lo),
		Expected: strconv.Itoa(hi - lo),
	}
}

// randIntInRange 返回 [min, max] 闭区间的随机整数。
func randIntInRange(minVal, maxVal int) int {
	if maxVal <= minVal {
		return minVal
	}
	return minVal + rand.IntN(maxVal-minVal+1) //nolint:gosec
}

// validateChallenge 在响应文本中查找 expected 整数答案，返回是否通过校验。
func validateChallenge(responseText, expected string) bool {
	if responseText == "" || expected == "" {
		return false
	}
	matches := monitorChallengeNumberRegex.FindAllString(responseText, -1)
	for _, m := range matches {
		if m == expected {
			return true
		}
	}
	return false
}
