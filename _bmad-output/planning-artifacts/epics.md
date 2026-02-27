# lurus-identity — Epics & Stories

## Epic 总览表

| Epic ID | 名称 | 状态 | 优先级 | Story 数 | 当前 Sprint |
|---------|------|------|--------|---------|------------|
| Epic 1 | 账号身份与认证 | ✅ Done | P0 | 5 | — |
| Epic 2 | 产品目录管理 | ✅ Done | P0 | 3 | — |
| Epic 3 | 订阅生命周期 | ✅ Done | P0 | 5 | — |
| Epic 4 | 权益引擎 | ✅ Done | P0 | 3 | — |
| Epic 5 | 钱包账本 | ✅ Done | P0 | 5 | — |
| Epic 6 | 支付网关 | ✅ Done | P0 | 4 | — |
| Epic 7 | VIP 忠诚度 | ✅ Done | P1 | 3 | — |
| Epic 8 | 推荐 & 兑换码 | ✅ Done | P1 | 3 | — |
| Epic 9 | 管理后台 API | ✅ Done | P1 | 4 | — |
| Epic 10 | 前端 Portal | ✅ Done | P1 | 5 | — |
| Epic 11 | 安全加固 | 📋 Planned | P0 | 5 | Sprint 3 |
| Epic 12 | 订阅自动化 | 📋 Planned | P0 | 4 | Sprint 3 |
| Epic 13 | 可观测性 | 📋 Planned | P1 | 4 | Sprint 4 |
| Epic 14 | 财务合规 | 📋 Planned | P1 | 5 | Sprint 4 |
| Epic 15 | 平台扩展 | 📋 Planned | P2 | 5 | Sprint 5 |
| Epic 16 | 运营工具 | 📋 Planned | P1 | 4 | Sprint 4 |

**Total Stories: 67**

---

## Epic 1: 账号身份与认证

**状态**: ✅ Done | **优先级**: P0 | **负责人**: backend

### 目标

为 Lurus 平台所有产品提供统一用户身份层，每个自然人对应唯一 LurusID（格式 `LU0000001`），通过 Zitadel OIDC 作为上游 IdP，支持多 OAuth 绑定。

### Stories

#### Story 1.1: 账号 Upsert — Zitadel 用户同步

**状态**: ✅ Done
**描述**: 当 Zitadel 登录事件触发时，lurus-api 调用 `/internal/v1/accounts/upsert`，若账号不存在则创建并分配唯一 LurusID 和邀请码（`aff_code`）；若已存在则更新 display_name、avatar_url。
**验收标准（AC）**:
- AC1: 首次调用以相同 `zitadel_sub` 仅创建一条 `identity.accounts` 记录（幂等）
- AC2: LurusID 格式为 `LU` + 零填充 7 位数字，由 `GenerateLurusID(id)` 生成
- AC3: `aff_code` 唯一、不可为空，自动生成 8 位大写字母数字串
- AC4: 并发 upsert 同一 sub 不产生重复行（DB 层 unique index 保障）
- AC5: 返回完整 Account 结构体，HTTP 200
**技术要点**:
- `app.AccountService.UpsertByZitadelSub` → `adapter/repo/account.go` `ON CONFLICT DO UPDATE`
- 表: `identity.accounts`，索引: `zitadel_sub UNIQUE`、`email UNIQUE`、`aff_code UNIQUE`

#### Story 1.2: 账号查询 — by ZitadelSub / by ID / by LurusID

**状态**: ✅ Done
**描述**: 提供三种内部查询路径，供服务间调用和前端 me 端点使用。
**验收标准（AC）**:
- AC1: `GET /internal/v1/accounts/by-zitadel-sub/:sub` 返回 Account 或 404
- AC2: `GetByID`、`GetByLurusID` 在账号不存在时返回 nil（非 error），上层映射 404
- AC3: 所有查询携带 ctx，超时由调用方设置
**技术要点**:
- `accountStore` 接口定义于 `internal/app/interfaces.go`
- `adapter/repo/account.go` 实现三种查询路径

#### Story 1.3: OAuth 多绑定

**状态**: ✅ Done
**描述**: 支持同一账号绑定 github / discord / wechat / telegram / linuxdo / oidc 等多个第三方身份，关联存于 `identity.account_oauth_bindings`。
**验收标准（AC）**:
- AC1: 同一 (provider, provider_id) 组合全局唯一（UNIQUE 约束）
- AC2: 同一 account 可绑定多个不同 provider
- AC3: 账号删除时，OAuth 绑定级联删除（ON DELETE CASCADE）
- AC4: `UpsertOAuthBinding` 在绑定已存在时做 no-op 更新（幂等）
**技术要点**:
- 表: `identity.account_oauth_bindings`
- `accountStore.UpsertOAuthBinding` 使用 `ON CONFLICT (provider, provider_id) DO UPDATE`

#### Story 1.4: 账号状态管理（激活/暂停/软删除）

**状态**: ✅ Done
**描述**: Account 有三种状态: `1=active`、`2=suspended`、`3=deleted`。暂停账号无法通过 JWT 认证，软删除保留数据用于审计。
**验收标准（AC）**:
- AC1: `IsActive()` 方法仅在 `status=1` 时返回 true
- AC2: 暂停账号的 JWT 认证中间件返回 HTTP 403（而非 401）
- AC3: 软删除只更新 `status=3`，数据行保留，email/aff_code 唯一索引仍占位
- AC4: 管理后台可通过 PUT 接口切换账号状态
**技术要点**:
- 常量 `AccountStatusActive/Suspended/Deleted` 定义于 `entity/account.go`
- 认证中间件在 `jwtAuth()` 中校验 `IsActive()`

#### Story 1.5: 账号档案自助更新

**状态**: ✅ Done
**描述**: 用户通过 `PUT /api/v1/account/me` 更新 display_name 和 locale，AvatarURL 只允许系统写入（来自 Zitadel 同步）。
**验收标准（AC）**:
- AC1: 仅允许更新 `display_name`（1-64 字符）和 `locale`（BCP-47 格式）
- AC2: 空字段跳过更新，不清空已有值
- AC3: `updated_at` 自动刷新
- AC4: 非账号持有人不可修改他人资料（由 jwtAuth 中间件的 account_id 绑定保障）
**技术要点**:
- Handler: `adapter/handler/account.go::UpdateMe`
- 字段白名单校验防止 mass-assignment 攻击

---

## Epic 2: 产品目录管理

**状态**: ✅ Done | **优先级**: P0 | **负责人**: backend

### 目标

提供可由运营团队通过管理 API 动态维护的产品 & 套餐目录，特性存为 JSONB，无需代码变更即可扩展权益字段。

### Stories

#### Story 2.1: 产品 CRUD

**状态**: ✅ Done
**描述**: 管理员通过 `/admin/v1/products` 创建和更新产品条目，定义计费模型（free/quota/subscription/hybrid/one_time/seat/usage）。
**验收标准（AC）**:
- AC1: `POST /admin/v1/products` 创建产品，`product_id` 为运营自定义字符串（如 `llm-api`），不自增
- AC2: `PUT /admin/v1/products/:id` 支持更新 name、description、status、sort_order、config
- AC3: 下架产品（`status=2`）在 `GET /api/v1/products` 中不返回
- AC4: `config` 字段为 JSONB，存储产品级别的自定义配置（如限流策略）
**技术要点**:
- 表: `identity.products`，`billing_model` 常量定义于 `entity/product.go`
- `app.ProductService` 通过 `planStore` 接口操作

#### Story 2.2: 套餐 CRUD 与特性矩阵

**状态**: ✅ Done
**描述**: 每个产品可有多个套餐（free/pro/team），特性通过 JSONB `features` 字段存储，支持任意 key-value 扩展而无需 schema 变更。
**验收标准（AC）**:
- AC1: `POST /admin/v1/products/:id/plans` 创建套餐，`(product_id, code)` 全局唯一
- AC2: `features` 为 JSONB，支持 string/integer/boolean/decimal 值类型
- AC3: 每产品只允许一个 `is_default=true` 套餐（DB 层可触发器或应用层强制）
- AC4: `billing_cycle` 合法值: forever/weekly/monthly/quarterly/yearly/one_time
- AC5: `price_cny` 和 `price_usd` 均可为 0（免费套餐）
**技术要点**:
- 表: `identity.product_plans`，UNIQUE `(product_id, code)`
- `BillingCycle*` 常量定义于 `entity/product.go`

#### Story 2.3: 产品目录公开查询

**状态**: ✅ Done
**描述**: 前端通过公开 JWT 认证路由 `GET /api/v1/products` 和 `GET /api/v1/products/:id/plans` 获取产品及价格列表，用于展示定价页面。
**验收标准（AC）**:
- AC1: 仅返回 `status=1` 的产品，按 `sort_order ASC` 排序
- AC2: 套餐列表包含 `features`、`price_cny`、`price_usd`、`billing_cycle`
- AC3: 响应时间 < 100ms（由 Redis 缓存产品目录，TTL 5min）
**技术要点**:
- Handler: `adapter/handler/product.go::ListProducts`、`ListPlans`
- 产品目录变更后需 invalidate Redis 缓存

---

## Epic 3: 订阅生命周期

**状态**: ✅ Done | **优先级**: P0 | **负责人**: backend

### 目标

管理用户与产品之间的订阅关系，实现从购买、有效期、宽限期到到期/取消的完整状态机，保证同一账号同一产品同时只有一个有效订阅。

### Stories

#### Story 3.1: 订阅激活（Activate）

**状态**: ✅ Done
**描述**: 支付成功后通过 webhook 触发 `SubscriptionService.Activate`，自动关闭同产品旧订阅，创建新订阅，并同步权益快照至 Redis。
**验收标准（AC）**:
- AC1: 激活前若存在 active/grace/trial 订阅，旧订阅状态变更为 `expired`
- AC2: 订阅 `expires_at` 按 `billing_cycle` 正确计算（月份边界 clamp，如 1月31日+1月=2月28日）
- AC3: `forever` 和 `one_time` 周期的 `expires_at` 为 NULL
- AC4: 激活后立即调用 `EntitlementService.SyncFromSubscription`，刷新权益快照
- AC5: 同一 (account_id, product_id) 不存在两条 active 订阅（partial unique index 保障）
**技术要点**:
- `app/subscription_service.go::Activate`
- `calculateExpiry` + `addMonthsClamped` 处理月份溢出
- DB: `idx_subs_active_unique` partial unique index WHERE status IN ('active','grace','trial')

#### Story 3.2: 订阅到期进入宽限期（Expire → Grace）

**状态**: ✅ Done
**描述**: 订阅到期时不立即降级，进入 7 天宽限期（`status=grace`），宽限期内权益不变，鼓励续费。
**验收标准（AC）**:
- AC1: `Expire` 调用后 `status=grace`，`grace_until=now()+7d`
- AC2: 宽限期内 `IsLive()` 返回 true，权益正常
- AC3: `grace_until` 精确到秒，不受 DST 影响（UTC 存储）
**技术要点**:
- 常量 `gracePeriodDays=7` 定义于 `app/subscription_service.go`
- `IsLive()` 判断: status in (active, grace, trial)

#### Story 3.3: 宽限期结束强制降级（EndGrace）

**状态**: ✅ Done
**描述**: 宽限期到期后 `EndGrace` 将订阅标记为 `expired`，并调用 `EntitlementService.ResetToFree` 将权益重置为免费。
**验收标准（AC）**:
- AC1: `EndGrace` 后 `status=expired`，`entitlements` 仅保留 `plan_code=free`
- AC2: Redis 缓存在 `ResetToFree` 中立即 invalidate
- AC3: `IsLive()` 返回 false，下游产品下次请求权益时得到免费套餐
**技术要点**:
- `app/subscription_service.go::EndGrace` → `entitlements.ResetToFree`
- `entitlement_service.go::ResetToFree`: DELETE 旧记录 + INSERT plan_code=free

#### Story 3.4: 订阅取消（Cancel）

**状态**: ✅ Done
**描述**: 用户或管理员可取消订阅，取消后关闭 auto_renew，当前周期权益保留至 `expires_at`。
**验收标准（AC）**:
- AC1: `POST /api/v1/subscriptions/:product_id/cancel` 将 `status=cancelled`，`auto_renew=false`
- AC2: 取消后订阅仍在有效期内，`IsLive()` 仍为 true
- AC3: 非本账号订阅无法取消（accountID 校验）
- AC4: 无活跃订阅时返回 400 with 明确错误信息
**技术要点**:
- Handler: `adapter/handler/subscription.go::CancelSubscription`
- `app.SubscriptionService.Cancel`

#### Story 3.5: 订阅结账流程（Checkout）

**状态**: ✅ Done
**描述**: 用户通过 `POST /api/v1/subscriptions/checkout` 选择产品 + 套餐 + 支付方式，系统创建 PaymentOrder 并返回支付 URL，待 webhook 回调后激活订阅。
**验收标准（AC）**:
- AC1: 请求体包含 `product_id`、`plan_id`、`payment_method`（必填）及 `return_url`（可选）
- AC2: 创建 `order_type=subscription` 的 PaymentOrder，`status=pending`
- AC3: 返回 `order_no` 和 `pay_url`，前端跳转支付
- AC4: 同一 order_no 重复 checkout 返回幂等结果
- AC5: 钱包余额不足时走外部支付，钱包余额充足时可选择直接扣减
**技术要点**:
- Handler: `adapter/handler/subscription.go::Checkout`
- `order_no` 格式: `LO` + yyyyMMdd + 8位 UUID hex

---

## Epic 4: 权益引擎

**状态**: ✅ Done | **优先级**: P0 | **负责人**: backend

### 目标

维护以 `identity.account_entitlements` 为单一事实来源的权益快照，通过 Redis（DB=3）缓存实现低延迟读取，供所有产品服务使用。

### Stories

#### Story 4.1: 权益快照同步（SyncFromSubscription）

**状态**: ✅ Done
**描述**: 订阅激活/续费时，将 `product_plans.features` JSONB 展开为行级 `account_entitlements` 记录（upsert），并 invalidate Redis 缓存。
**验收标准（AC）**:
- AC1: `features` 中每个 key-value 对产生一条 entitlement 行
- AC2: 始终写入 `plan_code` 键（值为套餐 code）
- AC3: `value_type` 自动推断: boolean/integer/decimal/string
- AC4: `source=subscription`、`source_ref=sub_id`、`expires_at=sub.expires_at`
- AC5: UPSERT（ON CONFLICT (account_id, product_id, key) DO UPDATE）保证幂等
**技术要点**:
- `app/entitlement_service.go::SyncFromSubscription`
- 表: `identity.account_entitlements`，UNIQUE (account_id, product_id, key)

#### Story 4.2: 权益缓存 Get/Set/Invalidate（Redis）

**状态**: ✅ Done
**描述**: `EntitlementService.Get` 优先从 Redis 读取，miss 时从 DB 加载并回填，TTL=5min。订阅变更时立即 invalidate。
**验收标准（AC）**:
- AC1: Redis key 格式: `ent:{account_id}:{product_id}`，TTL=5min（configurable）
- AC2: Cache miss 时从 `identity.account_entitlements` 全量加载并 SET
- AC3: 至少含 `plan_code=free` 兜底，不返回 nil map
- AC4: Redis 不可用时 fallback 到 DB，不返回错误给调用方
**技术要点**:
- `entitlementCache` 接口定义于 `app/interfaces.go`
- Redis DB=3（`IDENTITY_REDIS_DB` env，默认 3）

#### Story 4.3: 内部权益查询 API

**状态**: ✅ Done
**描述**: 下游产品服务（lurus-api、lurus-gushen 等）通过 `/internal/v1/accounts/:id/entitlements/:product_id` 获取当前权益，以 Internal API Key 鉴权。
**验收标准（AC）**:
- AC1: 响应为 `map[string]string`（flat 结构，无嵌套）
- AC2: Internal Key 通过 `Authorization: Bearer <INTERNAL_API_KEY>` 传递
- AC3: 账号或产品不存在时返回 `{"plan_code": "free"}`（降级，不 404）
- AC4: P99 响应时间 < 10ms（缓存命中路径）
**技术要点**:
- Handler: `adapter/handler/internal_api.go::GetEntitlements`
- 鉴权: `internalKeyAuth` 中间件，constant-time 字符串比较

---

## Epic 5: 钱包账本

**状态**: ✅ Done | **优先级**: P0 | **负责人**: backend

### 目标

实现以 `1 Credit = 1 CNY` 为单位的统一预付费钱包，维护不可变追加式账本，支持充值、消费、退款、奖励多种交易类型。

### Stories

#### Story 5.1: 钱包创建与余额查询

**状态**: ✅ Done
**描述**: 账号首次访问钱包时自动创建，维护 `balance`（可用）、`frozen`（冻结中）、`lifetime_topup`（累计充值）、`lifetime_spend`（累计消费）。
**验收标准（AC）**:
- AC1: `GetOrCreate` 在并发场景下不创建重复行（`account_id UNIQUE NOT NULL`）
- AC2: `GET /api/v1/wallet` 返回完整 Wallet 结构体（含 4 个字段）
- AC3: `balance` 精度 DECIMAL(14,4)，支持最大 10 亿余额
- AC4: 余额不可为负（Debit 前校验，不足时返回明确错误）
**技术要点**:
- `walletStore.GetOrCreate` 使用 `INSERT ... ON CONFLICT DO NOTHING`
- 表: `billing.wallets`

#### Story 5.2: 追加式账本（Credit / Debit）

**状态**: ✅ Done
**描述**: 所有余额变动产生不可变 `wallet_transactions` 记录，`balance_after` 记录变动后余额，Account 上的 `balance` 原子更新。
**验收标准（AC）**:
- AC1: `wallet_transactions` 只做 INSERT，严禁 UPDATE/DELETE（append-only 账本）
- AC2: Credit/Debit 在同一 DB 事务中完成（更新 wallets.balance + 插入 transaction）
- AC3: Debit 前校验 `balance >= amount`，不足时回滚并返回 `insufficient balance`
- AC4: `type` 字段合法值: topup/subscription/product_purchase/refund/bonus/referral_reward/redemption/checkin_reward/admin_credit/admin_debit
- AC5: `reference_type` + `reference_id` 提供可追溯的来源引用
**技术要点**:
- `adapter/repo/wallet.go` 使用 DB 事务保证原子性
- `balance_after = current_balance ± amount`，写入前读取余额（SELECT FOR UPDATE）

#### Story 5.3: 钱包充值 Topup（触发支付）

**状态**: ✅ Done
**描述**: 用户通过 `POST /api/v1/wallet/topup` 发起充值，系统创建 PaymentOrder 并返回支付 URL，webhook 回调后自动 Credit 钱包。
**验收标准（AC）**:
- AC1: `amount_cny` 必须 > 0，`payment_method` 必须为已启用的支付方式之一
- AC2: 创建 `order_type=topup` 的 PaymentOrder，`status=pending`
- AC3: 返回 `order_no` + `pay_url`
- AC4: 支付成功后 `MarkOrderPaid` 自动 Credit wallet，触发 VIP 重算
- AC5: 重复 webhook 回调幂等处理（已付款订单直接返回，不重复入账）
**技术要点**:
- `app/wallet_service.go::CreateTopup` + `MarkOrderPaid`
- 幂等: `if order.Status == OrderStatusPaid { return order, nil }`

#### Story 5.4: 交易历史分页查询

**状态**: ✅ Done
**描述**: 用户通过 `GET /api/v1/wallet/transactions` 分页查看所有账本记录，按时间倒序。
**验收标准（AC）**:
- AC1: 支持 `page`（默认 1）和 `page_size`（默认 20，最大 100）参数
- AC2: 返回 `data`（记录列表）和 `total`（总条数）
- AC3: 仅返回当前认证账号的记录，不可查他人
- AC4: 交易记录包含 type、amount、balance_after、description、created_at
**技术要点**:
- `adapter/repo/wallet.go::ListTransactions`，按 `created_at DESC`
- 索引: `idx_wtx_created ON billing.wallet_transactions(created_at DESC)`

#### Story 5.5: 支付订单查询

**状态**: ✅ Done
**描述**: 用户可查看自己的支付订单列表及单笔订单详情，确认支付状态（pending/paid/failed/cancelled/refunded）。
**验收标准（AC）**:
- AC1: `GET /api/v1/wallet/orders` 返回分页订单列表
- AC2: `GET /api/v1/wallet/orders/:order_no` 返回单笔订单详情
- AC3: `order_no` 为他人订单时返回 404（防枚举，不暴露他人订单号存在性）
- AC4: 订单详情包含 order_no、order_type、amount_cny、status、paid_at、payment_method
**技术要点**:
- `app/wallet_service.go::GetOrderByNo` 包含 `accountID == order.AccountID` 校验

---

## Epic 6: 支付网关

**状态**: ✅ Done | **优先级**: P0 | **负责人**: backend

### 目标

集成易支付（支付宝/微信）、Stripe、Creem 三个支付渠道，通过统一 `Provider` 接口抽象，实现签名验证安全的 Webhook 回调处理。

### Stories

#### Story 6.1: 易支付（Epay）集成

**状态**: ✅ Done
**描述**: 通过 `go-epay` SDK 集成易支付网关，支持支付宝（`epay_alipay`）和微信支付（`epay_wxpay`）两种方式，回调为 GET 请求。
**验收标准（AC）**:
- AC1: `GET /webhook/epay` 验证签名（`p.client.Verify(flat)`），签名失败返回 `fail`
- AC2: `trade_status=TRADE_SUCCESS` 时触发 `processOrderPaid`
- AC3: `EpayProvider` 在 `partnerID` 或 `key` 为空时优雅禁用（返回 nil，不报错）
- AC4: 支付 URL 为 GET 重定向 URL（`baseURL?params`）
- AC5: `EPAY_PARTNER_ID`、`EPAY_KEY`、`EPAY_GATEWAY_URL`、`EPAY_NOTIFY_URL` 从环境变量读取
**技术要点**:
- `adapter/payment/epay.go`，`EpayProvider.CreateCheckout` + `VerifyCallback`
- `url.Values` → `map[string]string` 展平后传给 SDK Verify

#### Story 6.2: Stripe 集成

**状态**: ✅ Done
**描述**: 通过 Stripe Checkout Session 处理国际信用卡支付，Webhook 使用 `Stripe-Signature` 头进行 HMAC 验证。
**验收标准（AC）**:
- AC1: `POST /webhook/stripe` 验证 `Stripe-Signature` 头
- AC2: 处理 `checkout.session.completed` 事件，从 `client_reference_id` 取 `order_no`
- AC3: `StripeProvider` 在 `STRIPE_SECRET_KEY` 或 `STRIPE_WEBHOOK_SECRET` 为空时优雅禁用
- AC4: Webhook body 读取限制 1MB（防 DoS）
**技术要点**:
- `adapter/payment/stripe.go`，`io.LimitReader(c.Request.Body, 1<<20)`

#### Story 6.3: Creem 集成

**状态**: ✅ Done
**描述**: 集成 Creem 支付，Webhook 使用 `X-Creem-Signature` 头验证 HMAC-SHA256，处理 `payment.success` 事件。
**验收标准（AC）**:
- AC1: `POST /webhook/creem` 验证 `X-Creem-Signature`，失败返回 HTTP 401
- AC2: 处理 `event_type=payment.success`，从 `order_no` 字段取订单号
- AC3: `CreemProvider` 在 `CREEM_API_KEY` 为空时优雅禁用
**技术要点**:
- `adapter/payment/creem.go`，`creem.VerifyWebhook(body, sig)`

#### Story 6.4: 支付方式动态枚举

**状态**: ✅ Done
**描述**: `GET /api/v1/wallet/topup/info` 根据运行时已启用的 Provider 动态返回可用支付方式列表，未配置的 Provider 不出现在列表中。
**验收标准（AC）**:
- AC1: 未配置 Epay 时，响应中不含 `epay_alipay` / `epay_wxpay`
- AC2: 每种支付方式返回 `id`、`name`（人类可读）、`provider`
- AC3: 至少一个 Provider 启用，否则充值流程不可达
**技术要点**:
- Handler: `adapter/handler/wallet.go::TopupInfo`，nil 检查各 provider

---

## Epic 7: VIP 忠诚度

**状态**: ✅ Done | **优先级**: P1 | **负责人**: backend

### 目标

基于累计消费金额和年度订阅档次实现 VIP 等级体系（Level 0-N），等级影响全局折扣和增值权益，NATS 事件驱动自动重算。

### Stories

#### Story 7.1: VIP 等级计算（spend_grant + yearly_sub_grant）

**状态**: ✅ Done
**描述**: VIP Level = MAX(yearly_sub_grant, spend_grant)。`spend_grant` 基于 `wallet.lifetime_topup` 对照 `vip_level_configs.min_spend_cny` 阈值计算；`yearly_sub_grant` 在年订阅激活时显式设置。
**验收标准（AC）**:
- AC1: `RecalculateFromWallet` 读取 `lifetime_topup`，按 `vip_level_configs` 升序阈值确定 `spend_grant`
- AC2: `Level = maxInt16(yearly_sub_grant, spend_grant)`（取较高值）
- AC3: 充值后自动触发重算，不阻塞充值主流程（best-effort）
- AC4: `vip_level_configs` 支持运营动态配置阈值（JSONB `perks_json`）
**技术要点**:
- `app/vip_service.go::RecalculateFromWallet`
- 表: `identity.account_vip` + `identity.vip_level_configs`

#### Story 7.2: 年订阅 VIP 授予（GrantYearlySub）

**状态**: ✅ Done
**描述**: 年度订阅激活时，根据套餐 code 匹配 `vip_level_configs.yearly_sub_min_plan`，授予对应 VIP 等级。
**验收标准（AC）**:
- AC1: `GrantYearlySub(accountID, grantLevel)` 更新 `yearly_sub_grant` 并重算 Level
- AC2: 年订阅到期后 `yearly_sub_grant` 不自动清零（由管理员或专项任务处理）
- AC3: VIP 等级变更时更新 `level_name`（查 vip_level_configs 表）
**技术要点**:
- `app/vip_service.go::GrantYearlySub`
- NATS 消费者接收 LLM_USAGE 事件后调用 `RecalculateFromWallet`

#### Story 7.3: NATS 驱动 VIP 自动重算

**状态**: ✅ Done
**描述**: NATS Consumer 订阅 `identity.llm_usage_reported`（由 lurus-api 发布），异步触发 `RecalculateFromWallet`，实现 VIP 积分实时更新。
**验收标准（AC）**:
- AC1: Consumer 使用 Durable queue group `lurus-identity-llm-usage`，保证 Pod 重启后不丢消息
- AC2: `MaxDeliver=5`，处理失败时 NAK 触发重试
- AC3: `AccountID <= 0` 的无效消息静默丢弃（ACK，不重试）
- AC4: NATS 连接断开时 Consumer goroutine 优雅退出（context cancel）
**技术要点**:
- `adapter/nats/consumer.go`，JetStream QueueSubscribe + AckExplicit

---

## Epic 8: 推荐 & 兑换码

**状态**: ✅ Done | **优先级**: P1 | **负责人**: backend

### 目标

通过邀请码（aff_code）推荐奖励链和兑换码（redemption_code）两套激励机制，驱动用户增长和留存。

### Stories

#### Story 8.1: 推荐奖励链

**状态**: ✅ Done
**描述**: 用户注册时可携带 `aff_code` 参数，系统将推荐人绑定为 `referrer_id`，并在被推荐人完成注册/首充/首订阅时，向推荐人发放 Credits 奖励。
**验收标准（AC）**:
- AC1: 注册时奖励推荐人 5 Credits（`RewardSignup=5.0`）
- AC2: 首次充值时奖励推荐人 10 Credits（`RewardFirstTopup=10.0`）
- AC3: 首次订阅时奖励推荐人 20 Credits（`RewardFirstSubscription=20.0`）
- AC4: 奖励写入 `billing.referral_reward_events` 并 Credit 推荐人钱包
- AC5: 推荐人不存在时静默忽略，不阻塞被推荐人注册流程
**技术要点**:
- `app/referral_service.go`，事件类型: signup/first_topup/first_subscription/renewal
- 表: `billing.referral_reward_events`，`identity.accounts.referrer_id`

#### Story 8.2: 兑换码核销（Redeem）

**状态**: ✅ Done
**描述**: 用户通过 `POST /api/v1/wallet/redeem` 输入兑换码，系统验证有效期和使用次数后发放奖励（当前支持 `credits` 类型）。
**验收标准（AC）**:
- AC1: 兑换码大小写不敏感（输入自动 TRIM + UPPER）
- AC2: `expires_at` 过期或 `used_count >= max_uses` 时返回明确错误
- AC3: `used_count` 在发放后原子 +1（防并发重复核销）
- AC4: `reward_type=credits` 时直接 Credit 钱包，支持 `product_id` 限定用途（NULL=全局）
- AC5: 当前不支持的 `reward_type` 返回 400 with 说明，不静默忽略
**技术要点**:
- `app/wallet_service.go::Redeem`
- 防并发: DB UPDATE redemption_codes SET used_count=used_count+1 WHERE used_count < max_uses

#### Story 8.3: 每日签到奖励

**状态**: ✅ Done
**描述**: 用户每日首次登录可领取签到奖励，Credits 入账，交易类型 `TxTypeCheckinReward`。
**验收标准（AC）**:
- AC1: 同一自然日（UTC+8）只能领取一次，重复请求返回 409
- AC2: 奖励金额从配置读取（默认 1 Credit/天），不硬编码
- AC3: 签到记录写入 `wallet_transactions`，`reference_type=checkin`，`reference_id=yyyy-MM-dd`
- AC4: 跨天签到不补发（非连续签到不积累）
**技术要点**:
- 通过 Redis SET NX `checkin:{account_id}:{date}` 实现日去重
- 常量 `TxTypeCheckinReward = "checkin_reward"` 定义于 `entity/wallet.go`

---

## Epic 9: 管理后台 API

**状态**: ✅ Done | **优先级**: P1 | **负责人**: backend

### 目标

提供运营和客服团队使用的管理 API（`/admin/v1/*`），支持账号管理、钱包调账、产品运维等操作，Admin JWT 鉴权。

### Stories

#### Story 9.1: 账号管理（列表/详情/权益授予）

**状态**: ✅ Done
**描述**: 管理员可分页搜索账号列表，查看任意账号详情（含 VIP 和订阅），并手动授予权益。
**验收标准（AC）**:
- AC1: `GET /admin/v1/accounts?q=keyword&page=1&page_size=20` 关键字搜索（email/display_name/lurus_id）
- AC2: `GET /admin/v1/accounts/:id` 返回 account + vip + subscriptions 聚合响应
- AC3: `POST /admin/v1/accounts/:id/grant` 手动授予 `admin_grant` 来源权益
- AC4: `page_size` 上限 100，超出时自动 clamp
**技术要点**:
- Handler: `adapter/handler/account.go::AdminListAccounts`、`AdminGetAccount`、`AdminGrantEntitlement`

#### Story 9.2: 钱包人工调账

**状态**: ✅ Done
**描述**: 管理员通过 `POST /admin/v1/accounts/:id/wallet/adjust` 对用户钱包做 Credit（正数）或 Debit（负数）调整，附带必填说明。
**验收标准（AC）**:
- AC1: `amount > 0` 触发 Credit（type=admin_credit），`amount < 0` 触发 Debit（type=admin_debit）
- AC2: `description` 必填，用于审计追踪
- AC3: 调账后返回最新 Wallet 余额
- AC4: amount=0 时返回 400（无意义操作）
- AC5: 所有调账记录写入 `wallet_transactions`，reference_type=admin
**技术要点**:
- Handler: `adapter/handler/wallet.go::AdminAdjustWallet`

#### Story 9.3: 产品与套餐运维

**状态**: ✅ Done
**描述**: 管理员可创建/更新产品和套餐，通过 status 字段控制上下架，无需代码变更即可扩展权益字段。
**验收标准（AC）**:
- AC1: `POST /admin/v1/products` 创建产品（product_id 由运营指定，非自增）
- AC2: `PUT /admin/v1/products/:id` 更新产品属性
- AC3: `POST /admin/v1/products/:id/plans` 创建套餐，features 为 JSONB 自由结构
- AC4: `PUT /admin/v1/plans/:id` 更新套餐价格/特性，不触发已有订阅权益变更
**技术要点**:
- Handler: `adapter/handler/product.go`
- 套餐 features 变更仅影响新订阅，存量账号权益由 SyncFromSubscription 在续费时更新

#### Story 9.4: VIP 手动设置

**状态**: ✅ Done
**描述**: 管理员可为特定账号直接设置 VIP 等级（用于客服补偿、内测邀请等场景）。
**验收标准（AC）**:
- AC1: `POST /admin/v1/accounts/:id/vip` 设置指定 VIP level
- AC2: 设置后 level_name 自动更新为对应 vip_level_configs.name
- AC3: 管理员设置不覆盖 spend_grant/yearly_sub_grant 逻辑（AdminSet 直接设 level 字段）
- AC4: 操作写入审计日志（who/when/from/to）
**技术要点**:
- `app/vip_service.go::AdminSet`

---

## Epic 10: 前端 Portal

**状态**: ✅ Done | **优先级**: P1 | **负责人**: frontend

### 目标

基于 React + Semi UI（QQ 钻石权益风格）的用户自服务 Portal，覆盖钱包管理、充值、订阅查看、兑换码等核心功能。

### Stories

#### Story 10.1: 钱包概览页（Wallet）

**状态**: ✅ Done
**描述**: 展示用户 Credits 余额、VIP 等级徽章（LurusBadge）、近期交易记录（分页）。
**验收标准（AC）**:
- AC1: 余额实时从 `GET /api/v1/wallet` 读取，格式化显示（2 位小数）
- AC2: VIP 等级以 QQ 钻石风格徽章展示（level 0-5 对应不同样式）
- AC3: 交易记录表格含 type、amount（正/负着色）、description、时间，分页 20 条/页
- AC4: 加载中状态显示骨架屏，错误状态友好提示
**技术要点**:
- `web/src/pages/Wallet/`，`web/src/components/LurusBadge/`
- Semi UI Table + Pagination 组件

#### Story 10.2: 充值页（Topup）

**状态**: ✅ Done
**描述**: 用户选择充值金额（预设选项 + 自定义输入）和支付方式，跳转支付，支付结果页轮询订单状态。
**验收标准（AC）**:
- AC1: 动态从 `GET /api/v1/wallet/topup/info` 获取可用支付方式，未配置的不显示
- AC2: 金额输入限制: > 0，最大 10000 CNY，精度 2 位小数
- AC3: 点击支付后打开新标签跳转 `pay_url`，当前页轮询 `GET /api/v1/wallet/orders/:order_no`
- AC4: 订单 `status=paid` 时刷新余额并提示成功；超时（5min）提示联系客服
**技术要点**:
- `web/src/pages/Topup/`，轮询间隔 2s，最多 150 次（5min）

#### Story 10.3: 订阅管理页（Subscriptions）

**状态**: ✅ Done
**描述**: 展示用户所有产品的订阅状态（active/grace/cancelled/expired），支持取消操作和结账购买。
**验收标准（AC）**:
- AC1: 从 `GET /api/v1/subscriptions` 获取所有订阅，按产品分组展示
- AC2: active/grace 状态显示到期日倒计时，grace 状态红色警告
- AC3: 取消订阅弹窗二次确认，调用 `POST /api/v1/subscriptions/:product_id/cancel`
- AC4: 无订阅时展示产品目录引导购买
**技术要点**:
- `web/src/pages/Subscriptions/`

#### Story 10.4: 兑换码页（Redeem）

**状态**: ✅ Done
**描述**: 用户输入兑换码，系统即时兑换并展示到账 Credits 金额和余额变化。
**验收标准（AC）**:
- AC1: 输入框自动 TRIM，提交时转 UPPER（与后端一致）
- AC2: 兑换成功展示 Credits 到账金额和新余额，失败展示具体原因
- AC3: 连续兑换支持（成功后清空输入框，余额实时更新）
**技术要点**:
- `web/src/pages/Redeem/`

#### Story 10.5: 管理后台前端（Admin）

**状态**: ✅ Done
**描述**: 基础管理界面，支持账号搜索、钱包调账、兑换码批量创建等运营操作。
**验收标准（AC）**:
- AC1: 账号搜索表格，点击查看详情（余额、VIP、订阅）
- AC2: 调账表单，正数=充值，负数=扣减，description 必填
- AC3: 仅 admin role 账号可访问（前端路由守卫 + 后端双重校验）
**技术要点**:
- `web/src/pages/Admin/`

---

## Epic 11: 安全加固

**状态**: 📋 Planned | **优先级**: P0 | **负责人**: backend

### 目标

将占位性认证中间件替换为生产级 Zitadel JWKS JWT 完整验证，加入限流防护和支付 Webhook 幂等性保障，消除所有安全漏洞。

### Stories

#### Story 11.1: Zitadel JWKS JWT 完整验证

**状态**: 📋 Planned
**描述**: 替换 `router.go::jwtAuth()` 中的 X-Account-ID header 占位实现，改为从 Zitadel JWKS endpoint 动态获取公钥、完整验证 JWT 签名及 claims，并将 `zitadel_sub` 解析后查询本地 accounts 表取得 account_id。
**验收标准（AC）**:
- AC1: JWKS 公钥从 `https://auth.lurus.cn/.well-known/jwks.json` 获取，本地缓存 1h，自动轮换
- AC2: 验证 JWT 签名算法（仅接受 RS256）、`iss`（必须为 Zitadel issuer）、`exp`、`iat`
- AC3: 从 `sub` claim 查询 `identity.accounts.zitadel_sub`，得到 `account_id` 写入 gin.Context
- AC4: 账号不存在时自动 upsert（首次登录场景），不返回 401
- AC5: JWKS fetch 失败时使用 stale 缓存，缓存彻底失效则返回 503 而非跳过验证
- AC6: 添加 token replay 防护（检查 `jti` claim）
**技术要点**:
- 引入 `github.com/lestrrat-go/jwx/v2` 或等效库
- JWKS URL 从 `ZITADEL_ISSUER` env 拼接

#### Story 11.2: Admin 角色 JWT Claims 验证

**状态**: 📋 Planned
**描述**: 替换 `adminAuth()` 占位实现，验证 JWT 中包含 `role=admin` claim（Zitadel 自定义 claim），无 admin 角色的请求返回 403。
**验收标准（AC）**:
- AC1: Admin role claim 路径从 `ADMIN_CLAIM_PATH` env 配置（如 `urn:zitadel:iam:org:role`）
- AC2: 缺少 admin role 返回 HTTP 403（Forbidden），区别于 401（Unauthenticated）
- AC3: Admin 操作记录操作者 account_id 写入审计日志
- AC4: Admin JWT 验证复用 Story 11.1 的 JWKS 验证流程（不重复实现）
**技术要点**:
- `router.go::adminAuth` 在 jwtAuth 基础上追加 role 检查

#### Story 11.3: 限流中间件

**状态**: 📋 Planned
**描述**: 实现三层限流: 全局（保护服务整体）、per-IP（防暴力扫描）、per-user（防滥用），使用 Redis 令牌桶算法。
**验收标准（AC）**:
- AC1: 全局限流: 1000 req/s（可配置），超出返回 HTTP 429 with `Retry-After` header
- AC2: per-IP 限流: 60 req/min（可配置），针对未认证路由（`/api/v1/`、`/webhook/`）
- AC3: per-user 限流: 100 req/min 对认证用户，防止单账号 API 滥用
- AC4: Webhook 端点单独限流: per-source-IP 20 req/min（防 Webhook 重放轰炸）
- AC5: 限流触发时记录 structured log（who/endpoint/rate）
- AC6: 限流阈值通过环境变量配置，不硬编码
**技术要点**:
- 使用 Redis 滑动窗口（INCR + EXPIRE）或令牌桶
- Gin 中间件注入到对应 router group

#### Story 11.4: 支付 Webhook 幂等性保障

**状态**: 📋 Planned
**描述**: 当前 `MarkOrderPaid` 虽有状态检查，但非原子操作。需用 Redis 分布式锁 + DB 唯一约束双重保障，防止支付渠道重试导致重复入账。
**验收标准（AC）**:
- AC1: 处理 Webhook 前获取 Redis 分布式锁 `lock:order:{order_no}`，TTL=30s
- AC2: `MarkOrderPaid` 使用 DB 事务 + `WHERE status='pending'` 条件更新，返回影响行数
- AC3: 影响行数=0 表示已处理，直接返回成功（幂等）
- AC4: 锁获取失败（另一 Pod 正在处理）返回 HTTP 200（避免支付方重试）而非 500
- AC5: 处理成功/幂等/失败均写入结构化审计日志
**技术要点**:
- Redis SETNX `lock:order:{order_no}` with TTL
- DB UPDATE ... WHERE status='pending' RETURNING id

#### Story 11.5: CORS 精确配置

**状态**: 📋 Planned
**描述**: 替换当前 `cors.Default()`（允许所有来源），改为精确配置允许的 Origins、Headers 和 Methods。
**验收标准（AC）**:
- AC1: `AllowOrigins` 从 `CORS_ALLOWED_ORIGINS` env 读取（逗号分隔），支持通配符子域
- AC2: 预检请求缓存 12h（`MaxAge=43200`）
- AC3: 仅允许必要的 Headers: `Authorization`, `Content-Type`, `X-Request-ID`
- AC4: 非允许来源的请求返回 CORS 错误，不暴露 API 数据
**技术要点**:
- `gin-contrib/cors` 精确配置
- `/webhook/*` 路由不需要 CORS（无浏览器直接调用）

---

## Epic 12: 订阅自动化

**状态**: 📋 Planned | **优先级**: P0 | **负责人**: backend

### 目标

实现订阅生命周期的自动化管理，包含到期提醒邮件、自动续费（钱包余额充足时）、宽限期到期强制降级和定时状态同步 Cron。

### Stories

#### Story 12.1: 到期提醒邮件（-7/-3/-1 天）

**状态**: 📋 Planned
**描述**: Cron 任务每日运行，查询 7 天、3 天、1 天后到期的 active 订阅，通过邮件服务发送提醒，每个时间节点只发一次。
**验收标准（AC）**:
- AC1: 查询 `expires_at BETWEEN now() AND now()+Nd` 的 active 订阅（不含 grace）
- AC2: 每个 (subscription_id, reminder_type) 组合只发送一次（Redis SET NX `reminder:{sub_id}:{days}` 记录）
- AC3: 邮件内容包含: 产品名、到期日、续费入口 URL
- AC4: 邮件发送失败记录错误日志，不阻塞其他提醒任务
- AC5: `REMINDER_EMAIL_ENABLED` env 控制开关
**技术要点**:
- Cron 实现于 `internal/lifecycle/` 使用 `github.com/robfig/cron/v3`
- 邮件通过 NATS 事件发布给邮件服务（解耦）

#### Story 12.2: 自动续费（钱包余额扣减）

**状态**: 📋 Planned
**描述**: 订阅到期当日，若 `auto_renew=true` 且钱包余额充足，自动扣减并激活新一周期订阅，跳过外部支付流程。
**验收标准（AC）**:
- AC1: 仅对 `auto_renew=true` 的订阅执行
- AC2: 从 `product_plans` 取当前 `price_cny`，调用 `WalletService.Debit`
- AC3: Debit 成功后立即 `SubscriptionService.Activate`（新周期）
- AC4: 余额不足时跳过自动续费，转入宽限期，并发送余额不足提醒邮件
- AC5: 每个订阅续费操作写入审计日志（success/fail/skipped）
- AC6: 自动续费事务失败需回滚 Debit，不产生"扣款未续费"的异常状态
**技术要点**:
- DB 事务包含 Debit + Activate，失败全部回滚

#### Story 12.3: 宽限期到期自动降级 Cron

**状态**: 📋 Planned
**描述**: Cron 任务每小时运行，查询 `grace_until < now()` 的 grace 状态订阅，批量调用 `EndGrace` 执行降级。
**验收标准（AC）**:
- AC1: 查询 `status='grace' AND grace_until < NOW()`
- AC2: 每条 grace 订阅执行 `EndGrace`（expired + ResetToFree + Redis invalidate）
- AC3: 批处理不超过 100 条/批，避免长事务（分批 cursor-based 查询）
- AC4: 执行结果写入日志（processed count / failed count）
- AC5: 单条失败不影响批次内其他记录的处理
**技术要点**:
- `internal/lifecycle/` Cron 任务，每小时 `@hourly`

#### Story 12.4: 订阅状态定时同步（每小时）

**状态**: 📋 Planned
**描述**: 每小时运行定时任务，扫描到期（`expires_at < now()` 且 `status=active`）订阅并转入宽限期，确保因 Pod 重启或 Webhook 丢失导致的状态滞后得到修复。
**验收标准（AC）**:
- AC1: 查询 `status='active' AND expires_at < NOW() AND expires_at IS NOT NULL`
- AC2: 对每条过期订阅调用 `SubscriptionService.Expire`（active → grace）
- AC3: Cron 任务使用分布式锁（Redis），同一时刻只有一个 Pod 执行
- AC4: 执行超时设置为 5min，防止 Cron 积压
- AC5: 扫描结果（扫描数、转移数、失败数）写入 Prometheus metrics
**技术要点**:
- Redis 分布式锁 `lock:cron:subscription-sync` TTL=10min

---

## Epic 13: 可观测性

**状态**: 📋 Planned | **优先级**: P1 | **负责人**: backend

### 目标

实现生产级可观测性三要素：Prometheus 指标、Jaeger 分布式追踪、结构化审计日志，以及增强健康检查，满足 SLA 监控要求。

### Stories

#### Story 13.1: Prometheus Metrics

**状态**: 📋 Planned
**描述**: 暴露 `/metrics` 端点，提供业务和系统维度的 Prometheus 指标，供 Grafana 大盘使用。
**验收标准（AC）**:
- AC1: 账号指标: `identity_accounts_total`（按 status 分组）
- AC2: 交易指标: `identity_wallet_transactions_total`（按 type 分组）、`identity_transaction_amount_cny_total`
- AC3: 支付指标: `identity_payment_orders_total`（按 provider/status 分组）、`identity_payment_success_rate`
- AC4: 订阅指标: `identity_subscriptions_active`（按 product_id 分组）
- AC5: HTTP 指标: 请求数、响应时间直方图（p50/p95/p99），按 endpoint/status_code
- AC6: `/metrics` 端点只对内网（Prometheus Pod IP）开放，不经过公网 Ingress
**技术要点**:
- `github.com/prometheus/client_golang`，Gin 中间件记录 HTTP metrics
- 业务 metrics 在关键 app 层操作后 Inc/Observe

#### Story 13.2: Jaeger 分布式追踪

**状态**: 📋 Planned
**描述**: 在关键业务链路（账号查询 → 权益获取 → 钱包扣减）注入 OpenTelemetry trace，与 lurus-api 的 trace context 传播实现跨服务全链路追踪。
**验收标准（AC）**:
- AC1: 使用 OpenTelemetry SDK，trace 导出至 Jaeger（`OTEL_EXPORTER_JAEGER_ENDPOINT` env）
- AC2: HTTP handler 层自动注入 span（Gin OTel 中间件）
- AC3: 关键 app 层操作（SyncFromSubscription、MarkOrderPaid、RecalculateFromWallet）手动创建 span
- AC4: 跨服务 trace context 通过 W3C TraceContext header 传播（traceparent）
- AC5: `OTEL_TRACE_ENABLED=false` 时优雅禁用（不影响业务逻辑）
**技术要点**:
- `go.opentelemetry.io/otel` + `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin`

#### Story 13.3: 结构化审计日志

**状态**: 📋 Planned
**描述**: 关键操作（认证、支付、状态变更、Admin 操作）输出 JSON 结构化日志，包含 who/what/result 三要素，供 K8s 日志系统采集和告警。
**验收标准（AC）**:
- AC1: 所有审计日志使用 `log/slog` JSON handler，字段: timestamp/level/event/account_id/actor/resource/result/error
- AC2: 必须审计的事件: 账号创建/状态变更、订阅激活/取消/降级、支付订单状态变更、Admin 调账、兑换码核销
- AC3: 敏感字段（email、phone）在日志中脱敏（仅保留前 3 位 + `***`）
- AC4: 日志级别: 正常操作 INFO，失败 WARN，系统错误 ERROR
**技术要点**:
- `internal/pkg/audit/` 包封装审计日志方法
- 禁止在 audit log 中使用 `fmt.Sprintf` 字符串拼接（使用 slog 结构化字段）

#### Story 13.4: 健康检查增强

**状态**: 📋 Planned
**描述**: 当前 `/health` 只返回静态 ok，需增加 DB、Redis、NATS 连通性检查，区分 liveness 和 readiness 探针。
**验收标准（AC）**:
- AC1: `GET /health/live` 仅检查进程存活（总是 200），用于 K8s livenessProbe
- AC2: `GET /health/ready` 检查 PostgreSQL ping（`SELECT 1`）、Redis ping、NATS connection，任一失败返回 503
- AC3: Readiness 检查超时 2s，不阻塞正常请求路径
- AC4: 检查结果返回 JSON: `{"status":"ok","checks":{"db":"ok","redis":"ok","nats":"ok"}}`
- AC5: K8s Deployment 的 livenessProbe 和 readinessProbe 分别指向两个端点
**技术要点**:
- `internal/lifecycle/` 注册健康检查 handler
- `deploy/` K8s manifests 更新探针配置

---

## Epic 14: 财务合规

**状态**: 📋 Planned | **优先级**: P1 | **负责人**: backend

### 目标

满足财务和法务要求，提供发票/收据生成、退款工作流、套餐升降级差价计算、年度账单汇总和 GDPR 数据处理能力。

### Stories

#### Story 14.1: 收据 PDF 生成

**状态**: 📋 Planned
**描述**: 用户支付成功后可下载 PDF 格式的支付收据，包含订单信息、商品明细、支付金额和付款时间。
**验收标准（AC）**:
- AC1: `GET /api/v1/wallet/orders/:order_no/receipt` 返回 PDF 二进制（Content-Type: application/pdf）
- AC2: 收据包含: 公司名称/税号、订单号、产品名称、金额（CNY）、支付时间、用户 Lurus ID
- AC3: 只有订单 `status=paid` 时可生成收据，其他状态返回 404
- AC4: 收据只能由订单归属账号下载（accountID 校验）
- AC5: PDF 使用无中文字体乱码的嵌入字体（如思源黑体）
**技术要点**:
- `github.com/jung-kurt/gofpdf` 或 `github.com/signintech/gopdf`
- 收据模板定义于 `internal/pkg/receipt/`

#### Story 14.2: 退款工作流

**状态**: 📋 Planned
**描述**: 支持管理员发起退款，系统对接支付渠道 API 执行退款，并将 Credits 从用户钱包扣除（防止已消费的 Credits 也被退款）。
**验收标准（AC）**:
- AC1: `POST /admin/v1/orders/:order_no/refund` 发起退款，附退款金额和原因
- AC2: 退款金额 ≤ 已付金额，部分退款须注明原因
- AC3: 对接各支付渠道退款 API（Epay/Stripe/Creem），按原支付渠道路由
- AC4: 退款成功后: 订单 `status=refunded`，钱包扣减已充入的 Credits，产生 `TxTypeRefund` 交易
- AC5: 钱包余额不足以扣减时，仍完成退款但记录差额欠款状态，人工处理
- AC6: 所有退款操作写入审计日志
**技术要点**:
- `adapter/payment/*.go` 各 Provider 实现 `Refund(orderNo, amount) error`

#### Story 14.3: 套餐升降级差价计算（Proration）

**状态**: 📋 Planned
**描述**: 用户在订阅周期中途升级套餐时，按剩余天数比例计算差价，避免重复收费；降级时剩余价值退回 Credits。
**验收标准（AC）**:
- AC1: 升级: `proration = (new_price - old_price) * (remaining_days / total_days)`，向上取整到分
- AC2: 降级: `refund = (old_price - new_price) * (remaining_days / total_days)`，退回钱包 Credits
- AC3: `POST /api/v1/subscriptions/upgrade` 接受 `new_plan_id`，返回差价金额供用户确认
- AC4: 差价金额可优先从钱包余额扣减，不足时走外部支付
- AC5: 升降级后立即 SyncFromSubscription 更新权益快照
**技术要点**:
- `app/subscription_service.go::Upgrade`，新增 proration 计算函数

#### Story 14.4: 年度账单汇总

**状态**: 📋 Planned
**描述**: 每年 1 月 1 日自动生成上一年度账单汇总，用户可随时查看历史年度账单。
**验收标准（AC）**:
- AC1: `GET /api/v1/wallet/annual-summary?year=2025` 返回年度汇总
- AC2: 汇总包含: 总充值、总消费（按产品分组）、总订阅支出、期末余额
- AC3: 数据从 `billing.wallet_transactions` 聚合（按 `created_at` 年份过滤）
- AC4: 汇总支持 CSV 导出（`Accept: text/csv`）
**技术要点**:
- 聚合查询: `SELECT type, SUM(amount) FROM wallet_transactions WHERE EXTRACT(YEAR FROM created_at)=$1`

#### Story 14.5: GDPR 数据导出与删除

**状态**: 📋 Planned
**描述**: 支持用户申请导出本人全部数据（JSON 格式）或申请账号删除（软删除 + 脱敏），满足 GDPR 及国内数据安全法要求。
**验收标准（AC）**:
- AC1: `POST /api/v1/account/me/export-data` 触发异步生成，完成后通过邮件发送下载链接（有效期 24h）
- AC2: 导出包含: account 基本信息、wallet 历史、subscription 历史、referral 记录（不含其他用户的 PII）
- AC3: `DELETE /api/v1/account/me` 执行软删除: status=3，email/phone 替换为哈希值，display_name 替换为 "Deleted User"
- AC4: 删除后账号 JWT 立即失效（注销 Zitadel session）
- AC5: 数据导出文件使用密码加密（密码在邮件中发送）
**技术要点**:
- 软删除后 aff_code/email 的 UNIQUE 索引仍占位（使用 hash 替代原值）

---

## Epic 15: 平台扩展

**状态**: 📋 Planned | **优先级**: P2 | **负责人**: backend

### 目标

为下一阶段企业级功能打基础，包括 gRPC 接口、企业 Workspace、自定义域名、API Key 服务账号和细粒度 RBAC 权限系统。

### Stories

#### Story 15.1: gRPC 接口（端口 18105）

**状态**: 📋 Planned
**描述**: 在现有 HTTP API 基础上，新增 gRPC 服务端（端口 18105），提供高性能的服务间调用接口（替代 /internal/v1/ HTTP 接口）。
**验收标准（AC）**:
- AC1: Proto 定义 `IdentityService`: GetAccount / GetEntitlements / UpsertAccount / ReportUsage
- AC2: gRPC 服务端在独立 goroutine 启动，HTTP 服务不受影响
- AC3: 使用 mTLS 鉴权（替代 INTERNAL_API_KEY bearer token）
- AC4: gRPC 接口与 HTTP 接口行为一致，使用相同 app 层逻辑（不重复实现）
- AC5: Protobuf 定义版本化，向后兼容（添加字段不破坏已有客户端）
**技术要点**:
- `cmd/server/main.go` 启动两个监听器
- Proto 文件定义于 `api/proto/`

#### Story 15.2: 企业账号（Workspace）

**状态**: 📋 Planned
**描述**: 支持企业/团队创建 Workspace，邀请成员（seat 计费），统一管理子账号的产品权益。
**验收标准（AC）**:
- AC1: 新增 `identity.workspaces` 表，存储企业名称、Owner accountID、seat 数量
- AC2: `identity.workspace_members` 关联成员与 Workspace（含 role: owner/admin/member）
- AC3: Workspace Owner 购买 seat 订阅后，成员自动获得对应产品权益
- AC4: 成员移除时权益即时回收（Redis invalidate + DB 清除）
- AC5: Workspace 成员上限由 `seat` 数量控制
**技术要点**:
- 新增 `app/workspace_service.go`，seat 计费复用 SubscriptionService

#### Story 15.3: 自定义域名绑定（webmail 产品）

**状态**: 📋 Planned
**描述**: webmail 产品的高级套餐支持用户绑定自定义域名，lurus-identity 维护域名 → 账号的映射关系，供 lurus-webmail 查询。
**验收标准（AC）**:
- AC1: 新增 `identity.custom_domains` 表（account_id, product_id, domain, verified, verification_token）
- AC2: `POST /api/v1/domains` 添加域名，返回 DNS TXT 验证 token
- AC3: `POST /api/v1/domains/:domain/verify` 触发 DNS TXT 查询验证
- AC4: 域名验证成功后 `verified=true`，lurus-webmail 可通过内部 API 查询
- AC5: 仅订阅了支持自定义域名套餐（权益 key `custom_domain_enabled=true`）的账号可使用
**技术要点**:
- DNS TXT 验证: `net.LookupTXT(domain)` 匹配 verification_token

#### Story 15.4: API Key 管理（服务账号）

**状态**: 📋 Planned
**描述**: 用户可为自己的产品服务创建 API Key，Key 关联账号权益，供无头（headless）调用场景使用（替代 OAuth 登录）。
**验收标准（AC）**:
- AC1: 新增 `identity.api_keys` 表（key_hash, account_id, name, last_used_at, expires_at, scopes）
- AC2: `POST /api/v1/api-keys` 创建 API Key，响应中只返回一次明文 key（后续不可再查）
- AC3: Key 以 `lurus_` 为前缀 + 32 位随机 hex，存储时只存 SHA-256 hash
- AC4: 下游产品可通过内部 API 验证 API Key 并取得 account_id + 权益
- AC5: 单账号最多 10 个 API Key，超出返回 409
**技术要点**:
- 认证流程: Bearer Key → SHA-256 hash lookup → account_id
- 新增 `internal/v1/api-keys/verify` 内部端点

#### Story 15.5: 细粒度 RBAC 权限

**状态**: 📋 Planned
**描述**: 在现有 user/admin 二元角色基础上，实现资源级 RBAC 权限系统，支持产品级别的权限配置（如 workspace admin 可管理本 workspace 成员）。
**验收标准（AC）**:
- AC1: 新增 `identity.role_assignments` 表（account_id, resource_type, resource_id, role）
- AC2: 权限检查通过 `HasPermission(ctx, accountID, resource, action)` 统一调用
- AC3: 内置角色: `system_admin`、`workspace_owner`、`workspace_admin`、`workspace_member`
- AC4: 权限矩阵配置化（不硬编码 role-action 映射）
- AC5: 权限检查结果 Redis 缓存 5min，角色变更时 invalidate
**技术要点**:
- `internal/pkg/rbac/` 包实现权限引擎

---

## Epic 16: 运营工具

**状态**: 📋 Planned | **优先级**: P1 | **负责人**: backend + frontend

### 目标

为运营团队提供批量发码、营销活动管理、财务对账报表和异常订单处理工作台，提升运营效率和资金安全。

### Stories

#### Story 16.1: 批量兑换码生成

**状态**: 📋 Planned
**描述**: 管理员通过 Admin API 批量生成兑换码，支持指定数量、面额、使用次数、有效期和批次标签，并支持 CSV 导出。
**验收标准（AC）**:
- AC1: `POST /admin/v1/redemption-codes/batch` 批量生成，参数: count（1-10000）、reward_type、reward_value、max_uses、expires_at、batch_id
- AC2: 生成的 code 格式: 4×4 大写字母数字串（如 `ABCD-1234-WXYZ-5678`），全局唯一
- AC3: 批量生成采用 DB 批量 INSERT（非逐条），10000 条 < 5s
- AC4: `GET /admin/v1/redemption-codes/batch/:batch_id/export` 导出 CSV（code, reward_value, used_count, expires_at）
- AC5: `batch_id` 标记批次，支持按 batch_id 查询该批次使用情况统计
**技术要点**:
- `billing.redemption_codes.batch_id` 字段已存在
- 批量 INSERT: `gorm.DB.CreateInBatches` 每批 500 条

#### Story 16.2: 营销活动管理

**状态**: 📋 Planned
**描述**: 支持创建限时折扣活动和首充奖励规则，活动期间购买套餐自动应用折扣，首充用户额外赠送 Credits。
**验收标准（AC）**:
- AC1: 新增 `billing.campaigns` 表（name, type, discount_rate, bonus_credits, start_at, end_at, applicable_products, budget_cny）
- AC2: 活动类型: `discount`（百分比折扣）、`first_topup_bonus`（首充赠送）、`double_credits`（充值翻倍）
- AC3: 结账时自动查询当前有效活动并应用最优惠规则（同一订单最多一个活动）
- AC4: 活动设置预算上限，达到上限自动停止
- AC5: 管理界面实时显示活动使用量/预算消耗
**技术要点**:
- 活动查询加 Redis 缓存（TTL 1min），避免高频 DB 查询

#### Story 16.3: 财务对账报表

**状态**: 📋 Planned
**描述**: 提供按日/月维度的财务对账报表，包括各渠道收款金额、退款金额、实际入账 Credits 及差异分析。
**验收标准（AC）**:
- AC1: `GET /admin/v1/reports/reconciliation?start=2026-01-01&end=2026-01-31` 返回对账数据
- AC2: 报表字段: 支付渠道、订单数、总收款（CNY）、退款额、净收款、实际入账 Credits（CNY）、差异（应=0）
- AC3: 每日自动生成快照存表，查询读快照不 OLAP 实时扫描
- AC4: 报表支持 CSV/Excel 导出（`Accept` header 控制）
- AC5: 数据口径: 以 `paid_at` 为准，跨日 pending 订单以实际付款日归属
**技术要点**:
- `billing.daily_reconciliation` 快照表，每日 23:59 Cron 生成

#### Story 16.4: 异常订单处理工作台

**状态**: 📋 Planned
**描述**: 自动检测和展示异常订单（长时间 pending、支付成功但未入账、退款失败等），提供人工干预入口。
**验收标准（AC）**:
- AC1: `GET /admin/v1/orders/anomalies` 返回异常订单列表，分类: `stale_pending`（>30min pending）、`paid_not_credited`（paid 但余额未变）、`refund_failed`
- AC2: 异常检测 Cron 每 15min 运行，异常订单打标 `anomaly_type` 字段
- AC3: `POST /admin/v1/orders/:order_no/force-paid` 管理员手动标记订单为已付并触发入账（需二次确认）
- AC4: `POST /admin/v1/orders/:order_no/force-refund` 管理员强制退款（记录原因）
- AC5: 所有人工干预操作写入审计日志，不可删除
**技术要点**:
- `billing.payment_orders` 新增 `anomaly_type VARCHAR(32)` 字段
- 异常检测使用 Cron + Redis lock 避免重复标记
