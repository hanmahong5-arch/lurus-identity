# lurus-identity Architecture / 架构设计文档

**版本**: 1.0
**日期**: 2026-02-27
**状态**: Phase 1 生产就绪

---

## 目录 / Table of Contents

1. [架构概述 / Architecture Overview](#1-架构概述--architecture-overview)
2. [技术选型决策记录 / ADR](#2-技术选型决策记录--adr)
3. [分层架构详解 / Layered Architecture](#3-分层架构详解--layered-architecture)
4. [数据模型 / Data Model](#4-数据模型--data-model)
5. [API 设计规范 / API Design](#5-api-设计规范--api-design)
6. [安全架构 / Security Architecture](#6-安全架构--security-architecture)
7. [事件架构 / Event Architecture](#7-事件架构--event-architecture)
8. [缓存策略 / Cache Strategy](#8-缓存策略--cache-strategy)
9. [部署架构 / Deployment Architecture](#9-部署架构--deployment-architecture)
10. [可观测性 / Observability](#10-可观测性--observability)
11. [性能与伸缩性 / Performance & Scalability](#11-性能与伸缩性--performance--scalability)
12. [灾难恢复 / Disaster Recovery](#12-灾难恢复--disaster-recovery)
13. [架构演进路线 / Evolution Roadmap](#13-架构演进路线--evolution-roadmap)

---

## 1. 架构概述 / Architecture Overview

### 1.1 服务定位

lurus-identity 是 Lurus 平台的**统一用户层**，承担所有产品（lurus-api、lurus-gushen、lurus-webmail、lurus-switch）的账号、权益、计费职责。它是平台的信任锚点（trust anchor）：

- 任何产品需要判断"这个用户能不能做某件事"，都来查 lurus-identity 的权益快照
- 任何资金流转（充值、订阅扣费、退款）都通过 lurus-identity 的钱包账本
- 任何账号状态变更（注册、VIP 变化、订阅激活）都由 lurus-identity 发出事件

### 1.2 C4 容器图（文字描述）

```
┌─────────────────────────────────────────────────────────────────────┐
│  Lurus Platform (K3s Cluster, VPN-gated)                            │
│                                                                     │
│  ┌──────────────┐    ┌──────────────┐    ┌────────────────────┐    │
│  │  lurus-api   │    │ lurus-gushen │    │  lurus-webmail     │    │
│  │ (LLM Gateway)│    │  (Quant AI)  │    │  (Email SaaS)      │    │
│  └──────┬───────┘    └──────┬───────┘    └────────┬───────────┘    │
│         │                   │                     │                 │
│         │  /internal/v1/*   │  /internal/v1/*     │  /internal/v1/ │
│         │  Bearer token     │  Bearer token       │  Bearer token  │
│         └───────────────────┴─────────────────────┘                │
│                             │                                       │
│                    ┌────────▼────────────────────────────────────┐  │
│                    │         lurus-identity :18104               │  │
│                    │                                             │  │
│                    │  ┌────────────┐  ┌───────────────────────┐ │  │
│                    │  │  Gin HTTP  │  │  go:embed web/dist    │ │  │
│                    │  │  Router    │  │  (React SPA)          │ │  │
│                    │  └─────┬──────┘  └───────────────────────┘ │  │
│                    │        │                                    │  │
│                    │  ┌─────▼──────────────────────────────┐   │  │
│                    │  │          app/ (Use Cases)           │   │  │
│                    │  │  AccountSvc  SubSvc  WalletSvc      │   │  │
│                    │  │  EntitlementSvc  VIPSvc  ProductSvc │   │  │
│                    │  └─────┬──────────────────┬───────────┘   │  │
│                    │        │                  │                │  │
│                    │  ┌─────▼──────┐  ┌───────▼──────────┐    │  │
│                    │  │ adapter/   │  │ adapter/nats/     │    │  │
│                    │  │ repo/      │  │ Publisher         │    │  │
│                    │  │ (GORM)     │  │ Consumer          │    │  │
│                    │  └─────┬──────┘  └───────┬───────────┘    │  │
│                    └────────┼──────────────────┼───────────────┘  │
│                             │                  │                   │
│         ┌───────────────────┼──────────────────┼────────────────┐ │
│         │  Infrastructure   │                  │                │ │
│         │  ┌────────────────▼───┐  ┌───────────▼──────────┐    │ │
│         │  │ PostgreSQL         │  │ NATS JetStream        │    │ │
│         │  │ schema: identity   │  │ Stream: IDENTITY_     │    │ │
│         │  │ schema: billing    │  │ EVENTS                │    │ │
│         │  └────────────────────┘  └──────────────────────┘    │ │
│         │  ┌──────────────────────────────────────────────┐    │ │
│         │  │ Redis DB=3  (entitlement cache, TTL 5min)     │    │ │
│         │  └──────────────────────────────────────────────┘    │ │
│         └────────────────────────────────────────────────────────┘ │
│                                                                     │
│  ┌──────────────────┐   ┌──────────────────────────────────────┐   │
│  │  Zitadel OIDC    │   │  Payment Providers (External)        │   │
│  │  auth.lurus.cn   │   │  Epay (易支付) / Stripe / Creem      │   │
│  └──────────────────┘   └──────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘

外部请求路径:
  Browser / App → Traefik IngressRoute (identity.lurus.cn)
               → lurus-identity:18104
               → 按路径分发到四类处理器
```

### 1.3 核心原则

| 原则 | 体现 |
|------|------|
| 单一职责 | lurus-identity 是唯一的账号/权益/计费权威；其他服务只读、不写 |
| 权益快照优先 | 消费方不做权益计算，只读 account_entitlements 快照 |
| 账本不变性 | wallet_transactions 只追加；余额通过悲观锁原子更新 |
| 接口抽象 | app/ 层通过 Go interface 依赖倒置，不导入框架类型 |
| 幂等设计 | 所有写操作在重试安全，支付回调多次到达结果一致 |

---

## 2. 技术选型决策记录 / ADR

### ADR-001: HTTP 框架选用 Gin

**Context**
服务需要高吞吐的 HTTP 处理，具备良好的中间件生态。选项有：Gin、Hertz、Echo、标准库。

**Decision**
选用 Gin v1.10。

**Consequences**
- 正向：成熟生态（CORS、Recovery、Binding 内建）、社区活跃、测试简单
- 正向：与 Lurus 平台其他 Go 服务保持一致，降低认知负担
- 负向：Gin Context 不可跨层传递（在 adapter/handler 层止步，app/ 层不可见）
- 缓解：router.go 提取 account_id 后通过标准 context.Context 值传递给 app 层

---

### ADR-002: ORM 选用 GORM + PostgreSQL

**Context**
需要与 PostgreSQL 交互，数据模型含 JSONB 字段和多 schema（identity/billing）。

**Decision**
选用 GORM v2 + lib/pq 驱动，直接操作两个 PostgreSQL schema。

**Consequences**
- 正向：TableName() 方法支持跨 schema 操作（`identity.accounts`），无需 ORM 配置改动
- 正向：GORM 的悲观锁 `db.Set("gorm:query_option", "FOR UPDATE")` 满足钱包原子性需求
- 正向：JSONB 字段用 `json.RawMessage` 类型透传，不需要 GORM 插件
- 负向：GORM 隐藏了部分 SQL 行为，对复杂查询（聚合、窗口函数）需降级为 Raw SQL
- 缓解：repo 层封装所有 DB 交互，上层不感知 GORM；复杂统计查询用 Raw SQL 明示

---

### ADR-003: 权益快照设计（account_entitlements）

**Context**
消费方服务（lurus-api 等）需要高频查询"某用户对某产品有什么权限"。直接 JOIN subscriptions + product_plans 每次请求代价较高，且将业务逻辑泄漏到消费方。

**Decision**
在 lurus-identity 维护 account_entitlements 表作为预计算快照：
- 订阅激活/变更时，将 product_plans.features JSONB 展开写入快照表
- 消费方只读此快照，不关心订阅状态机
- Redis 缓存快照，TTL 5分钟，写入时主动失效

**Consequences**
- 正向：消费方 P99 延迟从 ~20ms（DB JOIN）降至 ~1ms（Redis GET）
- 正向：权益计算逻辑集中在 lurus-identity，消费方零业务逻辑
- 正向：新增权益字段只改 product_plans.features JSONB，无 schema 迁移
- 负向：权益有最多 5 分钟的缓存延迟
- 缓解：订阅激活/取消时同步写 DB 快照并主动 Redis Invalidate，延迟不超过 1 个请求周期

---

### ADR-004: 钱包账本不变性

**Context**
钱包余额变更必须可审计、可回溯。余额不得因并发操作产生竞态条件。

**Decision**
- wallet_transactions 只 INSERT，永不 UPDATE/DELETE
- 余额操作通过 `SELECT ... FOR UPDATE` 悲观锁保证原子性
- Wallet.balance 是冗余计算字段（可由 SUM(transactions) 还原）

**Consequences**
- 正向：完整审计链，任意时刻余额可由账本重算验证
- 正向：悲观锁消除并发充值/扣费的竞态窗口
- 正向：退款只需反向 INSERT，不需要修改历史记录
- 负向：高并发单账户写入时锁争用；对 99.9% 的 SaaS 用户场景（低频写）可接受
- 缓解：如未来出现高频扣费场景（如 Token 实时计费），引入预扣-结算两段式模式

---

### ADR-005: 产品特性存 JSONB

**Context**
不同产品、不同套餐的权益字段差异巨大（API 配额、并发数、模型白名单等），且随产品迭代频繁新增。

**Decision**
product_plans.features 存 JSONB，权益字段在运行时从 JSON 中提取并写入 account_entitlements（key/value/value_type 三元组）。

**Consequences**
- 正向：新增权益字段（如 `gpu_quota`）只需更新 product_plans 数据，零代码改动、零 schema 迁移
- 正向：value_type 字段（string/integer/boolean/decimal）让消费方可做类型安全转换
- 负向：无 DB 层面的字段约束，需在 app 层校验 features JSON 格式
- 缓解：admin API 在创建/更新 plan 时做格式校验；种子数据（migration 003）作为正确格式参考

---

### ADR-006: 多支付 Provider 抽象

**Context**
需同时支持易支付（中国市场，GET 回调）、Stripe（国际信用卡，POST Webhook）、Creem（海外订阅，POST Webhook），三者接口差异显著。

**Decision**
定义 `payment.Provider` interface + 可选的 `EpayCallbackVerifier` / `WebhookVerifier` interface。Webhook handler 在运行时做类型断言选择验签逻辑。

**Consequences**
- 正向：新增支付 Provider 只需实现 interface，不改 handler 核心逻辑
- 正向：Provider 间完全隔离，易支付的 GET 回调逻辑不污染 Stripe Webhook 路径
- 负向：三个 Provider 的错误处理方式各异，需在 webhook handler 中保持一致的错误响应格式
- 缓解：webhook handler 统一包装所有 Provider 错误为标准 JSON 响应

---

### ADR-007: 前端 go:embed 嵌入

**Context**
lurus-identity 含账号管理、订阅、钱包等用户可见页面。前端需与后端一起部署，避免跨域复杂性。

**Decision**
React 18 + Vite 6 + Semi UI 2.9 构建 dist/，通过 go:embed 嵌入 web/embed.go，在 cmd/server/main.go 挂载为 NoRoute SPA 处理器。

**Consequences**
- 正向：单镜像部署，无跨域、无独立前端 Pod 维护成本
- 正向：SPA 路由与 API 路由共享同一端口，Traefik 配置简单
- 负向：前端每次变更需重新构建 Go 二进制并重新部署
- 缓解：GitHub Actions 流水线自动完成 `bun run build → go build → GHCR push` 全流程

---

### ADR-008: VIP 双来源合并

**Context**
VIP 等级由两条独立路径决定：累计消费（spend_grant）和年度订阅档位（yearly_sub_grant），任一路径达到阈值都应提升 VIP。

**Decision**
`level = MAX(yearly_sub_grant, spend_grant)`，两路独立计算、统一取最大值。

**Consequences**
- 正向：逻辑简单，不存在"路径之间互相抵消"的复杂性
- 正向：两路计算幂等，重复触发不产生副作用
- 负向：yearly_sub_grant 到期后不自动降级（需定时任务扫描），当前是已知架构债务
- 缓解：Phase 2 引入定时任务（CronJob），扫描 level_expires_at 并触发降级

---

## 3. 分层架构详解 / Layered Architecture

### 3.1 层次划分

```
cmd/server/main.go
│  依赖注入：连接所有层，组装 DI 容器
│
├── internal/adapter/handler/          [Transport Layer]
│   ├── router/router.go               路由注册、认证中间件
│   ├── account.go                     用户账号 HTTP handler
│   ├── subscription.go                订阅 HTTP handler
│   ├── wallet.go                      钱包 HTTP handler
│   ├── product.go                     产品目录 HTTP handler
│   ├── internal_api.go                服务内部 API handler
│   └── webhook.go                     支付回调 handler
│
├── internal/adapter/repo/             [Persistence Layer]
│   ├── account.go                     GORM 账号仓储
│   ├── subscription.go                GORM 订阅/权益仓储
│   ├── wallet.go                      GORM 钱包/账本仓储
│   ├── product.go                     GORM 产品/套餐仓储
│   └── vip.go                         GORM VIP 仓储
│
├── internal/adapter/nats/             [Messaging Layer]
│   ├── publisher.go                   NATS JetStream 发布者
│   └── consumer.go                    NATS JetStream 消费者
│
├── internal/adapter/payment/          [Payment Integration Layer]
│   ├── interfaces.go                  Provider interface 定义
│   ├── epay.go                        易支付 Provider 实现
│   ├── stripe.go                      Stripe Provider 实现
│   └── creem.go                       Creem Provider 实现
│
├── internal/app/                      [Application Layer - 核心]
│   ├── interfaces.go                  所有依赖的 Go interface 定义
│   ├── account_service.go             账号用例编排
│   ├── subscription_service.go        订阅生命周期编排
│   ├── wallet_service.go              钱包 / 支付订单编排
│   ├── entitlement_service.go         权益计算与缓存
│   ├── vip_service.go                 VIP 等级计算
│   ├── product_service.go             产品目录管理
│   └── referral_service.go            推荐奖励逻辑
│
├── internal/domain/entity/            [Domain Layer]
│   ├── account.go                     Account, OAuthBinding 实体
│   ├── subscription.go                Subscription, AccountEntitlement 实体
│   ├── wallet.go                      Wallet, WalletTransaction, PaymentOrder, RedemptionCode 实体
│   ├── product.go                     Product, ProductPlan 实体
│   ├── vip.go                         AccountVIP, VIPLevelConfig 实体
│   └── referral.go                    ReferralRewardEvent 实体
│
├── internal/lifecycle/                [Infrastructure Layer]
│   └── lifecycle.go                   后台任务生命周期管理
│
└── internal/pkg/                      [Shared Utilities]
    ├── config/config.go               环境变量配置加载与校验
    ├── cache/entitlement.go           Redis 权益缓存实现
    └── event/types.go                 NATS 事件类型定义
```

### 3.2 依赖方向规则

```
domain/entity   ←  app/  ←  adapter/{handler,repo,nats,payment}  ←  cmd/server
```

- **domain/entity**: 纯数据结构，零外部依赖
- **app/**: 只依赖 domain/entity 和 pkg/event；通过 interface 依赖适配器，永不 import adapter 包
- **adapter/**: 依赖 app/ interface，实现具体 IO 操作
- **cmd/server/main.go**: 唯一允许 import 所有层的地方，负责 DI 组装

### 3.3 接口抽象机制

app/interfaces.go 定义了所有外部依赖的最小接口（accountStore、walletStore、vipStore、subscriptionStore、planStore、entitlementCache）。

好处体现在：
1. **可测试性**: mock_test.go 实现这些 interface 用于单元测试，无需数据库
2. **可替换性**: 将 GORM 换成 sqlx 或换数据库，只改 adapter/repo，不动 app/
3. **无框架污染**: app/ 层看不到 gin.Context、http.Header 等传输层类型

---

## 4. 数据模型 / Data Model

### 4.1 Schema 划分

| Schema | 职责 | 表数量 |
|--------|------|--------|
| identity | 账号、产品目录、订阅、权益、VIP | 8 张 |
| billing | 钱包、账本、支付订单、兑换码、推荐奖励 | 5 张 |

**设计理由**: 两个 schema 物理上在同一 PostgreSQL 实例，通过 K8s Secret 的 `search_path=identity,billing,public` 统一访问。分 schema 的原因是业务职责明确隔离——账号身份属 identity，资金流水属 billing——同时保留未来独立拆库（独立 PG 实例）的可能性，届时只需修改 DSN 和外键约束（改为应用层维护）。

### 4.2 ER 关系说明

```
identity.accounts (中心实体)
│
├──< identity.account_oauth_bindings    (1:N，每个账号可绑定多个 OAuth 提供商)
│     UNIQUE(provider, provider_id)     每个外部身份只能绑一个 Lurus 账号
│
├──< identity.subscriptions             (1:N，历史订阅记录)
│     UNIQUE INDEX (account_id, product_id) WHERE status IN ('active','grace','trial')
│     每个账号每个产品同一时刻只能有一个 live 订阅
│
├──< identity.account_entitlements      (1:N，权益快照)
│     UNIQUE(account_id, product_id, key)
│     由 EntitlementService.SyncFromSubscription 维护
│
├──1  identity.account_vip              (1:1，每账号一行)
│     level = MAX(yearly_sub_grant, spend_grant)
│
└──1  billing.wallets                   (1:1，每账号一个钱包)
      │
      ├──< billing.wallet_transactions  (1:N，只追加账本)
      │
      └──< billing.payment_orders       (1:N，支付订单记录)

identity.products
└──< identity.product_plans             (1:N，每产品多个套餐)
      features JSONB                    零 schema 迁移扩展权益字段

billing.redemption_codes                独立表，与账号的关联在应用层（used_count 限流）

billing.referral_reward_events
  referrer_id → identity.accounts.id   推荐人
  referee_id  → identity.accounts.id   被推荐人
```

### 4.3 关键约束与索引

| 约束/索引 | 目的 |
|-----------|------|
| `UNIQUE INDEX ON subscriptions(account_id, product_id) WHERE status IN ('active','grace','trial')` | DB 层保证同一产品只有一个 live 订阅，防止应用层并发漏洞 |
| `UNIQUE(account_id, product_id, key)` on account_entitlements | 权益快照 upsert 幂等 |
| `UNIQUE(provider, provider_id)` on oauth_bindings | 防止同一第三方身份绑定多账号 |
| `wallet_transactions` 无 UPDATE 权限（应用层约定） | 账本不变性，审计可信 |
| `FOR UPDATE` on wallet balance read | 悲观锁防止并发充值余额错误 |
| `idx_subs_expires` 部分索引 | 高效扫描即将到期订阅（定时任务查询路径） |

### 4.4 数据类型设计

| 字段 | 类型 | 设计原因 |
|------|------|---------|
| wallet.balance | DECIMAL(14,4) | 精确到 0.0001 CNY，避免浮点误差 |
| account.lurus_id | VARCHAR(16) | "LU" + 7 位数字，可读可识别 |
| product_plans.features | JSONB | 支持 GIN 索引查询特定字段 |
| wallet_transactions.metadata | JSONB | 存储额外上下文（如 LLM model_name） |
| subscription.expires_at | TIMESTAMPTZ NULL | NULL 表示永久有效（forever 计费周期） |

---

## 5. API 设计规范 / API Design

### 5.1 路由分层

| 前缀 | 认证方式 | 访问者 | 职责 |
|------|----------|--------|------|
| `/api/v1/*` | Zitadel JWT（X-Account-ID header，Phase 1 占位） | 终端用户/SPA | 账号自助操作、订阅、钱包 |
| `/admin/v1/*` | Admin JWT role（占位） | 运营/管理后台 | 账号管理、产品配置、钱包调整 |
| `/internal/v1/*` | Bearer INTERNAL_API_KEY | 平台内部服务 | 服务间账号/权益查询 |
| `/webhook/*` | 无认证（各 Provider 签名验证） | 第三方支付 Provider | 支付结果异步通知 |
| `/*` (NoRoute) | 无 | 浏览器 | SPA 静态文件（go:embed） |

### 5.2 Internal API 契约（最高稳定性要求）

内部 API 是跨服务契约，一旦变更需要协调多个消费方。遵循：

- 路径语义稳定，参数扁平（不嵌套 optional union）
- 响应结构稳定，新字段可增、旧字段不删
- 错误码固定：200/400/401/404/500

**关键端点**:

```
GET  /internal/v1/accounts/by-zitadel-sub/:sub
     → 根据 Zitadel sub 查账号，返回 Account 对象
     → 404: 账号不存在（消费方应触发 upsert）

POST /internal/v1/accounts/upsert
     → 创建或更新账号（Zitadel webhook 触发）
     Body: { zitadel_sub, email, display_name, avatar_url }
     → 幂等，重复调用安全

GET  /internal/v1/accounts/:id/entitlements/:product_id
     → 返回 map[string]string 权益快照，Redis 缓存 5min
     → 无权益时返回 {"plan_code": "free"}，永不 404

GET  /internal/v1/accounts/:id/subscription/:product_id
     → 返回当前 live 订阅
     → 404: 无 live 订阅

POST /internal/v1/usage/report
     Body: { account_id, amount_cny }
     → 触发 VIP 重新计算，异步写
     → 永远返回 200 { "accepted": true }（fire-and-forget 语义）
```

### 5.3 User API 设计原则

- **分页**: 所有列表接口使用 `?page=1&page_size=20`，返回 `{ items, total, page, page_size }`
- **错误格式**: 统一 `{ "error": "<human-readable message>" }`
- **幂等键**: 创建支付订单生成 `LO{yyyyMMdd}{uuid[:8]}` 格式 order_no，用于重复提交检测
- **敏感字段**: 钱包余额精确到分（显示），内部精确到 0.0001
- **时间格式**: 所有时间字段 UTC，RFC3339 格式

### 5.4 Webhook 安全设计

```
易支付 (Epay):
  - GET /webhook/epay?...
  - MD5(params + key) 签名验证
  - 返回 "success" 字符串（Provider 约定）

Stripe:
  - POST /webhook/stripe
  - Stripe-Signature header，HMAC-SHA256
  - 返回 200 空 body

Creem:
  - POST /webhook/creem
  - Creem 自定义签名头
  - 返回 200 空 body

所有 Webhook handler 遵循:
1. 先验签，验签失败返回 400，不处理业务逻辑
2. 业务处理幂等（MarkOrderPaid 检查 status==paid 即返回）
3. 写日志：who(provider)/what(order_no)/result(paid/failed)
```

---

## 6. 安全架构 / Security Architecture

### 6.1 认证层次

```
层次 1: 网络隔离
  - K3s 节点要求 lurus.cn/vpn: "true" 标签
  - NATS/Redis/PostgreSQL 仅 K8s cluster 内网可达
  - Traefik IngressRoute 是唯一公网入口

层次 2: 传输加密
  - Traefik 终止 TLS（Let's Encrypt / 自签）
  - 服务间通信在 K8s cluster 内，走 ClusterIP（可选加 mTLS）

层次 3: API 认证（当前状态）
  - /api/v1/*: X-Account-ID header（Phase 1 占位，待替换为完整 JWKS 验证）
  - /internal/v1/*: Bearer INTERNAL_API_KEY（K8s Secret 注入）
  - /webhook/*: Provider 签名验证（MD5/HMAC-SHA256）
  - /admin/v1/*: 占位（待完整 JWT role claim 验证）

层次 4: 容器安全
  - runAsUser: 65534 (nobody)
  - readOnlyRootFilesystem: true
  - allowPrivilegeEscalation: false
  - capabilities.drop: [ALL]
  - scratch 镜像（无 shell，无包管理器，攻击面极小）
```

### 6.2 已知安全债务（Phase 1 限制）

| 债务 | 风险等级 | 缓解措施 | 修复计划 |
|------|----------|----------|---------|
| JWT 验证是占位（读 X-Account-ID header） | 高 | Traefik ForwardAuth 层做初步验证 | Phase 2 实现完整 JWKS 验证 |
| Admin 角色验证是占位 | 高 | 仅内网可达；运营操作通过 kubectl port-forward | Phase 2 实现 admin role claim |
| NATS 无 TLS/认证 | 中 | K8s 网络隔离，仅集群内可达 | 引入 NATS TLS + NKey |
| 无限流中间件 | 中 | K8s resource limits 兜底 | Phase 2 引入 Redis-based rate limiter |

### 6.3 Secret 管理

```yaml
K8s Secret: lurus-identity-secrets
  - DATABASE_DSN       # PostgreSQL DSN 含密码
  - INTERNAL_API_KEY   # 服务间认证密钥
  - STRIPE_SECRET_KEY  # Stripe API 密钥
  - STRIPE_WEBHOOK_SECRET
  - EPAY_PARTNER_ID
  - EPAY_KEY
  - CREEM_API_KEY
  - CREEM_WEBHOOK_SECRET
```

所有 Secret 通过 K8s Secret → Pod 环境变量注入，不落磁盘，不出现在镜像层。config.Load() 在启动时校验 required 字段，缺失即 panic（fast-fail，防止带空密钥的实例上线）。

### 6.4 数据安全

- 密码：lurus-identity 不存储密码（全由 Zitadel 管理）
- PII 字段（email/phone）：存储但当前无静态加密，依赖 PostgreSQL volume 加密
- 支付信息：不存储卡号/CVV；仅存 external_id（Provider 侧引用）和 callback_data JSONB（Provider 返回的公开回调数据）

---

## 7. 事件架构 / Event Architecture

### 7.1 NATS JetStream 配置

```
Stream: IDENTITY_EVENTS
  Subjects:  identity.>         (lurus-identity 发布)
  MaxAge:    7 days
  Retention: LimitsPolicy
  Storage:   FileStorage
  Replicas:  1 (Phase 1 单节点)

Stream: LLM_EVENTS (由 lurus-api 维护)
  Subject:   llm.usage.reported (lurus-identity 消费)
```

### 7.2 发布事件清单

| Subject | 触发时机 | Payload |
|---------|----------|---------|
| `identity.account.created` | 新账号首次 upsert | { lurus_id, email } |
| `identity.subscription.activated` | Activate() 成功 | { subscription_id, plan_code, expires_at } |
| `identity.subscription.expired` | EndGrace() 执行 | { subscription_id } |
| `identity.topup.completed` | MarkOrderPaid() 充值路径 | { payment_order_id, amount_cny, credits_added } |
| `identity.entitlement.updated` | SyncFromSubscription() / 管理员 grant | { keys: [...] } |
| `identity.vip.level_changed` | RecalculateFromWallet() / GrantYearlySub() | { old_level, new_level } |

### 7.3 消费事件

| Subject | 来源 | 处理逻辑 |
|---------|------|---------|
| `llm.usage.reported` | lurus-api | 触发 VIPService.RecalculateFromWallet()，更新 spend_grant |

Consumer 使用 Queue Subscribe（queue group: `lurus-identity-llm-usage`），确保多实例场景下每条消息只处理一次。Durable Consumer 配置保证重启后续接消费，MaxDeliver=5 防止毒丸消息无限重试。

### 7.4 事件 Envelope 设计

```json
{
  "event_id":   "550e8400-e29b-41d4-a716-446655440000",  // UUID，幂等去重 key
  "event_type": "identity.subscription.activated",
  "account_id": 42,
  "lurus_id":   "LU0000042",
  "product_id": "llm-api",
  "payload":    { "subscription_id": 17, "plan_code": "pro", "expires_at": "..." },
  "occurred_at": "2026-02-27T10:00:00Z"
}
```

标准 Envelope 让消费方无需解析具体 payload 即可做路由决策（按 event_type/account_id 过滤）。

---

## 8. 缓存策略 / Cache Strategy

### 8.1 Redis 使用规范

```
Redis DB:  3 (专用，与其他服务隔离)
Redis Addr: redis.messaging.svc:6379
```

**Key 命名规范**:
```
identity:entitlements:{account_id}:{product_id}
示例: identity:entitlements:42:llm-api
```

### 8.2 权益缓存生命周期

```
写入时机:
  1. EntitlementService.Refresh() — cache miss 时从 DB 重建
  2. EntitlementService.SyncFromSubscription() — 订阅变更后 Invalidate + 下次 Get 重建

失效时机:
  1. 主动 Invalidate — SyncFromSubscription()、ResetToFree()、AdminGrant()
  2. TTL 自然过期 — 默认 5 分钟（CACHE_ENTITLEMENT_TTL 环境变量可调）

读取路径:
  Get() → Redis HIT → return
        → Redis MISS → PostgreSQL read → Redis SET(TTL) → return
```

### 8.3 缓存值格式

```json
{
  "plan_code": "pro",
  "monthly_quota": "1000000",
  "concurrent_requests": "10",
  "model_whitelist": "gpt-4o,claude-3-opus"
}
```

JSON 序列化的 `map[string]string`，消费方按需类型转换（参考 value_type 字段）。

### 8.4 缓存一致性保证

- **写后读一致性**: SyncFromSubscription 先写 DB，后 Invalidate Redis。下一次 Get 必从 DB 读最新数据。
- **Redis 宕机降级**: cache.Get() 失败时自动 fallback 到 DB 查询，EntitlementService.Get() 捕获错误后调 Refresh()。Redis 故障不影响功能，只影响延迟。
- **最终一致性窗口**: 最大 5 分钟（TTL），主动 Invalidate 后立即一致。

---

## 9. 部署架构 / Deployment Architecture

### 9.1 K8s 资源清单

```yaml
Namespace:  lurus-identity
Deployment: lurus-identity
  Replicas: 1 (Phase 1)
  Strategy:  RollingUpdate (maxUnavailable=0, maxSurge=1) — 零宕机滚动发布
  Image:     ghcr.io/hanmahong5-arch/lurus-identity:main

Service: identity-service (ClusterIP)
  Port 18104: HTTP API
  Port 18105: gRPC (预留，Phase 2)

IngressRoute: identity.lurus.cn → identity-service:18104
```

### 9.2 资源配额

```yaml
requests:
  memory: 128Mi
  cpu:    50m
limits:
  memory: 512Mi
  cpu:    300m
```

**选型理由**: 静态二进制 + scratch 镜像，内存 footprint 极小（含 React dist ~20MB）。CPU limits 300m 满足当前单实例场景；内存 512Mi 是 Go GC + PostgreSQL 连接池 + 前端静态文件的安全上限。

### 9.3 数据库连接池

```go
MaxOpenConns:    25       // 防止超出 PG max_connections 配额
MaxIdleConns:    5        // 保持最小热连接，降低握手开销
ConnMaxLifetime: 5 min   // 防止 PG 主动断连引起的连接泄漏
```

### 9.4 健康探针

```yaml
livenessProbe:
  GET /health → {"status":"ok","service":"lurus-identity"}
  initialDelay: 15s, period: 15s, timeout: 5s
  # 进程存活检测，失败触发 Pod 重启

readinessProbe:
  GET /health
  initialDelay: 5s, period: 5s, timeout: 3s
  # 就绪检测，失败时从 Service endpoints 摘除（滚动发布安全窗口）
```

### 9.5 GitOps 发布流程

```
代码 push → main branch
  → GitHub Actions CI
      ├── go test -v ./...
      ├── bun run build (web/)
      └── docker build + push → ghcr.io/hanmahong5-arch/lurus-identity:main
  → ArgoCD 检测镜像变更
      → lurus-identity Deployment rollout
      → RollingUpdate: 新 Pod ready 后摘除旧 Pod
```

ArgoCD ApplicationSet 配置在 `deploy/argocd/appset-services.yaml`（平台层维护）。

### 9.6 优雅关闭序列

```
SIGTERM 接收
  → signal.NotifyContext 取消
  → errgroup 并发触发:
      1. NATS Consumer: ctx.Done() → 停止消费新消息
      2. HTTP Server: Shutdown(30s 超时)
          → 等待在途请求完成
          → 拒绝新连接
  → 所有 goroutine 退出 → 程序退出 0
```

30 秒关闭超时（`SHUTDOWN_TIMEOUT` 可调）覆盖最长在途请求（含支付 Provider 外调最长约 15 秒）。

---

## 10. 可观测性 / Observability

### 10.1 结构化日志

**当前实现**（Phase 1）：使用 Go 标准库 `log/slog`，默认 Text 格式。

```go
// 关键操作日志示例
slog.Info("event published",
    "subject", ev.EventType,
    "event_id", ev.EventID,
    "seq", ack.Sequence,
)

slog.Error("handle llm usage", "err", err)
```

**Phase 2 目标**: 切换为 JSON 格式（`slog.NewJSONHandler`），字段规范：

```json
{
  "time":       "2026-02-27T10:00:00Z",
  "level":      "INFO",
  "msg":        "subscription activated",
  "service":    "lurus-identity",
  "account_id": 42,
  "product_id": "llm-api",
  "plan_code":  "pro",
  "duration_ms": 12
}
```

**必须记录的关键操作**（who/what/result 三要素）:

| 操作类型 | 必须记录的字段 |
|----------|--------------|
| 账号注册 / upsert | account_id, lurus_id, zitadel_sub, email |
| 订阅激活 | account_id, product_id, plan_code, subscription_id |
| 支付完成 | account_id, order_no, amount_cny, provider |
| 权益变更 | account_id, product_id, keys[], source |
| VIP 等级变化 | account_id, old_level, new_level |
| Webhook 验签失败 | provider, remote_addr, reason |

### 10.2 Metrics（Phase 2 规划）

使用 Prometheus client_golang 暴露 `/metrics`：

```
# HTTP 层
http_requests_total{method, path, status}
http_request_duration_seconds{method, path}

# 业务层
identity_subscriptions_total{product_id, plan_code, action}  # activate/cancel/expire
identity_wallet_transactions_total{type}                      # topup/debit/refund
identity_payment_orders_total{provider, status}
identity_vip_level_changes_total{old_level, new_level}

# 缓存层
identity_cache_hits_total{product_id}
identity_cache_misses_total{product_id}

# 基础设施
identity_db_connections_open
identity_nats_publish_total{subject, status}
```

### 10.3 Tracing（Phase 2 规划）

接入 OpenTelemetry，在关键路径注入 Trace：
- HTTP handler → app service → repo（DB 查询）
- NATS publish/consume
- Redis 操作
- Payment Provider HTTP 调用

Span 携带 `account_id`、`product_id`、`order_no` 等业务 attribute，跨服务 trace propagation 通过 W3C TraceContext header。

### 10.4 告警规则（运维参考）

| 告警 | 条件 | 严重度 |
|------|------|--------|
| 服务宕机 | `up{service="lurus-identity"} == 0` 持续 1 分钟 | Critical |
| 高错误率 | HTTP 5xx > 5% 持续 5 分钟 | Warning |
| 支付回调延迟 | webhook 处理 P99 > 5s | Warning |
| 权益缓存命中率低 | cache_hits / (hits + misses) < 80% | Info |
| DB 连接池饱和 | db_connections_open / max > 80% | Warning |
| NATS 消费积压 | consumer_pending_messages > 1000 | Warning |

---

## 11. 性能与伸缩性 / Performance & Scalability

### 11.1 当前性能特征（Phase 1 单实例）

| 路径 | 预期 P99 延迟 | 瓶颈 |
|------|-------------|------|
| GET /internal/.../entitlements（缓存命中） | < 2ms | Redis |
| GET /internal/.../entitlements（缓存 miss） | < 20ms | PostgreSQL |
| POST /internal/accounts/upsert | < 30ms | PostgreSQL |
| POST /api/v1/subscriptions/checkout（含支付跳转） | < 500ms | Payment Provider |
| POST /webhook/*（支付回调处理） | < 100ms | PostgreSQL（含 FOR UPDATE） |

### 11.2 水平扩展路径

lurus-identity 设计为**无状态服务**（所有状态在 PostgreSQL/Redis/NATS），水平扩展无额外约束：

```
Step 1: Deployment replicas: 1 → N
  - HTTP 请求由 K8s Service 负载均衡（round-robin）
  - NATS Consumer: Queue Subscribe 自动分摊到多实例
  - 权益缓存: 多实例共享同一 Redis，一致性不变
  - 钱包操作: FOR UPDATE 锁在 PostgreSQL 层，多实例安全

Step 2: PostgreSQL 读写分离（如需）
  - 只读副本承接 entitlement 查询（cache miss 路径）
  - 写操作（充值、订阅激活）继续走主库
  - 需在 repo 层引入读写分离连接池

Step 3: Redis 集群（如需）
  - entitlement cache key 无跨 slot 依赖，直接迁移 Redis Cluster
  - key pattern: identity:entitlements:{account_id}:{product_id}
```

### 11.3 已知性能限制

| 限制 | 影响场景 | 缓解方案 |
|------|----------|---------|
| 无限流中间件 | 爬虫/DDoS 打爆 DB | Phase 2 引入 Redis token bucket |
| 无熔断器（支付 Provider 直调） | Stripe 超时导致请求堆积 | Phase 2 引入 hystrix-go 或 sentinel |
| 钱包 FOR UPDATE 行级锁 | 单账户极高频扣费 | 对 LLM 计费场景引入异步预扣-结算 |
| NATS Replicas=1 | 单节点 NATS 故障丢消息 | Phase 2 提升至 Replicas=3（需 3 节点 K8s） |

---

## 12. 灾难恢复 / Disaster Recovery

### 12.1 数据持久化级别

| 数据 | 持久化方式 | 恢复 RTO | 恢复 RPO |
|------|-----------|---------|---------|
| PostgreSQL（主账本） | K8s PVC + 定期备份 | < 30min（恢复备份） | < 24h（取决于备份频率） |
| Redis 权益缓存 | 非持久（可重建） | < 1min（自动从 DB 重建） | 0（无需恢复） |
| NATS 事件（FileStorage） | K8s PVC，7 天保留 | < 10min | < 7 days |

### 12.2 Redis 故障处理

Redis 宕机时：
1. `cache.Get()` 返回 error
2. `EntitlementService.Get()` 检测到 error，调 `Refresh()` 直接查 PostgreSQL
3. 服务继续正常工作，延迟从 ~2ms 升至 ~20ms
4. Redis 恢复后，缓存自动重建（无需人工干预）

### 12.3 NATS 故障处理

NATS 宕机时：
1. Publisher.Publish() 返回 error，当前实现 `_ = err`（非致命，事件丢失）
2. Consumer 退出 Run() 循环，errgroup 检测到并触发 HTTP Server 关闭
3. **改进计划**: Publisher 失败时写 PostgreSQL outbox 表，NATS 恢复后 replay

### 12.4 PostgreSQL 故障处理

PostgreSQL 宕机时：
1. 所有写操作（订阅、充值）失败，返回 500
2. 权益查询在 Redis 缓存有效期内继续服务（最多 5 分钟）
3. 健康探针 /health 当前只检查进程活跃（未检查 DB 连通性）
4. **改进计划**: /health 加 DB ping 检测，PG 故障时 readiness probe 失败，Pod 从 Service 摘除

### 12.5 Pod 重启恢复

K8s 自动重启 CrashLoop Pod。重启后：
1. config.Load() 校验所有必要环境变量
2. 连接 PostgreSQL/Redis/NATS（含 NATS RetryOnFailedConnect）
3. NATS Consumer 续接 Durable Consumer offset，不丢消息
4. HTTP Server 就绪后 readiness probe 通过，流量恢复

---

## 13. 架构演进路线 / Evolution Roadmap

### Phase 1（当前）—— 生产基础

**已实现**:
- 完整的账号/订阅/钱包/权益/VIP 领域模型
- 三路支付（易支付/Stripe/Creem）+ Webhook 验签
- NATS JetStream 事件发布/消费
- Redis 权益缓存（5min TTL，主动失效）
- go:embed SPA 前端
- K8s Deployment + RollingUpdate + 优雅关闭
- GitOps（GitHub Actions → GHCR → ArgoCD）

**已知欠账（按优先级）**:

| 优先级 | 欠账项 | 影响 |
|--------|--------|------|
| P0 | 完整 JWT JWKS 验证（替换 X-Account-ID 占位） | 安全漏洞 |
| P0 | Admin API 角色验证（替换占位） | 安全漏洞 |
| P1 | Redis 限流中间件 | DDoS 风险 |
| P1 | 支付 Provider 熔断器 | 级联故障风险 |
| P2 | 订阅到期定时续费（CronJob） | 手动运维负担 |
| P2 | Prometheus metrics 端点 | 可观测性盲区 |
| P2 | JSON 结构化日志 | 生产排查效率 |
| P3 | NATS Publisher outbox 模式（防事件丢失） | 数据一致性 |
| P3 | PostgreSQL 只读副本 | 高流量下 DB 压力 |

---

### Phase 2 —— 生产加固

**目标**: 消除 P0/P1 安全欠账，完善可观测性，实现 gRPC 内部通信。

**关键变更**:

#### 2.1 完整 JWT 验证

```go
// 替换 jwtAuth() 中的 X-Account-ID 占位实现
// 使用 Zitadel JWKS 端点验证 JWT，提取 claims
func jwtAuth(jwksURL string) gin.HandlerFunc {
    keySet := jwk.NewAutoRefresh(ctx)
    keySet.Configure(jwksURL, jwk.WithRefreshInterval(15*time.Minute))
    return func(c *gin.Context) {
        token := extractBearerToken(c)
        // 验证签名 + exp + iss + aud
        // 从 claims 提取 account_id
    }
}
```

#### 2.2 gRPC 内部通信（端口 18105）

当前 Internal HTTP API 的主要消费方是 lurus-api（高频调用 GetEntitlements）。Phase 2 将引入 gRPC：

```protobuf
service IdentityInternal {
  rpc GetEntitlements(GetEntitlementsRequest) returns (EntitlementMap);
  rpc GetAccountByZitadelSub(GetAccountRequest) returns (Account);
  rpc UpsertAccount(UpsertAccountRequest) returns (Account);
  rpc ReportUsage(UsageReport) returns (UsageAck);
}
```

**迁移策略**:
1. 新增 gRPC server 在 :18105，与现有 HTTP API 并行运行
2. 消费方逐一切换（先 lurus-api，后其他服务）
3. HTTP internal API 保留兼容期（3个月），随后废弃

**收益**:
- 延迟降低 40-60%（Protocol Buffer 序列化 vs JSON）
- 强类型契约（proto 文件作为单一 source of truth）
- 支持服务端 streaming（未来推权益变更通知）

#### 2.3 定时任务（CronJob）

```yaml
# 订阅到期处理
apiVersion: batch/v1
kind: CronJob
metadata:
  name: identity-subscription-expiry
spec:
  schedule: "*/5 * * * *"   # 每 5 分钟
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: expiry-worker
              image: ghcr.io/hanmahong5-arch/lurus-identity:main
              command: ["/lurus-identity", "--mode=cron", "--job=subscription-expiry"]
```

CronJob 查询 `expires_at < NOW() AND status = 'active'` 的订阅，调用 `SubscriptionService.Expire()` 进入宽限期，7 天后调 `EndGrace()` 重置权益。

#### 2.4 可观测性完善

```
Prometheus metrics → Grafana Dashboard
OpenTelemetry traces → Tempo / Jaeger
JSON logs → Loki / ELK
```

---

### Phase 3 —— 高可用与多区域

**目标**: 支持 3 副本高可用，为多区域部署预留。

**关键变更**:
- PostgreSQL 主从（CloudNativePG Operator），读写分离
- Redis Sentinel / Cluster 模式
- NATS JetStream Replicas=3（需 3 节点 K8s）
- Deployment replicas: 3，PodAntiAffinity 强制跨节点
- 引入全局限流（Redis token bucket，跨 Pod 共享计数）
- 考虑 lurus-identity 的 billing schema 独立为 lurus-billing 服务（如业务规模需要）

---

*本文档基于 lurus-identity 代码库（commit 截至 2026-02-27）生成，反映 Phase 1 生产状态。架构变更时同步更新本文档。*
