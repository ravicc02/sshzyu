package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyCatalogFilterPreservesSelectedMetadata(t *testing.T) {
	for _, catalog := range []string{
		`{"object":"list","data":[{"id":"allowed","description":"kept"},{"id":"blocked"}]}`,
		`{"models":[{"slug":"allowed","description":"kept"},{"slug":"blocked"}]}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{AllowedModels: []string{"allowed"}})
		body, err := filterAPIKeyModelCatalog(c, []byte(catalog))
		require.NoError(t, err)
		var parsed map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &parsed))
		raw := parsed["data"]
		if raw == nil {
			raw = parsed["models"]
		}
		var models []map[string]any
		require.NoError(t, json.Unmarshal(raw, &models))
		require.Len(t, models, 1)
		require.Equal(t, "kept", models[0]["description"])
	}
}

func TestAPIKeyCatalogFilterFailsClosedOnMalformedCatalog(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{AllowedModels: []string{"allowed"}})
	for _, body := range []string{`{}`, `{"data":[{"description":"missing identity"}]}`} {
		_, err := filterAPIKeyModelCatalog(c, []byte(body))
		require.Error(t, err)
	}
}
