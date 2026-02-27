# Product Requirements Document — lurus-identity
# 产品需求文档 — lurus-identity

**版本**: 1.0
**日期**: 2026-02-27
**状态**: 生产规格 (Production Spec)
**服务端口**: 18104
**命名空间**: lurus-identity

---

## 目录 (Table of Contents)

1. [产品概述 (Product Overview)](#1-产品概述)
2. [目标与成功指标 (Objectives & Key Results)](#2-目标与成功指标)
3. [用户角色 (User Personas)](#3-用户角色)
4. [功能需求 (Functional Requirements)](#4-功能需求)
5. [非功能性需求 (Non-Functional Requirements)](#5-非功能性需求)
6. [边界与集成 (Boundaries & Integrations)](#6-边界与集成)
7. [约束与假设 (Constraints & Assumptions)](#7-约束与假设)
8. [已知风险 (Known Risks)](#8-已知风险)

---

## 1. 产品概述 (Product Overview)

### 1.1 服务定位

lurus-identity 是 Lurus 平台的**统一用户层基础设施服务**（Identity Platform Foundation Service）。它不面向最终用户独立存在，而是作为所有 Lurus 产品（lurus-api、lurus-gushen、lurus-webmail）的权威用户数据源和计费引擎。

平台所有产品共享同一套账号体系（LurusID）、同一个钱包（1 Credit = 1 CNY）、同一套权益引擎，以及统一的 VIP 忠诚度积分。用户在任何一个产品的消费均计入同一生命周期总消费，统一升级 VIP 等级，享受跨产品折扣权益。

### 1.2 目标用户

| 用户类型 | 描述 |
|----------|------|
| 普通用户 | 通过 Zitadel OIDC 登录、管理账号、订阅产品、钱包充值的个人用户 |
| 企业用户 | 采购 Enterprise 套餐、需要发票、使用 Stripe 国际支付的企业客户 |
| 平台管理员 | 通过 Admin API 管理账号、调整钱包、配置套餐的运营人员 |
| 内部服务 | lurus-api、lurus-gushen、lurus-webmail 等通过 Internal API 查询权益的下游服务 |

### 1.3 核心价值主张

1. **统一账号**: 一个 LurusID，串联全部产品，无需重复注册。
2. **统一钱包**: Credits 跨产品通用，充值一次，全线消费。
3. **权益一致性**: 下游服务查询权益 P99 < 5ms（Redis 缓存），任何时刻各产品权益与订阅状态严格一致。
4. **VIP 忠诚度**: 跨产品累计消费驱动 VIP 升级，给予折扣、优先支持等特权，提升留存。
5. **支付灵活性**: 国内（易支付/支付宝/微信支付）、国际（Stripe/Creem）双线并行，用户无摩擦结算。

---

## 2. 目标与成功指标 (Objectives & Key Results)

### Objective 1: 成为 Lurus 平台的可靠用户基础设施

| Key Result | 目标值 | 度量方式 |
|------------|--------|----------|
| 权益查询接口可用性 | ≥ 99.9%（月统计） | Prometheus `up` 指标 + Alertmanager |
| 权益查询 P99 延迟 | < 5ms（Redis 命中） / < 50ms（DB 回源） | Jaeger / Prometheus histogram |
| 支付 Webhook 处理幂等率 | 100%（重复回调不产生重复入账） | 数据库唯一约束 + 订单状态机校验 |
| 内部 API 错误率 | < 0.1%（5xx / total） | Prometheus counter |

### Objective 2: 驱动付费转化

| Key Result | 目标值 | 度量方式 |
|------------|--------|----------|
| 免费用户付费转化率 | ≥ 8%（90天内） | 订阅记录分析 |
| 充值成功率 | ≥ 95% | payment_orders 状态统计 |
| 兑换码使用率 | ≥ 60%（批次发放后30天内） | redemption_codes.used_count |
| 推荐转化奖励发放准确率 | 100%（无遗漏、无重复） | referral_reward_events 审计 |

### Objective 3: 提升用户 LTV（生命周期价值）

| Key Result | 目标值 | 度量方式 |
|------------|--------|----------|
| VIP Silver+ 用户留存率 | ≥ 85%（季度） | account_vip + subscriptions 交叉分析 |
| 订阅续费率（月付） | ≥ 70% | 订阅状态迁移统计 |
| 年付套餐占比 | ≥ 20%（付费用户中） | product_plans.billing_cycle 统计 |

### Objective 4: 合规与安全

| Key Result | 目标值 | 度量方式 |
|------------|--------|----------|
| 审计日志覆盖率 | 100% 的关键操作（认证/支付/状态变更）均有结构化日志 | slog JSON 输出分析 |
| GDPR 删除请求响应时效 | ≤ 30 天 | 工单系统 + GDPR 删除 API |
| Zitadel JWT 验证完整实施 | 生产环境无 placeholder 代码 | 代码审查 + 集成测试 |

---

## 3. 用户角色 (User Personas)

### Persona A: 个人开发者（林峰，28岁）

**背景**: 独立开发者，订阅 llm-api Basic 套餐，通过支付宝充值 Credits，偶尔用兑换码。

**痛点**:
- 充值后希望立刻看到余额变化，不愿等待
- 担心余额不足时 API 调用被静默失败，希望有预警
- 不习惯订阅制，更喜欢用多少充多少

**需求**:
- 实时余额展示，充值后 < 3 秒刷新
- 余额预警提醒（低于阈值时推送）
- 明细账单，每笔扣费都能追溯

### Persona B: 企业采购（张明，42岁，CTO）

**背景**: 团队 20 人使用 quant-trading Enterprise 套餐，通过 Stripe 美元结算，需要 VAT 发票。

**痛点**:
- 每次续费需要财务审批流程，支持年付优先
- 必须提供合规发票，否则无法报销
- 账号安全性要求高，要求强制 MFA

**需求**:
- Stripe 订阅年付，自动续费
- 发票生成 API / 管理后台导出
- Admin 端可管理子账号权限

### Persona C: 平台运营（Anita，运营主管）

**背景**: 负责用户运营、促销活动、VIP 等级配置，使用 Admin API 和管理后台。

**痛点**:
- 手动给用户调整余额易出错，需要操作记录
- 希望批量发放兑换码，支持设定有效期和使用次数
- 想了解哪些 VIP 等级用户贡献了最多收入

**需求**:
- Admin 操作全审计，每次调整记录操作者
- 批量兑换码生成与导出
- VIP 等级配置热更新（不重启服务）

### Persona D: 下游服务（lurus-api 网关）

**背景**: 每次 LLM 请求前需查询用户权益（配额、模型分组），P99 < 5ms 是硬性要求。

**需求**:
- Internal API `GET /internal/v1/accounts/:id/entitlements/:product_id` 高可用、低延迟
- 权益缓存 TTL 5 分钟，订阅变更时主动失效
- 在 lurus-identity 不可用时，允许下游服务降级（返回最近一次缓存快照）

---

## 4. 功能需求 (Functional Requirements)

需求优先级定义：
- **P0**: 上线阻塞项，必须完成才能生产发布
- **P1**: 高优先级，第一个迭代内完成
- **P2**: 重要增强，第二个迭代内完成

---

### Epic 1: 账号管理 (Account Management)

#### FR-ACC-001: Zitadel JWT 完整验证 [P0]

**描述**: `/api/v1/*` 所有用户端点必须通过 Zitadel JWKS 端点验证 JWT 签名、`exp`、`iss`、`aud` 字段。禁止信任 `X-Account-ID` 明文头（当前 placeholder 实现）。

**验收标准**:
- 过期 JWT 返回 `401 Unauthorized`，错误消息 `"token expired"`
- 无效签名返回 `401 Unauthorized`，错误消息 `"invalid token signature"`
- `iss` 不匹配返回 `401 Unauthorized`
- JWKS 端点不可用时服务降级：返回 503，不接受任何请求（fail-closed）
- JWKS 公钥本地缓存 1 小时，过期自动刷新；刷新失败使用旧缓存，记录告警
- 单元测试覆盖：有效 token、过期 token、篡改 token、错误 issuer 四种场景

#### FR-ACC-002: 账号自动创建与 Upsert [P0]

**描述**: 用户首次通过 Zitadel 认证时，若 `lurus-identity` 中无对应账号，自动创建账号并分配 LurusID（格式 `LU` + 7位零填充数字，如 `LU0001234`）。

**验收标准**:
- LurusID 全局唯一，生成使用数据库序列，不可碰撞
- 同一 Zitadel Sub 的并发注册请求（最高 10 并发）只创建一个账号（数据库唯一约束保护）
- 新账号自动创建钱包（balance=0）和所有产品的 free 订阅
- 推荐码（`aff_code`）在账号创建时随机生成，8 位字母数字，全局唯一
- `Internal POST /internal/v1/accounts/upsert` 幂等：相同 Zitadel Sub 重复调用返回同一账号

#### FR-ACC-003: OAuth 多绑定 [P1]

**描述**: 用户可将 GitHub、Discord、微信、Telegram、Linux.do 等第三方 OAuth 账号绑定到同一 LurusID。

**验收标准**:
- 同一 provider+provider_id 只能绑定一个 LurusID；若已绑定其他账号返回 `409 Conflict`
- 绑定解除时校验用户还有其他登录方式（防锁号）
- 绑定/解除操作记录到结构化审计日志（who=account_id, action=oauth_bind/unbind, provider）

#### FR-ACC-004: 账号信息读写 [P1]

**描述**: 用户可读写自己的 `display_name`、`locale`；管理员可查看全部账号字段。

**验收标准**:
- `GET /api/v1/account/me` 返回账号基础信息、VIP 等级、全部订阅状态
- `PUT /api/v1/account/me` 仅允许修改 `display_name`（1-64 字符）、`locale`（合法 BCP-47）
- `display_name` 非空校验；注入特殊字符（`<script>`）不产生 XSS
- 管理员 `GET /admin/v1/accounts` 支持关键词搜索、分页（最大 page_size=100）

#### FR-ACC-005: 账号状态管理 [P1]

**描述**: 管理员可将账号设为 suspended（冻结）或 deleted（软删除）状态。

**验收标准**:
- suspended 账号登录时返回 `403 Forbidden`，消息 `"account suspended"`
- deleted 账号不出现在任何用户端查询中（逻辑删除，数据保留 90 天用于审计）
- 状态变更触发 NATS 事件 `identity.account.suspended` / `identity.account.deleted`
- 下游服务收到状态变更事件后 < 5 分钟内停止服务该账号（通过权益缓存失效实现）

---

### Epic 2: 产品与套餐管理 (Product & Plan Management)

#### FR-PROD-001: 产品目录读取 [P0]

**描述**: 所有产品和套餐数据存储在 `identity.products` 和 `identity.product_plans` 表中，支持零代码变更扩展新功能项（通过 `features` JSONB）。

**验收标准**:
- `GET /api/v1/products` 仅返回 `status=1` 的产品列表，按 `sort_order` 排序
- `GET /api/v1/products/:id/plans` 返回指定产品全部有效套餐，含 `features` JSONB
- 产品列表接口响应时间 < 20ms（内存/Redis 缓存）
- 管理员可通过 Admin API 新增/修改产品和套餐，无需重启服务

#### FR-PROD-002: 套餐管理 Admin API [P1]

**描述**: 管理员通过 `POST /admin/v1/products/:id/plans` 和 `PUT /admin/v1/plans/:id` 管理套餐。

**验收标准**:
- 创建套餐时 `product_id + code` 组合唯一，重复返回 `409`
- `billing_cycle` 枚举值校验：`forever / weekly / monthly / quarterly / yearly / one_time`
- `price_cny >= 0`，`features` 必须是合法 JSON 对象
- 套餐下架（`status=0`）不影响已有订阅；用户查询套餐时不显示下架套餐

---

### Epic 3: 订阅管理 (Subscription Management)

#### FR-SUB-001: 订阅激活 [P0]

**描述**: 支付成功后，SubscriptionService.Activate 创建新订阅，同步更新权益快照，失效 Redis 缓存。

**验收标准**:
- 每个账号每个产品同时只有一个 `active/grace/trial` 状态订阅（数据库唯一索引保障）
- 激活时先将旧订阅置为 `expired`，再创建新订阅（原子性：DB 事务内完成）
- `billing_cycle=monthly` 时到期日为购买日 + 1 自然月（月末日期 clamp 到目标月末，如 1月31日 + 1月 → 2月28日）
- `billing_cycle=forever` 时 `expires_at=NULL`，永不到期
- 激活后发布 NATS 事件 `identity.subscription.activated`，含 `account_id / product_id / plan_code`

#### FR-SUB-002: 订阅到期与宽限期 [P0]

**描述**: 定时任务（Cron）每小时扫描即将到期订阅，进入 7 天宽限期（grace），宽限期结束后权益降为 free。

**验收标准**:
- Cron 任务在单 Pod 场景和多 Pod 场景均只执行一次（使用 Redis 分布式锁 + leader election，或 Kubernetes CronJob 单实例）
- 到期时订阅状态变为 `grace`，`grace_until = expires_at + 7天`；权益不立即降级
- `grace_until` 过期后状态变为 `expired`，权益重置为 free（`plan_code=free`），缓存失效
- 每次状态迁移发布对应 NATS 事件（`identity.subscription.grace_started` / `identity.subscription.expired`）
- Cron 异常时告警（Prometheus gauge `subscription_cron_last_success_timestamp`），不影响主服务

#### FR-SUB-003: 订阅取消 [P1]

**描述**: 用户主动取消订阅，关闭自动续费，订阅在当前周期结束前保持有效。

**验收标准**:
- `POST /api/v1/subscriptions/:product_id/cancel` 将 `auto_renew=false`，状态变为 `cancelled`
- 取消后权益在 `expires_at` 前保持不变，到期后才降级（走宽限期流程）
- 已取消的订阅不可再次取消（返回 `400 Bad Request`，消息 `"subscription already cancelled"`）
- 取消操作发布 NATS 事件 `identity.subscription.cancelled`

#### FR-SUB-004: 套餐升降级 [P1]

**描述**: 用户在当前周期内升级或降级套餐，差价按剩余天数比例计算。

**验收标准**:
- 升级：立即生效，差价 = (新套餐月费 - 旧套餐月费) × (剩余天数 / 当前周期总天数)，从钱包扣除
- 降级：在当前周期结束时生效（记录 `pending_downgrade_plan_id`），不退款
- 钱包余额不足时升级返回 `402 Payment Required`，消息说明所需差价金额
- 差价计算按实际日历天数，使用 UTC 时区
- 升降级操作记录到结构化审计日志

#### FR-SUB-005: 订阅生命周期邮件通知 [P1]

**描述**: 关键订阅事件触发邮件通知。

**验收标准**:
- 触发邮件的事件：订阅激活、到期前 7 天预警、宽限期开始、宽限期结束（降级）、取消确认
- 邮件发送通过 NATS 事件解耦（lurus-identity 发事件，邮件服务消费）
- 发送失败不影响主流程（异步、最多重试 3 次）
- 邮件模板支持中英文（根据账号 `locale` 字段选择）

---

### Epic 4: 钱包与计费 (Wallet & Billing)

#### FR-WAL-001: 钱包账本不可变性 [P0]

**描述**: `billing.wallet_transactions` 表为 append-only 不可变账本，禁止 UPDATE / DELETE。余额通过事务内原子 CAS 操作保证一致性。

**验收标准**:
- 扣款前校验余额 `balance - amount >= 0`，余额不足返回 `402 Payment Required`
- 所有余额变更通过数据库事务完成（Credit/Debit 在同一事务内更新 wallet 和插入 transaction）
- `wallet_transactions` 无 UPDATE/DELETE 权限（通过 PostgreSQL 行级安全或应用层约束）
- 并发扣款测试：10 个并发扣款请求，余额不出现负数，事务记录无重复

#### FR-WAL-002: 钱包充值（Topup）[P0]

**描述**: 用户通过外部支付商充值 Credits（1 CNY = 1 Credit），支付成功后自动入账。

**验收标准**:
- `POST /api/v1/wallet/topup` 创建 pending 订单并返回支付 URL，最小充值额 ¥1
- 支付回调幂等：相同 `order_no` 重复回调只入账一次（`payment_orders.status` 状态机检查）
- 充值成功后 `lifetime_topup` 累计增加，触发 VIP 重计算
- 充值金额字段精度：`DECIMAL(14,4)`，防止浮点精度丢失
- 充值成功发布 NATS 事件 `identity.wallet.topped_up`

#### FR-WAL-003: 支付提供商集成 [P0]

**描述**: 支持易支付（支付宝/微信支付）、Stripe、Creem 三个支付渠道，任一渠道配置缺失时跳过该渠道（不影响其他渠道）。

**验收标准**:

**易支付（Epay）**:
- GET `/webhook/epay` 接收异步通知，使用 MD5 签名校验
- `trade_status=TRADE_SUCCESS` 且签名验证通过时入账
- 签名验证失败返回字符串 `"fail"`，不处理订单

**Stripe**:
- POST `/webhook/stripe` 使用 `stripe.VerifyWebhookSignature` 校验 `Stripe-Signature` 头
- 处理 `checkout.session.completed` 事件，通过 `client_reference_id` 关联内部订单
- 签名验证失败返回 `400 Bad Request`

**Creem**:
- POST `/webhook/creem` 校验 `X-Creem-Signature` 头
- 处理 `payment.success` 事件类型
- 签名验证失败返回 `401 Unauthorized`

**通用要求**:
- 所有 Webhook 端点响应时间 < 3 秒（否则支付商重试）
- 支付提供商不可用时（超时/5xx）触发熔断器，告警，不影响其他渠道

#### FR-WAL-004: 钱包余额查询与账单明细 [P1]

**描述**: 用户可查看当前余额和历史交易明细。

**验收标准**:
- `GET /api/v1/wallet` 返回 `balance`、`frozen`、`lifetime_topup`、`lifetime_spend`
- `GET /api/v1/wallet/transactions` 分页返回交易记录，按 `created_at DESC` 排序
- 每条记录含：类型（中文描述）、金额、变更后余额、关联产品、时间
- 分页最大 `page_size=100`，默认 20

#### FR-WAL-005: 兑换码系统 [P1]

**描述**: 运营人员批量生成兑换码，用户输入后获得 Credits、试用订阅或配额赠送。

**验收标准**:
- 兑换码格式：8 位大写字母数字，全局唯一
- 支持类型：`credits`（充值 N Credits）、`subscription_trial`（激活 N 天试用）、`quota_grant`（赠送额外配额）
- 每个码有独立使用次数上限（`max_uses`）和有效期（`expires_at`）
- 用户兑换前校验：有效期、使用次数上限、用户是否已兑换同批次码（同批次每用户限兑一次）
- 兑换操作原子性：使用次数 +1 和钱包入账在同一数据库事务中完成
- 兑换记录写入 `wallet_transactions`，`reference_type=redemption_code`

#### FR-WAL-006: 发票生成 [P2]

**描述**: 企业用户可为已支付订单申请发票。

**验收标准**:
- Admin 后台可导出指定账号、时间范围的支付记录 CSV
- `payment_orders` 记录含完整支付信息，足以手动或自动开具发票
- 后续迭代支持对接电子发票 API（预留接口）

---

### Epic 5: 权益引擎 (Entitlement Engine)

#### FR-ENT-001: 权益快照同步 [P0]

**描述**: 订阅激活/变更时，将套餐 `features` JSONB 展开写入 `identity.account_entitlements`，作为下游服务查询的单一数据源。

**验收标准**:
- `SyncFromSubscription` 在订阅激活后 < 1 秒内完成
- 权益键值为字符串，类型通过 `value_type` 字段声明（`string/integer/decimal/boolean`）
- 同一 `(account_id, product_id, key)` 的写入为 UPSERT（幂等）
- 同步失败时记录错误日志并触发异步重试（NATS 重投）

#### FR-ENT-002: Redis 权益缓存 [P0]

**描述**: 权益数据缓存在 Redis DB 3，TTL 5 分钟，订阅变更时主动失效。

**验收标准**:
- 缓存命中时 `GET /internal/v1/accounts/:id/entitlements/:product_id` P99 < 5ms
- 缓存失效（Invalidate）在订阅状态任何变更后 < 100ms 内完成
- Redis 不可用时降级到 DB 查询，响应时间 < 50ms，记录告警
- 缓存 key 格式：`ident:ent:{account_id}:{product_id}`，禁止存储个人敏感信息

#### FR-ENT-003: 权益回落（Fallback）[P0]

**描述**: 账号无任何订阅或订阅过期时，权益引擎自动返回 free 套餐的权益值。

**验收标准**:
- `plan_code=free` 始终存在于权益快照中（即使无任何订阅）
- 权益 `Get` 方法从不返回空 map（最差情况返回 `{"plan_code": "free"}`）
- 下游服务收到空权益或错误时，应默认使用 free 套餐（合约约定，非本服务强制）

#### FR-ENT-004: 管理员手动授权 [P1]

**描述**: 管理员可为账号手动授予特定权益（如 VIP 测试、补偿授权）。

**验收标准**:
- `POST /admin/v1/accounts/:id/grant` 接受 `product_id / key / value`，写入 `account_entitlements`，`source=admin_grant`
- 手动授权不覆盖订阅同步的权益（通过 `source` 区分；冲突时以最新操作时间为准）
- 操作记录到结构化审计日志（admin_id / target_account_id / key / value）

---

### Epic 6: VIP 忠诚度系统 (VIP Loyalty)

#### FR-VIP-001: VIP 等级计算规则 [P0]

**描述**: VIP 等级取 `MAX(yearly_sub_grant, spend_grant)`，spend_grant 由 `lifetime_topup` 对照 `vip_level_configs` 计算，yearly_sub_grant 在年付订阅激活时设置。

**验收标准**:

| 等级 | 名称 | 累计充值门槛 | 年付最低套餐 | 折扣 |
|------|------|------------|------------|------|
| 0 | Standard | ¥0 | — | 无折扣 |
| 1 | Silver | ¥500 | basic | 98折 |
| 2 | Gold | ¥2000 | pro | 95折 |
| 3 | Platinum | ¥10000 | pro | 92折 |
| 4 | Diamond | ¥50000 | enterprise | 90折 |

- VIP 等级重计算在每次充值成功后触发（幂等）
- `vip_level_configs` 可通过数据库热更新，无需重启服务
- VIP 等级变更发布 NATS 事件 `identity.vip.upgraded` / `identity.vip.downgraded`

#### FR-VIP-002: VIP 折扣应用 [P1]

**描述**: 用户订阅结算时，按当前 VIP 等级的 `global_discount` 计算实付金额。

**验收标准**:
- 折扣应用在 Checkout 阶段，实付金额 = 套餐标价 × `global_discount`，精度 2 位小数
- 折扣信息展示在结算页，用户可见折扣前后价格
- 钱包支付场景直接扣折后金额；外部支付场景创建折后金额的支付订单
- 折扣变更（管理员修改配置）不影响已创建的 pending 订单

#### FR-VIP-003: VIP 特权展示 [P1]

**描述**: 前端用户中心展示当前 VIP 等级、特权内容、升级进度（QQ 钻石风格）。

**验收标准**:
- 展示当前等级图标、等级名称、全局折扣率
- 展示距离下一等级所需充值金额（进度条）
- Diamond 等级显示专属徽章，`dedicated_manager=true` 时显示"专属客服"入口
- 移动端响应式适配（Semi UI 组件）

#### FR-VIP-004: 管理员 VIP 手动设置 [P1]

**描述**: 管理员可强制设置账号 VIP 等级（用于补偿或测试）。

**验收标准**:
- `POST /admin/v1/accounts/:id/vip` 接受 `level`（0-4），立即生效
- 手动设置记录到审计日志（admin_id / target_account_id / new_level / reason）
- 手动设置的等级在下次充值触发自动重算时可被覆盖（运营需知晓此行为）

---

### Epic 7: 推荐系统 (Referral System)

#### FR-REF-001: 推荐码生成与绑定 [P1]

**描述**: 每个账号拥有唯一推荐码（`aff_code`），新用户注册时可绑定推荐人。

**验收标准**:
- 推荐关系在账号创建时绑定（`referrer_id`），创建后不可修改
- 自我推荐检测：推荐人 ID 不可等于被推荐人 ID，返回 `400 Bad Request`
- 推荐链不超过 1 层（不支持多级推荐）

#### FR-REF-002: 推荐奖励发放 [P1]

**描述**: 被推荐用户完成特定动作时，推荐人获得 Credits 奖励。

**验收标准**:

| 触发事件 | 推荐人奖励 |
|----------|-----------|
| 被推荐人注册 | 5 Credits |
| 被推荐人首次充值 | 10 Credits |
| 被推荐人首次订阅付费套餐 | 20 Credits |

- 每个触发事件每对（referrer, referee）只奖励一次（`referral_reward_events` 唯一性校验）
- 奖励发放写入 `wallet_transactions`，`type=referral_reward`
- 奖励发放记录 `referral_reward_events`，`status=completed`

---

### Epic 8: 管理后台 (Admin Console)

#### FR-ADM-001: Admin JWT 验证 [P0]

**描述**: `/admin/v1/*` 所有端点必须验证 Zitadel JWT 中 `roles` 或 `groups` claim 包含 `lurus:admin`，当前 placeholder 实现必须替换。

**验收标准**:
- 无 `lurus:admin` role 的 JWT 访问 admin 端点返回 `403 Forbidden`
- Admin 操作全部携带操作者的 `account_id`（从 JWT claims 解析），写入审计日志
- Admin JWT 验证失败不影响用户端 JWT 验证

#### FR-ADM-002: 结构化审计日志 [P0]

**描述**: 所有关键操作（认证、支付、状态变更、admin 操作）输出 JSON 结构化日志，包含操作者、目标、操作类型、结果。

**验收标准**:
- 日志格式（JSON，slog）：`{"time":"...","level":"INFO","msg":"audit","who":123,"action":"wallet.credit","target":456,"amount":100,"result":"ok"}`
- 必须包含字段：`time / level / who / action / target / result`
- 敏感字段（支付卡号、webhook payload 原文）不进入日志
- 日志由 K8s fluentd/vector 采集，保留 90 天

#### FR-ADM-003: 兑换码批量管理 [P2]

**描述**: 管理员可批量生成、查询、禁用兑换码。

**验收标准**:
- `POST /admin/v1/redemption-codes/batch` 一次最多生成 1000 个码，返回 CSV
- 支持设置：有效期、使用上限、奖励类型、奖励金额、批次 ID
- 批次 ID 用于统计和批量禁用（`PUT /admin/v1/redemption-codes/batch/:batch_id/disable`）

---

### Epic 9: 前端用户界面 (Frontend)

#### FR-FE-001: 钱包与充值页面 [P1]

**描述**: React 前端展示钱包余额、交易明细、充值入口。

**验收标准**:
- 余额展示精度 2 位小数，充值成功后自动刷新（WebSocket 或轮询 3 秒）
- 充值金额输入：最小 ¥1，最大 ¥99999，数字键盘友好
- 支付方式图标清晰（支付宝/微信/Stripe/Creem），点击跳转对应收银台
- 充值结果页展示成功/失败状态，含订单号和充值金额

#### FR-FE-002: 订阅管理页面 [P1]

**描述**: 展示当前订阅、套餐对比、续费/升级入口。

**验收标准**:
- 套餐对比表格展示三个产品的所有套餐，特性通过 `features` JSONB 动态渲染
- 当前套餐高亮，到期日和自动续费状态清晰展示
- 宽限期内显示醒目提示（倒计时）
- 移动端套餐对比改为竖向滑动卡片布局

#### FR-FE-003: VIP 权益展示 [P1]

**描述**: QQ 钻石风格的 VIP 等级展示与权益说明。

**验收标准**:
- 五个等级使用差异化颜色系统（Standard 灰 / Silver 银 / Gold 金 / Platinum 铂金 / Diamond 钻石蓝）
- 展示当前等级动态图标、全局折扣、特权列表（`perks_json` 动态渲染）
- 升级进度条展示 `lifetime_topup` 相对下一等级门槛的百分比
- Diamond 等级有闪光动效（CSS animation，不影响性能）

#### FR-FE-004: 管理后台页面 [P2]

**描述**: 运营人员使用的 Web 管理界面。

**验收标准**:
- 账号列表：搜索（email/LurusID）、分页、点击查看详情
- 详情页：账号信息、VIP 等级（可编辑）、钱包余额（可手动调整）、订阅列表、操作历史
- 兑换码管理：批量生成、批次列表、使用统计
- Admin 操作前弹出二次确认对话框，含操作摘要

---

### Epic 10: 可观测性与运维 (Observability & Operations)

#### FR-OPS-001: Prometheus Metrics [P0]

**描述**: 暴露关键业务和系统指标，供 Alertmanager 告警使用。

**验收标准**:
- HTTP 端点 `GET /metrics` 暴露 Prometheus 格式指标
- 必须包含的指标：
  - `identity_http_requests_total{method,path,status}` — 请求计数
  - `identity_http_request_duration_seconds{method,path}` — 延迟分布
  - `identity_wallet_topup_total{method,status}` — 充值成功/失败计数
  - `identity_subscription_active_total{product_id,plan_code}` — 活跃订阅数
  - `identity_entitlement_cache_hits_total` / `identity_entitlement_cache_misses_total`
  - `identity_subscription_cron_last_success_timestamp` — Cron 存活检测
  - `identity_payment_provider_errors_total{provider}` — 支付商错误计数
- 指标命名遵循 Prometheus 最佳实践（snake_case，_total 后缀 for counters）

#### FR-OPS-002: 分布式追踪 (Jaeger/OpenTelemetry) [P1]

**描述**: 关键请求路径注入 OpenTelemetry Trace，便于跨服务排查延迟问题。

**验收标准**:
- 所有 HTTP 请求自动注入 TraceID/SpanID 到 slog 日志上下文
- Internal API 调用链（lurus-api → lurus-identity）可在 Jaeger UI 完整追踪
- 支付 Webhook 处理全链路可追踪（Webhook 接收 → 订单状态变更 → 钱包入账 → 权益同步）

#### FR-OPS-003: 支付提供商熔断器 [P1]

**描述**: 当某个支付提供商连续失败超过阈值时，自动熔断，避免级联等待。

**验收标准**:
- 每个 Provider 独立熔断器（基于 gobreaker 或 sony/gobreaker）
- 熔断阈值：60 秒内失败率 > 50% 且请求数 > 10，进入 Open 状态
- Open 状态下立即返回错误，不等待超时；Half-Open 状态探测恢复
- 熔断状态变更记录 slog WARN 日志，并更新 Prometheus gauge `identity_payment_circuit_state{provider}`

#### FR-OPS-004: 限流中间件 [P1]

**描述**: 防止单个用户或 IP 的滥用请求导致服务过载。

**验收标准**:
- 用户端 API（`/api/v1/*`）：每个 account_id 每分钟限 120 次请求（滑动窗口，Redis 实现）
- Webhook 端点（`/webhook/*`）：每个来源 IP 每秒限 10 次请求
- 超限返回 `429 Too Many Requests`，含 `Retry-After` 头
- 限流不影响 Internal API 和健康检查端点

---

### Epic 11: 合规与数据治理 (Compliance & Data Governance)

#### FR-COMP-001: GDPR 数据删除 API [P1]

**描述**: 应欧盟用户要求，提供完整的个人数据删除接口（"被遗忘权"）。

**验收标准**:
- `DELETE /admin/v1/accounts/:id/personal-data` 执行以下操作：
  1. 账号状态置为 `deleted`（软删除）
  2. `email`、`display_name`、`avatar_url`、`phone` 替换为 anonymized 占位值
  3. `account_oauth_bindings` 删除该账号全部绑定记录
  4. `wallet_transactions` 的 `description` 字段中的个人信息清除（账本记录保留用于财务合规）
- 操作不可逆，执行前需要 Admin 二次确认（请求体含 `confirm=true`）
- 操作完成后发布 NATS 事件 `identity.account.gdpr_deleted`
- 响应时间 < 5 秒（单次 API 调用内完成）

#### FR-COMP-002: 数据保留策略 [P2]

**描述**: 不同类型数据的保留期限配置与自动清理。

**验收标准**:
- `wallet_transactions` 保留 7 年（财务法规）
- `payment_orders` 保留 7 年
- `referral_reward_events` 保留 3 年
- 软删除账号（`status=deleted`）的个人信息在 90 天后通过定时任务 anonymize
- 保留策略文档化，可通过配置变更而非代码变更调整

---

## 5. 非功能性需求 (Non-Functional Requirements)

### 5.1 性能 (Performance)

| 指标 | 要求 | 备注 |
|------|------|------|
| 权益查询 P99 延迟 | < 5ms（缓存命中） | Redis 直读 |
| 权益查询 P99 延迟 | < 50ms（缓存未命中） | DB 回源 |
| 用户端 API P99 延迟 | < 200ms | 不含外部支付跳转 |
| 充值回调处理延迟 | < 3s | 支付商重试窗口 |
| 并发处理能力 | ≥ 500 RPS（单 Pod） | 基准测试验证 |
| 启动时间 | < 5s | 包含 DB/Redis/NATS 连接建立 |

### 5.2 安全 (Security)

| 类别 | 要求 |
|------|------|
| JWT 验证 | 完整 JWKS 验证（签名、exp、iss、aud），禁止信任 header 传入的 account_id |
| Internal API | 固定长度（≥32 字节）随机 Bearer token，不得使用生产 token 测试 |
| Webhook 签名 | 各 Provider 使用官方 SDK 签名验证，拒绝未签名请求 |
| 数据传输 | 全站 HTTPS/TLS 1.2+，Kubernetes 内部 mTLS（Istio 或 Traefik） |
| 容器安全 | `readOnlyRootFilesystem=true`、`runAsNonRoot=true`、`capabilities.drop=[ALL]` |
| 秘钥管理 | 所有秘钥通过 K8s Secret 注入，代码库中禁止明文秘钥 |
| SQL 注入 | GORM 参数化查询，禁止拼接 SQL 字符串 |
| 余额枚举 | 订单不存在和无权限返回相同错误消息（防枚举） |
| 速率限制 | 用户端 120 req/min，Webhook IP 级 10 req/s |
| 依赖扫描 | 每次 CI 运行 `govulncheck`，Critical 漏洞阻断发布 |

### 5.3 可用性 (Availability)

| 指标 | 要求 |
|------|------|
| 服务可用性（月） | ≥ 99.9%（允许 43.8 分钟停机） |
| 计划外停机 RTO | < 5 分钟（K8s 自动重启） |
| 数据丢失 RPO | 0（同步写入 PostgreSQL，无异步缓冲） |
| 发布停机时间 | 0（RollingUpdate，maxUnavailable=0） |
| DB 连接池耗尽 | 优雅降级：返回 503，不 panic |
| Redis 不可用 | 降级到 DB 查询，不影响核心功能 |
| NATS 不可用 | 主流程不受影响（事件发布异步，失败记录到日志） |

### 5.4 可维护性 (Maintainability)

| 要求 | 标准 |
|------|------|
| 测试覆盖率 | `app/` ≥ 80%，`adapter/repo/` ≥ 60%，`adapter/handler/` ≥ 50% |
| 代码风格 | `gofmt`、`golangci-lint` 通过（CI 强制） |
| 文档 | 所有公开函数有 godoc 注释，API 有 OpenAPI 3.0 规范 |
| 迁移策略 | 数据库迁移使用编号 SQL 文件，向前兼容，运行幂等 |
| 配置管理 | 全部配置通过环境变量，启动时校验必填项，缺失 fast-fail |
| 零硬编码 | 端口、URL、TTL、阈值均提取为常量或配置项 |

### 5.5 合规 (Compliance)

| 合规项 | 要求 |
|--------|------|
| 财务数据保留 | 交易记录保留 7 年（中国《会计法》要求） |
| GDPR | 欧盟用户数据删除请求 ≤ 30 天响应 |
| 个人数据最小化 | 仅采集业务必要字段，不采集用户行为埋点 |
| 支付合规 | 不存储完整卡号；Stripe PCI DSS 合规由 Stripe 托管 |
| 账本不可变 | `wallet_transactions` 禁止 UPDATE/DELETE（财务审计要求） |

---

## 6. 边界与集成 (Boundaries & Integrations)

### 6.1 依赖服务（lurus-identity 消费）

| 服务 | 用途 | 协议 | 降级策略 |
|------|------|------|----------|
| PostgreSQL (lurus-pg-rw) | 主数据存储（identity + billing schema） | TCP/5432 | 无降级，DB 不可用时服务不可用 |
| Redis (redis.messaging.svc:6379, DB 3) | 权益缓存、分布式锁、限流计数器 | TCP/6379 | 降级到 DB 查询，记录告警 |
| NATS (nats.messaging.svc:4222) | 事件发布（IDENTITY_EVENTS 流）、消费（LLM_EVENTS 流） | NATS/4222 | 主流程不阻塞，事件发布失败记录日志 |
| Zitadel (auth.lurus.cn) | JWKS 公钥获取、JWT 验证 | HTTPS | JWKS 缓存 1 小时，刷新失败用旧缓存 |
| 易支付 Epay | 国内支付宝/微信收款 | HTTPS | 熔断器，渠道独立 |
| Stripe | 国际信用卡收款 | HTTPS | 熔断器，渠道独立 |
| Creem | 补充支付渠道 | HTTPS | 熔断器，渠道独立 |

### 6.2 消费方服务（调用 lurus-identity 的服务）

| 服务 | 调用方式 | 关键端点 | SLA 要求 |
|------|----------|----------|----------|
| lurus-api | Internal API（Bearer token） | `GET /internal/v1/accounts/:id/entitlements/:product_id` | P99 < 5ms |
| lurus-api | Internal API | `POST /internal/v1/accounts/upsert` | P99 < 100ms |
| lurus-api | Internal API | `POST /internal/v1/usage/report` | P99 < 100ms，允许异步 |
| lurus-gushen | Internal API | `GET /internal/v1/accounts/:id/entitlements/quant-trading` | P99 < 5ms |
| lurus-webmail | Internal API | `GET /internal/v1/accounts/:id/entitlements/webmail` | P99 < 5ms |
| 邮件服务 | NATS 消费 | 订阅 `identity.subscription.*` 事件 | 异步，失败重试 |

### 6.3 NATS 事件契约

**发布（lurus-identity 发出）**:

| 主题 | 触发条件 | 关键字段 |
|------|----------|----------|
| `identity.account.created` | 新账号创建 | account_id, lurus_id, referrer_id |
| `identity.account.suspended` | 账号被冻结 | account_id |
| `identity.account.deleted` | 账号被删除 | account_id |
| `identity.account.gdpr_deleted` | GDPR 删除执行 | account_id |
| `identity.subscription.activated` | 订阅激活 | account_id, product_id, plan_code, expires_at |
| `identity.subscription.grace_started` | 进入宽限期 | account_id, product_id, grace_until |
| `identity.subscription.expired` | 订阅彻底过期 | account_id, product_id |
| `identity.subscription.cancelled` | 用户取消订阅 | account_id, product_id |
| `identity.vip.upgraded` | VIP 等级提升 | account_id, old_level, new_level |
| `identity.vip.downgraded` | VIP 等级降低 | account_id, old_level, new_level |
| `identity.wallet.topped_up` | 充值成功 | account_id, amount_cny, order_no |

**消费（lurus-identity 消费）**:

| 流 / 主题 | 来源服务 | 处理逻辑 |
|-----------|----------|----------|
| `LLM_EVENTS / llm.usage.reported` | lurus-api | 触发 VIP spend_grant 重计算 |

### 6.4 API 边界说明

- `/api/v1/*`: 面向最终用户，Zitadel JWT 认证，由 Traefik Ingress 代理，前端直接调用
- `/internal/v1/*`: 服务间通信，固定 Bearer token，**禁止**通过 Ingress 对外暴露
- `/admin/v1/*`: 管理员操作，Zitadel JWT + admin role，建议仅内网访问
- `/webhook/*`: 支付回调，无认证（通过 payload 签名校验），必须对外暴露
- `/health`: 健康检查，无认证，K8s probe 使用

---

## 7. 约束与假设 (Constraints & Assumptions)

### 7.1 技术约束

- **语言运行时**: Go 1.25，`CGO_ENABLED=0`（静态链接，scratch 容器）
- **数据库**: PostgreSQL，使用 `identity` 和 `billing` 两个 schema；不使用 ORM 迁移，手动 SQL 文件管理
- **缓存**: Redis 单实例，DB 3，不支持 Redis Cluster（当前基础设施约束）
- **消息队列**: NATS JetStream，流配置 `Replicas=1`（单 NATS 节点）
- **容器基础**: scratch/alpine，`readOnlyRootFilesystem`，仅挂载 `/tmp` emptyDir
- **部署模式**: Kubernetes Deployment（非 StatefulSet），无本地持久化状态
- **前端构建**: Bun + Vite 6 + React 18 + Semi UI，编译产物嵌入 Go binary（`embed.go`）

### 7.2 业务约束

- **货币**: 主要结算货币为 CNY（人民币），USD 价格仅作展示参考，实际扣款以 CNY 为准
- **Credit 汇率**: 1 Credit = 1 CNY，固定不变（不支持汇率浮动）
- **单产品单订阅**: 每个账号每个产品同时只能有一个有效订阅，不支持合并订单
- **推荐层级**: 仅支持单层推荐关系（被推荐人 → 推荐人），不支持多级分佣
- **最小充值额**: ¥1.00 CNY

### 7.3 假设

- Zitadel 作为唯一 Identity Provider，所有用户通过 Zitadel 完成注册和 MFA；lurus-identity 不管理密码
- 下游服务（lurus-api 等）在 lurus-identity 不可用时，使用本地降级策略（不假定 100% 依赖 lurus-identity 在线）
- 生产环境 PostgreSQL 为托管服务（有备份和高可用），lurus-identity 不负责 DB 备份
- 初期 DAU < 10,000，单 Pod 承载；超过 5,000 DAU 时评估水平扩展

---

## 8. 已知风险 (Known Risks)

### 风险矩阵

| ID | 风险描述 | 概率 | 影响 | 缓解措施 |
|----|----------|------|------|----------|
| R-01 | JWT 验证 placeholder 被绕过 | 高（当前为 placeholder） | 严重（任意用户冒充他人） | P0 任务，发布前必须完成；CI 添加检测 TODO 注释的检查 |
| R-02 | 支付回调重放攻击 | 中 | 高（重复入账） | 订单状态机校验（pending→paid 单向）+ 数据库唯一约束 |
| R-03 | Webhook 签名验证 fallback 路径存在绕过风险 | 中 | 高 | 生产环境强制配置支付商 SDK，fallback 路径仅允许在 ENV=development 下激活 |
| R-04 | Redis 不可用导致缓存穿透，DB 过载 | 低 | 中 | 降级到 DB 查询 + 连接池限流；DB 负载告警 |
| R-05 | 订阅到期 Cron 单点故障 | 低 | 中（订阅不自动到期，用户白嫖） | K8s CronJob 单实例 + Prometheus gauge 存活检测 + Alertmanager 告警 |
| R-06 | 并发充值导致 VIP 等级计算竞争 | 低 | 低（VIP 计算幂等，最终一致） | VIP 重计算幂等设计，接受短暂不一致 |
| R-07 | 外部支付商接口变更（API/签名算法） | 中 | 中（某渠道失效） | 各 Provider 熔断器隔离，监控充值成功率，变更时快速响应 |
| R-08 | 大量兑换码同时兑换导致数据库锁争用 | 低 | 中（超卖） | `UPDATE ... WHERE used_count < max_uses` 乐观锁，失败重试 |
| R-09 | Admin 角色 JWT 验证缺失 | 高（当前为 placeholder） | 严重（任意用户操作 admin 接口） | P0 任务，与 R-01 同优先级 |
| R-10 | 套餐升降级差价计算错误 | 中 | 高（多扣或少扣用户费用） | 差价计算单元测试覆盖所有边界日期场景（月末、年末、闰年） |
| R-11 | NATS 流消息堆积（消费者宕机） | 低 | 低（事件延迟，不丢失） | NATS JetStream 持久化 7 天，消费者恢复后补处理 |
| R-12 | 前端 Credits 余额与后端不一致（缓存/展示问题） | 中 | 低（用户体验） | 充值/扣款后前端主动刷新，不依赖客户端缓存 |

### 关键生产阻塞项（P0 必须完成）

以下四项必须在首次生产发布前完成，否则不得上线：

1. **Zitadel JWT 完整验证**（FR-ACC-001）— 替换 `jwtAuth()` placeholder
2. **Admin 角色 JWT 验证**（FR-ADM-001）— 替换 `adminAuth()` placeholder
3. **Webhook 签名 fallback 路径限制**（R-03）— 生产环境禁用 fallback
4. **订阅到期 Cron 任务**（FR-SUB-002）— 订阅生命周期闭环

---

*本文档由产品与工程团队联合制定，版本 1.0，2026-02-27。如有需求变更，请通过 BMAD sprint-status.yaml 跟踪。*
