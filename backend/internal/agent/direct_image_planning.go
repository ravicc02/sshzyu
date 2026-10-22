package agent

import "strings"

// The authenticated one-image request is the execution boundary. A model's
// choice of a planning tool cannot add a second approval step or more outputs.
// Explicit review requests and complex/batch requests have no direct delegation
// and continue to use the ordinary editable-card workflow.
func directImagePlanningCall(r *Run, call Call) (Call, error) {
	if r.RuntimeVersion >= 2 && greetingOnly(r.Input) && oneOf(call.Name, "generate_image", "save_work_plan", "update_draft", "present_review", "ask_user") {
		return call, &ToolFeedback{Code: "CURRENT_REQUEST_IS_GREETING", Category: "intent", Message: "用户本轮只是在打招呼。请直接自然回复，不要沿用历史图片任务创建配置卡、生成图片或追问生图参数。"}
	}
	if r.RuntimeVersion < 2 || r.DirectImage == nil || r.ForceAskUser {
		return call, nil
	}
	feedback := func(message string) (Call, error) {
		return call, &ToolFeedback{Code: "DIRECT_IMAGE_REQUEST", Category: "permission", Message: message}
	}
	var draft WorkDraft
	switch call.Name {
	case "present_review":
		return feedback("本轮用户明确要求直接生成一张图片，不需要配置卡。请使用 generate_image，task_id/purpose 传 null；只在仍缺少必要信息时 ask_user。")
	case "save_work_plan":
		if StrictJSON([]byte(call.Arguments), &draft) != nil {
			return call, invalidArguments("draft")
		}
	case "update_draft":
		var update DraftUpdate
		if StrictJSON([]byte(call.Arguments), &update) != nil {
			return call, invalidArguments("patch")
		}
		if update.PlanID != nil && *update.PlanID != "" {
			return feedback("当前是新的单张生成请求，请读取需要的已有信息后直接 generate_image，不要修改旧配置卡或要求再次批准。")
		}
		draft = update.Patch.apply(WorkDraft{})
	default:
		return call, nil
	}
	if len(draft.Items) != 1 {
		return feedback("本轮仅授权直接生成一张图片。请使用 generate_image；不要扩大成多项配置卡。缺少必要信息时先 ask_user。")
	}
	if r.DirectImage.Size != "" {
		draft.Size = r.DirectImage.Size
	}
	if r.DirectImage.Aspect != "" {
		draft.Aspect = r.DirectImage.Aspect
	}
	normalizeDraft(&draft)
	if err := draft.ValidateDraft(); err != nil {
		return call, err
	}
	item := draft.Items[0]
	if strings.TrimSpace(item.Prompt) == "" && strings.TrimSpace(item.Content) == "" {
		return feedback("尚未提供图片的实际内容，请根据用户已明确的需求填写 generate_image.prompt；只有关键内容缺失时才 ask_user，不需要保存空配置卡。")
	}
	references := workReferences(draft, item)
	if len(references) > 1 {
		return feedback("当前单张工具只接受一张参考图。请先澄清素材用途或请求用户明确审阅复杂编排，不要在本轮擅自新增配置卡。")
	}
	if strings.TrimSpace(draft.Brief) == "" {
		draft.Brief = r.Input
	}
	args := ToolArgs{Prompt: WorkPrompt(draft, item), References: references}
	if draft.Size != "" {
		args.Size = stringPtr(draft.Size)
	}
	if draft.Aspect != "" {
		args.Aspect = stringPtr(draft.Aspect)
	}
	if _, err := ParseTool(jsonBytes(args)); err != nil {
		return call, invalidArguments("generate_image")
	}
	call.Name = "generate_image"
	call.Arguments = string(jsonBytes(args))
	return call, nil
}
