package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestDirectImageDelegationUsesCurrentExplicitRequest(t *testing.T) {
	yes := []string{
		"生成一张小猫", "生成一幅小猫", "我想生成一只小猫图片 16:9 4k", "生成一张小猫图片，16：9 4k", "生成一张小猫图片，１６： ９ ４Ｋ", "帮我画一只猫", "画个猫", "生成一张蓝白风格样张",
		"把这张图片的背景换成蓝色", "重新生成一张图片", "不用配置卡，直接生成一张猫图",
		"Generate an image of a cat, 16:9, 4K", "Draw a cat", "Edit this photo with a blue background",
		"我想生成一张图，先问我几个问题", "制作一张海报", "设计一个logo",
	}
	no := []string{
		"生成一张猫图要多少钱", "生成一张图，先给配置", "generate a cat image, how much does it cost", "你好", "请分析这张图片", "你是什么模型", "我想做PPT", "生成5张统一风格图片",
		"生成两张猫图", "做3-5页幻灯片", "制作一套海报", "先展示生图方案", "先沟通生图风格",
		"先展示一张蓝白风格样张的方案，批准后再生成", "不要生成图片", "只写图片提示词",
		"图片呢", "上次生图是什么状态", "生成图片有没有扣费", "你会画图吗", "如何生成图片",
		"Generate 5 images", "Create two images of cats", "Do not generate an image", "Explain how to generate images",
	}
	for _, text := range yes {
		if directImageRequest(text) == nil {
			t.Errorf("explicit single image not admitted: %s", text)
		}
	}
	for _, text := range no {
		if directImageRequest(text) != nil {
			t.Errorf("unrelated/batch request admitted: %s", text)
		}
	}
	p := directImageRequest("我想生成一只小猫图片 16:9 4k")
	if p == nil || p.Size != "4K" || p.Aspect != "16:9" {
		t.Fatal("explicit specifications lost", p)
	}
}

func TestPostgresDirectSingleImageWithoutReview(t *testing.T) {
	s, f, c := newAdaptive(t)
	f.choose = func(in Invocation, n int) Outcome {
		if n == 1 {
			if !in.DirectImageAllowed {
				t.Fatal("model not told about bounded delegation")
			}
			return adaptiveCall("cat", "generate_image", ToolArgs{Prompt: "一只小猫", Size: stringPtr("1K"), Aspect: stringPtr("1:1")})
		}
		if in.DirectImageAllowed {
			t.Fatal("model told it can generate another image")
		}
		return adaptiveReply("图片已生成。")
	}
	r := adaptiveTurn(t, s, c, "direct-cat", "我想生成一只小猫图片 16:9 4k")
	if r.Status != "completed" || f.images != 1 || len(r.AssetIDs) != 1 || r.ToolErrors != 0 || r.ImageCost != "0.17000000" {
		t.Fatalf("status=%s error=%s images=%d assets=%d costs=%s", r.Status, r.ErrorCode, f.images, len(r.AssetIDs), r.ImageCost)
	}
	for _, in := range f.inputs {
		if in.Kind == "image" && (in.Args == nil || *in.Args.Size != "4K" || *in.Args.Aspect != "16:9" || in.Model != "image" || in.Binding.KeyID != 2) {
			t.Fatal("user specs or configured binding lost", in)
		}
	}
	ps, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
	if err != nil || len(ps) != 0 {
		t.Fatal("simple image created a review card", ps, err)
	}
	if _, exists := r.Public()["direct_image_request"]; exists {
		t.Fatal("private authorization exposed")
	}
	replay, err := s.Submit(context.Background(), 1, c.ID, "direct-cat", "", r.Input, nil, c.Version, "5")
	if err != nil || replay.ID != r.ID || f.images != 1 {
		t.Fatal("idempotent request dispatched again", err)
	}
}

func TestPostgresDirectSingleImageCannotExpandToolBatch(t *testing.T) {
	s, f, c := newAdaptive(t)
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return Outcome{Cost: "0.01000000", Body: jsonBytes(map[string]any{"status": "completed", "output": []any{
				map[string]any{"type": "function_call", "call_id": "one", "name": "generate_image", "arguments": string(jsonBytes(ToolArgs{Prompt: "cat"}))},
				map[string]any{"type": "function_call", "call_id": "extra", "name": "generate_image", "arguments": string(jsonBytes(ToolArgs{Prompt: "another cat"}))},
			}})}
		}
		return adaptiveReply("已生成一张图片。")
	}
	r := adaptiveTurn(t, s, c, "single-batch", "生成一张猫图")
	if r.Status != "completed" || f.images != 1 || len(r.AssetIDs) != 1 || r.ToolErrors != 1 || r.ImageCost != "0.17000000" {
		t.Fatal("tool loop expanded single-image permission", r.Status, r.ErrorCode, f.images, r.ToolErrors)
	}
	ps, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
	if err != nil || len(ps) != 0 {
		t.Fatal("exhaustion created redundant approval card", err)
	}
	var count int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM agent_operations WHERE run_id=$1 AND kind='image'`, r.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("extra operation reserved", count, err)
	}
}

func TestPostgresDirectSingleImageReservationIsAtomicAndBound(t *testing.T) {
	s, _, c := newAdaptive(t)
	ctx := context.Background()
	if _, err := s.Submit(ctx, 1, c.ID, "single-reserve", "", "生成一张猫图", nil, c.Version, ""); err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "single-worker")
	if err != nil {
		t.Fatal(err)
	}
	// A copied run cannot change the trusted binding or expand its permission.
	for _, mutate := range []func(*Run){func(v *Run) { v.Config.ImageModel = "foreign" }, func(v *Run) { v.Config.ImageKeyID = 123 }, func(v *Run) { v.UserID = 2 }, func(v *Run) { v.ConversationID = "foreign" }, func(v *Run) { v.DirectImage = &DirectImageRequest{Size: "4K"} }} {
		forged := *r
		mutate(&forged)
		_, err = s.Store.Prepare(ctx, &forged, Operation{ID: r.ID + "_forged", Kind: "image", Step: 9, RequestHash: "forged", Quote: "0.2"})
		if SafeCode(err) != "AGENT_IMAGE_AUTHORIZATION_REQUIRED" {
			t.Fatal("forged delegation accepted", err)
		}
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Store.Prepare(ctx, r, Operation{ID: fmt.Sprintf("%s_image_%d", r.ID, i), Kind: "image", Step: i, RequestHash: fmt.Sprint(i), Quote: "0.2"})
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	count := 0
	for e := range results {
		if e == nil {
			count++
		} else if SafeCode(e) != "SINGLE_IMAGE_LIMIT_REACHED" {
			t.Fatal(e)
		}
	}
	if count != 1 {
		t.Fatal("over-reserved", count)
	}
	var id, hash string
	var step int
	if err = s.Store.DB.QueryRow(`SELECT id,step,request_hash FROM agent_operations WHERE run_id=$1 AND kind='image'`, r.ID).Scan(&id, &step, &hash); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Prepare(ctx, r, Operation{ID: id, Kind: "image", Step: step, RequestHash: hash, Quote: "0.2"}); err != nil {
		t.Fatal("same-operation recovery lost delegation", err)
	}
	// Even a confirmed failed operation retains its slot. A separate authenticated
	// user retry is required, rather than a model silently trying a second image.
	if _, err = s.Store.DB.Exec(`UPDATE agent_operations SET status='failed',cost=0 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Prepare(ctx, r, Operation{ID: r.ID + "_extra", Kind: "image", Step: 8, RequestHash: "extra", Quote: "0.2"}); SafeCode(err) != "SINGLE_IMAGE_LIMIT_REACHED" {
		t.Fatal("failure reset delegation", err)
	}
}

func TestPostgresDirectSingleImageClarifiesAndResumesWithoutCard(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	f.choose = func(in Invocation, n int) Outcome {
		switch n {
		case 1:
			return adaptiveCall("question", "ask_user", AskUserArgs{Question: "需要什么主体和规格？", Fields: []QuestionField{{ID: "subject", Label: "主体", Required: true}, {ID: "specs", Label: "规格", Required: true}}})
		case 2:
			if !in.ClarificationAnswered || !in.DirectImageAllowed {
				t.Fatal("clarified request lost delegation")
			}
			return adaptiveCall("image", "generate_image", ToolArgs{Prompt: "窗边小猫"})
		default:
			return adaptiveReply("图片已生成。")
		}
	}
	waiting := adaptiveTurn(t, s, c, "clarify-single", "我想生成一张图，先问我几个问题")
	if waiting.Status != "waiting_user" || f.images != 0 {
		t.Fatal("did not wait for clarification")
	}
	_, err := s.Store.Answer(ctx, 1, waiting.ID, waiting.Question.ID, map[string]string{"subject": "窗边小猫", "specs": "16:9 4K"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Store.Claim(ctx, "resumed-worker")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	_ = s.Store.Release(ctx, r)
	if r.Status != "completed" || f.images != 1 || len(r.AssetIDs) != 1 {
		t.Fatal("clarification did not generate", r.Status, r.ErrorCode, f.images)
	}
	for _, in := range f.inputs {
		if in.Kind == "image" && (*in.Args.Size != "4K" || *in.Args.Aspect != "16:9") {
			t.Fatal("answer specifications lost")
		}
	}
	plans, err := s.Store.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(plans) != 0 {
		t.Fatal("clarification forced review", err)
	}
}

func TestPostgresDirectPermissionCannotBeMintedFromHistoryOrModel(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	r := adaptiveTurn(t, s, c, "plain-chat", "你好")
	if r.DirectImage != nil {
		t.Fatal("ordinary chat has direct permission")
	}
	// Persisted permission is absent even if a model/runtime object invents it.
	if _, err := s.Submit(ctx, 1, c.ID, "invented", "", "看看上次的图片", nil, c.Version, ""); err != nil {
		t.Fatal(err)
	}
	current, err := s.Store.Claim(ctx, "forgery-worker")
	if err != nil {
		t.Fatal(err)
	}
	current.DirectImage = &DirectImageRequest{}
	_, err = s.Store.Prepare(ctx, current, Operation{ID: current.ID + "_image_0", Kind: "image", RequestHash: "injected", Quote: "0.2"})
	if SafeCode(err) != "AGENT_IMAGE_AUTHORIZATION_REQUIRED" || f.images != 0 {
		t.Fatal("model minted permission", err)
	}
}

func TestPostgresDirectUnknownImageNeverRedispatches(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	f.unknown = true
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("cat", "generate_image", ToolArgs{Prompt: "cat"})
		}
		return adaptiveReply("请等待已有操作的结果。")
	}
	r := adaptiveTurn(t, s, c, "unknown-single", "生成一张猫图")
	if r.Status != "failed" || f.images != 1 || r.ReconcileDeadline != nil {
		t.Fatal("unknown image still waits", r.Status, r.ErrorCode)
	}
	if _, err := s.Store.Claim(ctx, "recovery-worker"); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed run still queued", err)
	}
	s.process(ctx, r)
	if f.images != 1 {
		t.Fatal("unknown operation dispatched twice")
	}
	if _, err := s.Retry(ctx, 1, r.ID, "retry-unknown", "", c.Version, ""); SafeCode(err) != "AGENT_RETRY_UNSETTLED" {
		t.Fatal("unsettled run retried", err)
	}
}

func TestPostgresSingleRequestCanStillChooseComplexReview(t *testing.T) {
	s, f, c := newAdaptive(t)
	var planID string
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("complex", "save_work_plan", workDraft(3))
		}
		if n == 2 {
			ps, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
			if err != nil || len(ps) != 1 {
				t.Fatal("missing complex plan", err)
			}
			planID = ps[0].ID
			purpose := "output"
			return adaptiveCall("not-approved", "generate_image", ToolArgs{Prompt: "page 1", TaskID: &planID, Purpose: &purpose})
		}
		return adaptiveReply("复杂内容已整理为三页方案，请审核。")
	}
	r := adaptiveTurn(t, s, c, "model-plans", "生成一张复杂活动海报，先审核配置卡")
	if r.Status != "completed" || f.images != 0 || r.DirectImage != nil || r.ToolErrors != 1 {
		t.Fatal("complex review bypassed", r.Status, f.images, r.ToolErrors)
	}
	if planID == "" {
		t.Fatal("model could not select planning tools")
	}
}

func TestPostgresSingleClarificationCannotExpandToMultipleImages(t *testing.T) {
	s, f, c := newAdaptive(t)
	ctx := context.Background()
	f.choose = func(_ Invocation, n int) Outcome {
		if n == 1 {
			return adaptiveCall("ask", "ask_user", AskUserArgs{Question: "需要怎样的内容？", Fields: []QuestionField{{ID: "details", Label: "内容", Required: true}}})
		}
		if n == 2 {
			return adaptiveCall("image", "generate_image", ToolArgs{Prompt: "两张不同构图的猫图"})
		}
		return adaptiveReply("已整理方案，请审核。")
	}
	waiting := adaptiveTurn(t, s, c, "change-count", "我想生成一张图，先问我几个问题")
	if waiting.Question == nil {
		t.Fatal("not waiting for answer")
	}
	answered, err := s.Store.Answer(ctx, 1, waiting.ID, waiting.Question.ID, map[string]string{"details": "改成两张猫图，构图不同"})
	if err != nil || answered.DirectImage != nil {
		t.Fatal("multi-output answer retained single delegation", err)
	}
	r, err := s.Store.Claim(ctx, "multi-answer")
	if err != nil {
		t.Fatal(err)
	}
	s.process(ctx, r)
	_ = s.Store.Release(ctx, r)
	if r.Status != "completed" || f.images != 0 {
		t.Fatal("multi-output request ran before approval", r.Status, f.images)
	}
	ps, err := s.Store.WorkPlans(ctx, 1, c.ID)
	if err != nil || len(ps) != 1 {
		t.Fatal("review missing", err)
	}
}

func TestDirectImageFullWidthSpecifications(t *testing.T) {
	for _, text := range []string{"生成一张小猫图片，16：9 4k", "生成一张小猫图片，１６： ９ ４Ｋ", "生成一张图，16 : 9 4K"} {
		p := directImageRequest(text)
		if p == nil || p.Aspect != "16:9" || p.Size != "4K" {
			t.Fatal("full-width specifications lost", text, p)
		}
	}
	if directImageRequest("生成２张图片") != nil {
		t.Fatal("full-width batch became single image")
	}
}

func TestPostgresSinglePlanningToolsCannotForceReview(t *testing.T) {
	for _, tool := range []string{"save_work_plan", "update_draft", "present_review", "too_many_items"} {
		t.Run(tool, func(t *testing.T) {
			s, f, c := newAdaptive(t)
			f.choose = func(_ Invocation, n int) Outcome {
				if n == 1 {
					draft := workDraft(1)
					draft.Brief = "生成小猫图片"
					draft.Items[0].Prompt = "一只小猫"
					draft.Size = "1K"
					draft.Aspect = "1:1"
					switch tool {
					case "save_work_plan":
						return adaptiveCall("plan", tool, draft)
					case "update_draft":
						return adaptiveCall("plan", tool, DraftUpdate{Patch: DraftPatch{Items: &draft.Items, Brief: &draft.Brief, Size: &draft.Size, Aspect: &draft.Aspect}})
					case "present_review":
						return adaptiveCall("review", tool, map[string]string{"plan_id": "unused-plan"})
					default:
						return adaptiveCall("too-many", "save_work_plan", workDraft(3))
					}
				}
				if n == 2 && (tool == "present_review" || tool == "too_many_items") {
					return adaptiveCall("direct", "generate_image", ToolArgs{Prompt: "一只小猫"})
				}
				return adaptiveReply("图片已生成。")
			}
			r := adaptiveTurn(t, s, c, "single-"+tool, "生成一张小猫图片，16：9 4k")
			if r.Status != "completed" || f.images != 1 || len(r.AssetIDs) != 1 {
				t.Fatal("simple planning still requires a card", r.Status, r.ErrorCode, f.images)
			}
			plans, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
			if err != nil || len(plans) != 0 {
				t.Fatal("simple request persisted a review card", len(plans), err)
			}
			for _, step := range r.Trace {
				if step.ReviewPlanID != "" {
					t.Fatal("review presentation requested")
				}
			}
			for _, in := range f.inputs {
				if in.Kind == "image" && (*in.Args.Size != "4K" || *in.Args.Aspect != "16:9") {
					t.Fatal("explicit specs lost")
				}
			}
		})
	}
}

func TestPostgresGreetingCannotStartVisualTasksFromHistory(t *testing.T) {
	for _, tool := range []string{"generate_image", "save_work_plan", "update_draft", "present_review", "ask_user"} {
		t.Run(tool, func(t *testing.T) {
			s, f, c := newAdaptive(t)
			f.choose = func(in Invocation, n int) Outcome {
				if n == 1 {
					switch tool {
					case "generate_image":
						return adaptiveCall("offtopic", tool, ToolArgs{Prompt: "a cat"})
					case "save_work_plan":
						return adaptiveCall("offtopic", tool, workDraft(1))
					case "update_draft":
						d := workDraft(1)
						return adaptiveCall("offtopic", tool, DraftUpdate{Patch: DraftPatch{Items: &d.Items}})
					case "present_review":
						return adaptiveCall("offtopic", tool, map[string]string{"plan_id": "unrelated-history"})
					default:
						return adaptiveCall("offtopic", tool, AskUserArgs{Question: "要生成什么图片？", Fields: []QuestionField{{ID: "subject", Label: "主体", Required: true}}})
					}
				}
				return adaptiveReply("你好！")
			}
			r := adaptiveTurn(t, s, c, "greeting-"+tool, "hi！")
			if r.Status != "completed" || f.images != 0 || r.Question != nil || r.DirectImage != nil {
				t.Fatal("greeting started a visual task", r.Status, f.images, r.Question)
			}
			plans, err := s.Store.WorkPlans(context.Background(), 1, c.ID)
			if err != nil || len(plans) != 0 {
				t.Fatal("greeting created review cards", plans, err)
			}
		})
	}
}
