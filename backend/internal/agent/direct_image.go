package agent

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
)

// DirectImageRequest is a bounded delegation from an authenticated user message,
// not a model-supplied approval. It is private, persisted with the run and never
// inherited by another turn. The operation ledger consumes its one image slot.
type DirectImageRequest struct {
	Size   string `json:"size,omitempty"`
	Aspect string `json:"aspect,omitempty"`
}

var imageAction = regexp.MustCompile(`(?i)(画|绘制|绘画|生图|出图|生成|制作|设计|做|修改|改成|改为|换成|替换|重绘|draw|paint|render|generate|create|make|design|edit)`)
var imageSubject = regexp.MustCompile(`(?i)(图|照片|样张|海报|封面|插画|壁纸|头像|logo|image|picture|photo|illustration|poster|wallpaper|portrait|cover)`)
var imageDrawing = regexp.MustCompile(`(?i)(画一|画个|画只|画张|帮我画|给我画|请画|生成一[张幅只]|draw\b|paint\b)`)
var imageWorkflow = regexp.MustCompile(`(?i)([2-9][0-9]*|1[0-9]+|[两二三四五六七八九十百几多])[ \t]*(张|幅|页|款|套|images?\b|pictures?\b|photos?\b|slides?\b)|批量|多张|组图|一组|一套|系列|分镜|幻灯片|演示文稿|ppt|slide\s*deck|batch|multiple|several|two images|three images|five images`)
var imageDiscussion = regexp.MustCompile(`(?i)(复杂|多步骤|逐项审核|多少钱|价格.{0,5}(多少|如何)|how much|方案|配置卡|取消|不需要了|算了|cancel|never mind|分析|识别|解读|看看|看一下|解释|教程|原理|扣费|费用|状态|有没有|是否支持|你会|你能|会不会|can you (draw|generate) images|先.{0,12}(讨论|沟通|规划|大纲|审核|确认|配置|报价|核价|估价)|不要.{0,12}(生成|生图|画|执行)|不.{0,6}(生成|生图)|仅.{0,6}(分析|识别|解释)|只.{0,6}(分析|识别|解释|写|讨论)|如何|怎么.{0,6}(生成|画|制作)|提示词|教程|what model|how (to|do)|do not|don't|not yet|prompt for|plan first|review first)`)
var imageSize = regexp.MustCompile(`(?i)\b[124]k\b`)
var imageAspect = regexp.MustCompile(`(?:^|[^0-9])(1\s*:\s*1|16\s*:\s*9|9\s*:\s*16|4\s*:\s*3|3\s*:\s*4)(?:$|[^0-9])`)

// This only recognizes explicit image actions for paid delegation. The model
// still decides whether information is missing and how to handle the task.
// Ambiguous, informational and multi-output requests keep the review boundary.
func directImageRequest(text string) *DirectImageRequest {
	text = strings.ToLower(strings.TrimSpace(normalizeImageSpecs(text)))
	for _, phrase := range []string{"不用配置卡", "不需要配置卡", "不要配置卡", "无需配置卡", "不用方案", "不需要方案", "不要方案", "不要再问", "不用再确认", "无需确认", "no review needed"} {
		text = strings.ReplaceAll(text, phrase, "")
	}
	if ImageStatusInquiry(text) || imageWorkflow.MatchString(text) || imageDiscussion.MatchString(text) || !imageAction.MatchString(text) || !(imageSubject.MatchString(text) || imageDrawing.MatchString(text)) {
		return nil
	}
	p := &DirectImageRequest{}
	p.updateSpecs(text)
	return p
}

func normalizeImageSpecs(text string) string {
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		switch r {
		case '：', '∶':
			return ':'
		case 'Ｋ':
			return 'K'
		case 'ｋ':
			return 'k'
		}
		return r
	}, text)
}

func (p *DirectImageRequest) updateSpecs(text string) {
	text = normalizeImageSpecs(text)
	if sizes := imageSize.FindAllString(text, -1); len(sizes) > 0 {
		p.Size = strings.ToUpper(sizes[len(sizes)-1])
	}
	if aspects := imageAspect.FindAllStringSubmatch(text, -1); len(aspects) > 0 {
		p.Aspect = strings.Join(strings.Fields(aspects[len(aspects)-1][1]), "")
	}
}

func (r *Run) directImageArgs(args *ToolArgs) bool {
	if r.DirectImage == nil || (args.TaskID != nil && *args.TaskID != "") {
		return false
	}
	// Preserve explicit user specifications even when the model omits or
	// paraphrases them. Defaults remain available for genuinely unspecified fields.
	if r.DirectImage.Size != "" {
		args.Size = stringPtr(r.DirectImage.Size)
	}
	if r.DirectImage.Aspect != "" {
		args.Aspect = stringPtr(r.DirectImage.Aspect)
	}
	return true
}

// Prepare already holds the run row lock. A distinct image operation cannot
// consume this delegation again, including failed or unknown paid attempts.
// Retrying preparation of the same operation is idempotent. Missing permission
// on old checkpoints stays fail-closed; it is never reconstructed from history.
func reserveDirectImage(ctx context.Context, tx *sql.Tx, r *Run, op Operation) error {
	if r.DirectImage == nil {
		return Invalid("AGENT_GRANT_REQUIRED")
	}
	var authorized bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_conversations c ON c.id=r.conversation_id WHERE r.id=$1 AND r.user_id=$2 AND c.user_id=$2 AND c.id=$3 AND c.deleted_at IS NULL AND r.cancel_requested=false AND r.snapshot->'direct_image_request'=$4::jsonb AND r.snapshot->'config'->>'image_api_key_id'=$5 AND r.snapshot->'config'->>'image_model'=$6 AND (r.snapshot->>'deadline')::timestamptz>now())`, r.ID, r.UserID, r.ConversationID, jsonBytes(r.DirectImage), strconv.FormatInt(r.Config.ImageKeyID, 10), r.Config.ImageModel).Scan(&authorized)
	if err != nil {
		return err
	}
	if !authorized {
		return Invalid("AGENT_IMAGE_AUTHORIZATION_REQUIRED")
	}
	var used bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_operations WHERE run_id=$1 AND kind='image' AND id<>$2)`, r.ID, op.ID).Scan(&used); err != nil {
		return err
	}
	if used {
		return &ToolFeedback{Code: "SINGLE_IMAGE_LIMIT_REACHED", Category: "permission", Message: "本次单张生图请求已占用一张额度。请返回已有结果；未知结果应等待恢复，不得再次生图。用户明确要求再生成后才能开启新的请求。"}
	}
	return nil
}

func greetingOnly(text string) bool {
	text = strings.TrimFunc(strings.ToLower(strings.TrimSpace(text)), func(r rune) bool {
		return r == '!' || r == '！' || r == '?' || r == '？' || r == '.' || r == '。' || r == ',' || r == '，' || r == '~' || r == '～' || r == ' ' || r == '\n' || r == '\t'
	})
	switch text {
	case "hi", "hello", "hey", "你好", "您好", "嗨", "哈喽", "在吗":
		return true
	default:
		return false
	}
}
