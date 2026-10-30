package securityaudit

import (
	"strings"
)

const (
	defaultPromptKeywordLimit  = 500
	maxPromptKeywordRuneLength = 128
)

// DefaultPromptAuditKeywords are high-precision routing signals, not verdicts.
// A matched request still goes through the configured model classifier.
var DefaultPromptAuditKeywords = []string{
	"忽略之前",
	"忽略以上",
	"忽略先前",
	"无视之前",
	"无视以上",
	"忘记之前",
	"覆盖系统指令",
	"绕过安全",
	"绕过限制",
	"解除限制",
	"无限制模式",
	"开发者模式",
	"越狱模式",
	"系统提示词",
	"系统提示",
	"开发者消息",
	"隐藏指令",
	"内部指令",
	"泄露提示词",
	"输出提示词",
	"打印提示词",
	"重复以上内容",
	"不要遵守",
	"不受规则约束",
	"忽略你的设定",
	"无视你的设定",
	"覆盖你的设定",
	"你现在没有限制",
	"你现在不受限制",
	"关闭安全策略",
	"关闭审核规则",
	"跳过安全检查",
	"跳过审核",
	"进入管理员模式",
	"进入调试模式",
	"假装你是",
	"角色扮演为",
	"你现在是",
	"系统开发者",
	"最高权限",
	"管理员指令",
	"泄露内部规则",
	"显示系统提示",
	"输出内部指令",
	"忽略安全规则",
	"无视安全规则",
	"绕过内容审核",
	"绕过过滤器",
	"解除所有限制",
	"不受任何约束",
	"ignore previous instructions",
	"ignore all previous instructions",
	"ignore prior instructions",
	"disregard previous instructions",
	"forget previous instructions",
	"disregard all previous instructions",
	"system prompt",
	"developer message",
	"hidden instructions",
	"internal instructions",
	"jailbreak",
	"developer mode",
	"do anything now",
	"dan mode",
	"bypass safety",
	"bypass restrictions",
	"no restrictions",
	"unrestricted mode",
	"override instructions",
	"reveal system prompt",
	"print system prompt",
	"repeat the instructions above",
	"you are now",
	"act as",
	"roleplay as",
	"pretend to be",
	"administrator mode",
	"admin mode",
	"debug mode",
	"god mode",
	"no safety rules",
	"ignore safety rules",
	"disable safety",
	"disable moderation",
	"skip safety check",
	"skip moderation",
	"bypass moderation",
	"bypass filter",
	"remove all restrictions",
	"without restrictions",
	"no limitations",
	"break the rules",
	"disregard your system prompt",
	"reveal hidden instructions",
	"show system message",
	"output system prompt",
	"leak system prompt",
	"highest privileges",
	"root access",
	"grant yourself permission",
	// Violent
	"杀人方法",
	"谋杀他人",
	"制造炸弹",
	"恐怖袭击",
	"绑架计划",
	"how to kill someone",
	"make a bomb",
	"plan a terror attack",
	"kidnap someone",
	"commit a massacre",
	// Non-violent illegal acts
	"黑客攻击",
	"入侵系统",
	"制作木马",
	"勒索软件",
	"钓鱼网站",
	"盗取账号密码",
	"洗钱方法",
	"毒品交易",
	"伪造证件",
	"走私渠道",
	"hack into a system",
	"create malware",
	"ransomware code",
	"phishing page",
	"steal account credentials",
	"money laundering method",
	"drug trafficking",
	"forge a passport",
	"bypass law enforcement",
	"buy illegal drugs",
	// Sexual content
	"色情视频",
	"成人视频",
	"裸照生成",
	"性服务",
	"未成年色情",
	"儿童色情",
	"性剥削",
	"explicit sexual content",
	"pornographic video",
	"nude image generation",
	"sexual services",
	"child sexual material",
	"csam",
	"non consensual sexual content",
	"sexual exploitation",
	// PII
	"身份证号码",
	"家庭住址",
	"银行卡号",
	"手机号码",
	"人脸数据",
	"客户名单",
	"导出个人信息",
	"泄露个人信息",
	"人肉搜索",
	"home address",
	"credit card number",
	"social security number",
	"passport number",
	"personal phone number",
	"customer database export",
	"leak personal information",
	"find someone's address",
	"dox someone",
	"scrape personal data",
	"extract user records",
	// Suicide and self-harm
	"自杀方法",
	"如何自残",
	"割腕",
	"不想活了",
	"结束自己的生命",
	"伤害自己的身体",
	"suicide method",
	"how to self harm",
	"kill myself",
	"cut myself",
	"end my life",
	"hurt myself",
	"want to die",
	"suicide instructions",
	// Unethical acts
	"诈骗话术",
	"实施诈骗",
	"贿赂",
	"考试作弊",
	"伪造证据",
	"内幕交易",
	"逃税方法",
	"操纵市场",
	"刷单造假",
	"scam script",
	"commit fraud",
	"bribe an official",
	"cheat on an exam",
	"fabricate evidence",
	"insider trading",
	"tax evasion",
	"market manipulation",
	"fake reviews",
	"academic dishonesty",
	// Politically sensitive topics
	"颠覆国家政权",
	"分裂国家",
	"煽动叛乱",
	"暴力革命",
	"政治暗杀",
	"泄露国家秘密",
	"推翻政府",
	"发动政变",
	"煽动民族仇恨",
	"组织非法游行",
	"overthrow the government",
	"incite insurrection",
	"political assassination",
	"leak state secrets",
	"stage a coup",
	"incite ethnic hatred",
	"violent revolution",
	"terrorist propaganda",
	"extremist propaganda",
	"election fraud",
	// Copyright violation
	"盗版资源",
	"破解软件",
	"盗录视频",
	"未经授权下载",
	"无水印下载",
	"付费内容破解",
	"会员内容提取",
	"侵权资源",
	"piracy download",
	"pirated content",
	"cracked software",
	"software keygen",
	"torrent download",
	"copyrighted content",
	"paywall bypass",
	"remove watermark",
	"download paid course",
	"unauthorized copy",
	"leak paid content",
	"copyright infringement",
}

func normalizePromptKeywords(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		runes := []rune(value)
		if len(runes) > maxPromptKeywordRuneLength {
			value = string(runes[:maxPromptKeywordRuneLength])
		}
		key := normalizePromptMatchText(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) >= defaultPromptKeywordLimit {
			break
		}
	}
	return result
}

func normalizePromptMatchText(value string) string {
	value = strings.NewReplacer(
		"\u200b", "",
		"\u200c", "",
		"\u200d", "",
		"\ufeff", "",
	).Replace(value)
	value = strings.ToLower(value)
	return strings.Join(strings.Fields(value), " ")
}

func matchPromptKeyword(text string, keywords []string) (string, bool) {
	normalized := normalizePromptMatchText(text)
	if normalized == "" {
		return "", false
	}
	for _, keyword := range keywords {
		needle := normalizePromptMatchText(keyword)
		if needle != "" && strings.Contains(normalized, needle) {
			return keyword, true
		}
	}
	return "", false
}

func shouldAuditPrompt(cfg ActiveConfig, text string) bool {
	if !cfg.KeywordFilterEnabled {
		return true
	}
	keywords := cfg.Keywords
	if len(keywords) == 0 {
		keywords = DefaultPromptAuditKeywords
	}
	_, matched := matchPromptKeyword(text, keywords)
	return matched
}
