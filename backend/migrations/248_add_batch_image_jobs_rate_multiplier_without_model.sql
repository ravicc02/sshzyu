-- Add per-job snapshot of the group rate multiplier WITHOUT the model-level override.
--
-- 背景：模型级计费倍率（groups.model_pricing[].rate_multiplier）是「隐式配置」——
-- 计费按它走，但不对外暴露。batch_image_jobs.group_rate_multiplier 快照的是「应用
-- 模型级倍率之后」的生效倍率，结算写用量记录时直接展示会把隐式配置泄露给终端用户。
--
-- 本列存「不含模型级覆盖」的分组/用户图片倍率（分组开启 image_rate_independent 时
-- 即为该独立倍率，其本身不受模型级影响）。结算时展示倍率 = 本列 × batch_discount_multiplier。
--
-- 可空：早于本列创建的任务为 NULL，结算时回退到 group_rate_multiplier（即引入本列前的行为）。

ALTER TABLE IF EXISTS batch_image_jobs
  ADD COLUMN IF NOT EXISTS group_rate_multiplier_without_model DECIMAL(10,4);

COMMENT ON COLUMN batch_image_jobs.group_rate_multiplier_without_model IS
  'Submit-time snapshot of the group/user image rate multiplier excluding the model-level override; NULL for jobs created before this column existed.';
