-- 239_lottery.sql
--
-- 抽奖功能(本地 MVP)数据表:
--   lottery_activities   活动配置
--   lottery_prizes       奖品配置(权重/库存/类型)
--   lottery_user_stats   用户抽奖资格状态(每用户一行)
--   lottery_draws        抽奖流水(只追加,幂等唯一约束)
--
-- 业务口径:
--   首抽: 正常账号(active、未删除)统一获得 1 次免费抽奖
--   阶梯: 基线后累计余额消耗达 $5 解锁第 2 抽,此后每新增 $10 解锁 1 次
--   余额消耗 = SUM(usage_logs.actual_cost) WHERE billing_type = 0 AND created_at > baseline_at
--   金额列用 DECIMAL 存储;服务层比较阈值时先换算为整数美分(round),避免浮点误差
--
-- 本 migration 幂等(IF NOT EXISTS),可重复执行。

-- ============================================
-- 活动表
-- ============================================

CREATE TABLE IF NOT EXISTS lottery_activities (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(100) NOT NULL,
    status         VARCHAR(20)  NOT NULL DEFAULT 'draft',
    rules_version  INTEGER      NOT NULL DEFAULT 1,
    starts_at      TIMESTAMPTZ,
    ends_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lottery_activities_status ON lottery_activities(status);

COMMENT ON TABLE lottery_activities IS '抽奖活动配置。状态: draft/active/paused/ended。';

-- ============================================
-- 奖品表
-- ============================================

CREATE TABLE IF NOT EXISTS lottery_prizes (
    id            BIGSERIAL PRIMARY KEY,
    activity_id   BIGINT        NOT NULL REFERENCES lottery_activities(id) ON DELETE CASCADE,
    name          VARCHAR(100)  NOT NULL,
    prize_type    VARCHAR(20)   NOT NULL DEFAULT 'none',
    value         DECIMAL(20,8) NOT NULL DEFAULT 0,
    weight        INTEGER       NOT NULL DEFAULT 0,
    stock         INTEGER       NOT NULL DEFAULT -1,
    stock_issued  INTEGER       NOT NULL DEFAULT 0,
    enabled       BOOLEAN       NOT NULL DEFAULT TRUE,
    sort_order    INTEGER       NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lottery_prizes_activity ON lottery_prizes(activity_id);
CREATE INDEX IF NOT EXISTS idx_lottery_prizes_activity_enabled ON lottery_prizes(activity_id, enabled);

COMMENT ON TABLE lottery_prizes IS '抽奖奖品配置。prize_type: none/balance_bonus/quota;stock: -1 表示无限。';

-- ============================================
-- 用户抽奖状态表
-- ============================================

CREATE TABLE IF NOT EXISTS lottery_user_stats (
    id                   BIGSERIAL PRIMARY KEY,
    user_id              BIGINT       NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    first_draw_granted   BOOLEAN      NOT NULL DEFAULT FALSE,
    baseline_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    manual_adjustment    INTEGER      NOT NULL DEFAULT 0,
    spend_offset_cents   BIGINT       NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE lottery_user_stats IS '用户抽奖资格状态。可用次数由服务层按 eligible - used 实时计算,不落库。';
COMMENT ON COLUMN lottery_user_stats.baseline_at IS '余额消耗基线:只统计该时刻之后 usage_logs 的新增消耗,不追溯历史。';

-- ============================================
-- 抽奖流水表(只追加)
-- ============================================

CREATE TABLE IF NOT EXISTS lottery_draws (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id            BIGINT        NOT NULL REFERENCES lottery_activities(id) ON DELETE CASCADE,
    prize_id               BIGINT        REFERENCES lottery_prizes(id) ON DELETE SET NULL,
    prize_name             VARCHAR(100)  NOT NULL,
    prize_type             VARCHAR(20)   NOT NULL,
    prize_value            DECIMAL(20,8) NOT NULL DEFAULT 0,
    rules_version          INTEGER       NOT NULL,
    source                 VARCHAR(20)   NOT NULL,
    balance_spent_at_draw  DECIMAL(20,10) NOT NULL DEFAULT 0,
    fulfillment_status     VARCHAR(20)   NOT NULL DEFAULT 'pending',
    fulfilled_at           TIMESTAMPTZ,
    fulfillment_error      TEXT,
    idempotency_key        VARCHAR(64)   NOT NULL,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lottery_draws_user ON lottery_draws(user_id);
CREATE INDEX IF NOT EXISTS idx_lottery_draws_user_created ON lottery_draws(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_lottery_draws_activity ON lottery_draws(activity_id);
CREATE INDEX IF NOT EXISTS idx_lottery_draws_prize ON lottery_draws(prize_id);
CREATE INDEX IF NOT EXISTS idx_lottery_draws_fulfillment ON lottery_draws(fulfillment_status);

-- 幂等唯一约束: 同一用户同一 idempotency_key 只允许一条流水。
-- 重复抽奖请求命中该约束时返回已有结果,不重复扣次数。
CREATE UNIQUE INDEX IF NOT EXISTS uq_lottery_draws_user_idempotency
    ON lottery_draws(user_id, idempotency_key);

COMMENT ON TABLE lottery_draws IS '抽奖流水(只追加)。used 次数 = 本表按 user_id 计数;奖品字段为中奖时刻快照。';
COMMENT ON COLUMN lottery_draws.source IS '本次抽奖消耗的次数来源: first(首抽)/threshold(阶梯)/manual(手动补发)。';

-- ============================================
-- 本地默认活动与奖品(幂等 seed)
-- ============================================
-- 仅当不存在任何活动时插入一套本地默认配置，供本地 MVP 直接验证:
--   谢谢参与(无限) 70 | $0.1 余额(库存100) 20 | $0.5 余额(库存20) 9 | $1 余额(库存5) 1

DO $$
DECLARE
    seeded_activity_id BIGINT;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM lottery_activities) THEN
        INSERT INTO lottery_activities (name, status)
        VALUES ('新人幸运抽奖', 'active')
        RETURNING id INTO seeded_activity_id;

        INSERT INTO lottery_prizes (activity_id, name, prize_type, value, weight, stock, enabled, sort_order) VALUES
            (seeded_activity_id, '谢谢参与',   'none',          0,   70, -1,  TRUE, 1),
            (seeded_activity_id, '$0.1 余额',  'balance_bonus', 0.1, 20, 100, TRUE, 2),
            (seeded_activity_id, '$0.5 余额',  'balance_bonus', 0.5, 9,  20,  TRUE, 3),
            (seeded_activity_id, '$1 余额',    'balance_bonus', 1,   1,  5,   TRUE, 4);
    END IF;
END $$;
