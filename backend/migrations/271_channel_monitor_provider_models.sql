ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS provider_models JSONB NOT NULL DEFAULT '{}'::jsonb;

WITH candidate_models AS (
    SELECT
        cm.id,
        models.model
    FROM channel_monitors cm
    JOIN groups g
      ON g.id = cm.group_id
     AND g.platform = 'composite'
     AND g.deleted_at IS NULL
    CROSS JOIN LATERAL (
        SELECT cm.primary_model AS model
        UNION ALL
        SELECT jsonb_array_elements_text(cm.extra_models)
    ) AS models
    WHERE cm.provider_models = '{}'::jsonb
      AND btrim(models.model) <> ''
),
mapped_models AS (
    SELECT
        id,
        model,
        CASE
            WHEN lower(model) LIKE 'deepseek-%' THEN 'deepseek'
            WHEN lower(model) LIKE 'glm-%' THEN 'zhipu'
            WHEN lower(model) LIKE 'kimi-%' THEN 'kimi'
            WHEN lower(model) LIKE 'minimax-%' OR lower(model) LIKE 'abab%' THEN 'minimax'
            ELSE ''
        END AS provider,
        ROW_NUMBER() OVER (
            PARTITION BY id,
            CASE
                WHEN lower(model) LIKE 'deepseek-%' THEN 'deepseek'
                WHEN lower(model) LIKE 'glm-%' THEN 'zhipu'
                WHEN lower(model) LIKE 'kimi-%' THEN 'kimi'
                WHEN lower(model) LIKE 'minimax-%' OR lower(model) LIKE 'abab%' THEN 'minimax'
                ELSE ''
            END
            ORDER BY model
        ) AS provider_rank
    FROM candidate_models
),
provider_maps AS (
    SELECT id, jsonb_object_agg(provider, model) AS provider_models
    FROM mapped_models
    WHERE provider <> ''
      AND provider_rank = 1
    GROUP BY id
)
UPDATE channel_monitors cm
SET provider_models = provider_maps.provider_models,
    updated_at = NOW()
FROM provider_maps
WHERE cm.id = provider_maps.id
  AND cm.provider_models = '{}'::jsonb;
