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

// The 21-candy construction is the minimum guarantee; 29 is a common wrong
// answer produced when the participant is incorrectly assumed to draw without
// using touch to control how many candies come from each shape.
const monitorIntelligenceExpected = "21"
const monitorIntelligencePrompt = `请完成下面这道推理题，不要联网、不要执行代码。

在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表：

口味形状  苹果味  桃子味  西瓜味
圆形      7       9       8
五角星形  7       6       4

参赛者需要在活动前决定摸出的糖果总数，但摸取过程中可以靠手感控制从每种形状中各取多少颗，不能分辨口味。最少取出多少个糖果，才能保证手中同时拥有不同形状的苹果味和桃子味的糖？满足“圆形苹果味 + 五角星桃子味”或“圆形桃子味 + 五角星苹果味”任意一种即可。

只输出最终答案这一个阿拉伯数字。不要输出推理过程、解释、单位或任何其他数字。`

func validateIntelligenceResponse(response string) bool {
	numbers := monitorChallengeNumberRegex.FindAllString(strings.TrimSpace(response), -1)
	return len(numbers) == 1 && numbers[0] == monitorIntelligenceExpected
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
