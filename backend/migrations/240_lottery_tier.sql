-- 240_lottery_tier.sql
--
-- 阶梯抽奖升级(本地 MVP):
--   阶梯 tier 按基线后累计余额消耗划分:
--     0 青铜 < $5 | 1 白银 >= $5 | 2 黄金 >= $15 | 3 钻石 >= $25 | 4 王者 >= $35
--   每个奖品新增:
--     min_tier     可中该奖的最低阶梯,低于该阶梯的奖品不参与随机
--     tier_weights 按阶梯覆盖权重(jsonb, key 为 tier 数字字符串);
--                  缺失的 tier 回退 weight 基础值,显式 0 表示该 tier 不可中
--   中奖概率 = tier_weights[tier] ?? weight / sum(候选奖品权重),由服务端执行。
--
-- 本 migration 幂等(IF NOT EXISTS / 条件 UPDATE / NOT EXISTS 插入),可重复执行。

-- ============================================
-- 奖品表新增阶梯字段
-- ============================================

ALTER TABLE lottery_prizes
    ADD COLUMN IF NOT EXISTS min_tier INTEGER NOT NULL DEFAULT 0;

ALTER TABLE lottery_prizes
    ADD COLUMN IF NOT EXISTS tier_weights JSONB;

COMMENT ON COLUMN lottery_prizes.min_tier IS '可中该奖品的最低用户阶梯(0 青铜/1 白银/2 黄金/3 钻石/4 王者)。';
COMMENT ON COLUMN lottery_prizes.tier_weights IS '按阶梯覆盖权重: {"0":70,"1":50,...};缺失回退 weight,显式 0 表示该阶梯不可中。';

-- ============================================
-- 本地默认活动 seed 升级为阶梯奖品
-- ============================================
-- 仅当存在本地默认活动「新人幸运抽奖」且其奖品尚未配置阶梯时执行:
--   谢谢参与: 70/50/30/15/5
--   $0.1 余额: 25/20/10/0/0
--   $1 余额:    5/20/30/25/0
--   $2 余额:    -/10/20/30/25   (min_tier=1,新增)
--   $5 余额:    -/-/10/30/70    (min_tier=2,新增)
--   $0.5 余额:  旧档位退役(enabled=false),保留流水可追溯

DO $$
DECLARE
    seeded_activity_id BIGINT;
BEGIN
    SELECT id INTO seeded_activity_id
    FROM lottery_activities
    WHERE name = '新人幸运抽奖'
    LIMIT 1;

    IF seeded_activity_id IS NULL THEN
        RETURN;
    END IF;

    -- 幂等: 已配置过阶梯(任一奖品有 tier_weights 或 min_tier>0)则跳过
    IF EXISTS (
        SELECT 1 FROM lottery_prizes
        WHERE activity_id = seeded_activity_id
          AND (min_tier > 0 OR tier_weights IS NOT NULL)
    ) THEN
        RETURN;
    END IF;

    UPDATE lottery_prizes SET
        tier_weights = '{"0":70,"1":50,"2":30,"3":15,"4":5}'::jsonb,
        sort_order = 1, updated_at = NOW()
    WHERE activity_id = seeded_activity_id AND prize_type = 'none';

    UPDATE lottery_prizes SET
        tier_weights = '{"0":25,"1":20,"2":10,"3":0,"4":0}'::jsonb,
        sort_order = 2, updated_at = NOW()
    WHERE activity_id = seeded_activity_id AND prize_type = 'balance_bonus' AND value = 0.1;

    UPDATE lottery_prizes SET
        tier_weights = '{"0":5,"1":20,"2":30,"3":25,"4":0}'::jsonb,
        weight = 5, stock = 50,
        sort_order = 3, updated_at = NOW()
    WHERE activity_id = seeded_activity_id AND prize_type = 'balance_bonus' AND value = 1;

    UPDATE lottery_prizes SET
        enabled = FALSE, weight = 0, updated_at = NOW()
    WHERE activity_id = seeded_activity_id AND prize_type = 'balance_bonus' AND value = 0.5;

    INSERT INTO lottery_prizes (activity_id, name, prize_type, value, weight, min_tier, tier_weights, stock, enabled, sort_order)
    SELECT seeded_activity_id, '$2 余额', 'balance_bonus', 2, 0, 1,
           '{"0":0,"1":10,"2":20,"3":30,"4":25}'::jsonb, 20, TRUE, 4
    WHERE NOT EXISTS (
        SELECT 1 FROM lottery_prizes
        WHERE activity_id = seeded_activity_id AND prize_type = 'balance_bonus' AND value = 2
    );

    INSERT INTO lottery_prizes (activity_id, name, prize_type, value, weight, min_tier, tier_weights, stock, enabled, sort_order)
    SELECT seeded_activity_id, '$5 余额', 'balance_bonus', 5, 0, 2,
           '{"0":0,"1":0,"2":10,"3":30,"4":70}'::jsonb, 10, TRUE, 5
    WHERE NOT EXISTS (
        SELECT 1 FROM lottery_prizes
        WHERE activity_id = seeded_activity_id AND prize_type = 'balance_bonus' AND value = 5
    );
END $$;
