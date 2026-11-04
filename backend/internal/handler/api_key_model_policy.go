package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// The editor discovers models only on request, without static catalog defaults.
// The selected group must be bindable by the signed-in user.
func (h *GatewayHandler) UserGroupModels(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	group, err := h.apiKeyService.BindableGroup(c.Request.Context(), subject.UserID, groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{UserID: subject.UserID, GroupID: &groupID, Group: group})
	if group.Platform == service.PlatformOpenAI && group.CodexModelsManifestConfig.Enabled {
		h.pinnedOpenAIModels(c, group)
		return
	}
	models, err := h.gatewayService.GetConfiguredGroupModels(c.Request.Context(), group)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	writeModelsList(c, group.Platform, models)
}

func apiKeyHasSelectedModels(c *gin.Context) bool {
	key, ok := middleware.GetAPIKeyFromContext(c)
	return ok && key != nil && len(key.AllowedModels) > 0
}

// Preserve model metadata and filter both OpenAI lists and native/Codex catalogs.
func filterAPIKeyModelCatalog(c *gin.Context, body []byte) ([]byte, error) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key == nil || len(key.AllowedModels) == 0 {
		return body, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
	field := "data"
	raw, exists := object[field]
	if !exists {
		field = "models"
		raw, exists = object[field]
	}
	if !exists {
		return nil, fmt.Errorf("missing model catalog")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	filtered := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		var model struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(entry, &model); err != nil {
			return nil, err
		}
		id := model.ID
		if id == "" {
			id = model.Slug
		}
		if id == "" {
			id = model.Name
		}
		if id == "" {
			return nil, fmt.Errorf("missing model ID")
		}
		if key.AllowsModel(id) {
			filtered = append(filtered, entry)
		}
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	object[field] = encoded
	return json.Marshal(object)
}

func writeScopedModelsError(c *gin.Context) {
	writeOpenAIModelsError(c, http.StatusBadGateway, "upstream_error", "Invalid model catalogue")
}
