-- Add per-usage snapshot of the effective rate multiplier WITHOUT the model-level override.
--
-- 背景：模型级计费倍率（groups.model_pricing[].rate_multiplier）是「隐式配置」——
-- 计费按它走，但不对外暴露。usage_logs.rate_multiplier 记录的是最终生效倍率
-- （模型级倍率 × 高峰因子），直接展示会把隐式配置泄露给终端用户。
--
-- 本列存「不含模型级倍率」的展示用倍率，即：
--   用户专属倍率（缺省时取分组默认倍率）× 高峰因子
-- 用量详情的「费率」展示这一列，从而只隐藏模型级倍率，保留用户专属倍率与高峰因子。
--
-- 注意：不做回填、不设 NOT NULL。历史行为 NULL 时，前端回退展示 rate_multiplier；
-- 模型级倍率上线（0.2.13-r10）之前的记录两者本就相等，因此回退语义正确。

ALTER TABLE IF EXISTS usage_logs
  ADD COLUMN IF NOT EXISTS rate_multiplier_without_model DECIMAL(10,4);

COMMENT ON COLUMN usage_logs.rate_multiplier_without_model IS
  'Display-only rate multiplier snapshot excluding the model-level override (user/group multiplier x peak factor). NULL for rows written before this column existed.';
