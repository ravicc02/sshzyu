package agent

import (
	"encoding/json"
	"time"
)

// TraceStep is a public projection, never a copy of protocol history or a
// provider response. Only user-visible summaries and verified tool fields enter it.
type TraceStep struct {
	Commentary string          `json:"commentary,omitempty"`
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	EndedAt    *time.Time      `json:"ended_at,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Arguments  *TraceArguments `json:"arguments,omitempty"`
	AssetIDs   []string        `json:"asset_ids,omitempty"`
	Cost       string          `json:"cost_usd,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
}
type TraceArguments struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	ImageSize      string `json:"image_size"`
	AspectRatio    string `json:"aspect_ratio"`
	ReferenceCount int    `json:"reference_count"`
}

func traceArguments(args ToolArgs, model string) *TraceArguments {
	size, aspect := "1K", "1:1"
	if args.Model != nil {
		model = *args.Model
	}
	if args.Size != nil {
		size = *args.Size
	}
	if args.Aspect != nil {
		aspect = *args.Aspect
	}
	return &TraceArguments{Prompt: args.Prompt, Model: model, ImageSize: size, AspectRatio: aspect, ReferenceCount: len(args.References)}
}
func (r *Run) startStep(id, kind, name, status string, args *TraceArguments) *TraceStep {
	for i := range r.Trace {
		step := &r.Trace[i]
		if step.ID == id {
			// A recovered step retains its original time; a queued tool starts its
			// actual execution clock only when execution begins.
			if step.Status == "queued" && status == "running" {
				step.StartedAt = time.Now().UTC()
			}
			if !traceTerminal(step.Status) && step.Status != "reconciling" {
				step.Status = status
			}
			if args != nil {
				step.Arguments = args
			}
			return step
		}
	}
	r.Trace = append(r.Trace, TraceStep{ID: id, Kind: kind, Name: name, Status: status, StartedAt: time.Now().UTC(), Arguments: args})
	return &r.Trace[len(r.Trace)-1]
}
func traceTerminal(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "expired", "skipped":
		return true
	}
	return false
}
func (r *Run) completeStep(id, cost, summary string, assets []string) {
	for i := range r.Trace {
		step := &r.Trace[i]
		if step.ID != id {
			continue
		}
		now := time.Now().UTC()
		step.Status, step.Cost, step.Summary, step.AssetIDs = "completed", cost, summary, assets
		step.EndedAt, step.ErrorCode = &now, ""
		return
	}
}
func (r *Run) pendingSteps(status, code string) {
	now := time.Now().UTC()
	for i := range r.Trace {
		step := &r.Trace[i]
		if traceTerminal(step.Status) {
			continue
		}
		step.Status, step.ErrorCode = status, code
		if traceTerminal(status) {
			step.EndedAt = &now
		}
	}
}
func traceEventPayload(r *Run, data any) ([]byte, error) {
	payload, err := json.Marshal(data)
	if err != nil || len(r.Trace) == 0 {
		return payload, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	fields["trace"], err = json.Marshal(r.Trace)
	if err != nil {
		return nil, err
	}
	fields["run_id"], err = json.Marshal(r.ID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}
