# Code Review Checklist — lurus-identity

> Reviewer 必须逐项检查。标记 ★ 的为阻塞项（Block）——发现问题必须在合并前修复。
> 标记方式：`- [x]` 通过，`- [ ]` 未通过（必须在评论中说明原因和修改建议）。

---

## 1. 架构合规

- [ ] ★ `app/` 层仅依赖 `domain/` 中的实体和自身定义的 interface，无直接 import `adapter/` 包
- [ ] ★ `adapter/handler/` 不含业务逻辑；handler 只做：参数解析 → 调用 app use case → 格式化响应
- [ ] ★ `adapter/repo/` 不含业务规则；repo 只做数据存取，不做权限判断或状态计算
- [ ] ★ 无 "legacy vs new" 并存的双路径代码；迁移必须一次性完成，不留过渡态
- [ ] ★ 跨服务调用（调用 lurus-api 等）通过 `internal/pkg/` 中的 client 抽象，不在业务层直接发 HTTP
- [ ] 新增 package 的职责明确，符合 `cmd / internal / domain / app / adapter / lifecycle / pkg` 分层约定
- [ ] 循环依赖检查：`go mod graph` 或 `goimports` 无新增循环依赖

---

## 2. Go 规范

- [ ] ★ 所有接受 context 的调用必须传递上层 context，禁止在业务代码中使用裸 `context.Background()`
- [ ] ★ 外部调用（DB query、Redis、HTTP 请求）必须设置超时，通过 `context.WithTimeout` 或连接池配置实现
- [ ] ★ 无 `_ = fn()` 吞错误；每处错误向上用 `fmt.Errorf("operation desc: %w", err)` 包装，保留错误链
- [ ] ★ 所有 `defer Close()` 调用在资源打开后立即放置，且关闭错误不丢弃（日志记录或返回）
- [ ] ★ goroutine 均有退出机制：通过 context 取消或 done channel 控制生命周期，无孤立 goroutine
- [ ] 无全局可变状态（global var 可接受只读配置常量，不可接受可变业务状态）
- [ ] 接口定义在使用方（consumer）所在 package，不在实现方 package 中定义
- [ ] 错误类型使用 sentinel error（`var ErrXxx = errors.New(...)`）或自定义 error 类型，便于 `errors.Is` 判断
- [ ] 数字类型正确：金额使用 `int64`（分）存储；时间使用 `time.Time`；ID 使用 `string`（UUID）

---

## 3. 安全要求

- [ ] ★ 无 SQL 字符串拼接；所有查询使用参数化（`$1`, `$2`）或 ORM 的参数绑定
- [ ] ★ 无敏感信息明文写入日志：token、密码、完整 DSN、信用卡号、私钥；DSN 中密码必须掩码
- [ ] ★ 所有需要认证的端点均经过 JWT 验证中间件；`/internal/v1/*` 端点经过 `INTERNAL_API_KEY` 验证
- [ ] ★ 无硬编码凭证（API key、密码、secret）；凭证必须来自环境变量或 Secret 挂载
- [ ] ★ 跨用户资源访问有权限校验（防 IDOR）：操作时验证资源 owner_id == current_user_id 或 is_admin
- [ ] Webhook 处理器验证请求签名（Creem HMAC-SHA256、支付宝签名）；签名错误返回 400，不执行业务逻辑
- [ ] 输入验证：字符串长度上限、数值范围、UUID 格式、枚举值合法性均在 handler 层校验
- [ ] 金额运算无浮点精度问题（使用整数分运算，不使用 float64）
- [ ] 返回错误时不暴露内部实现细节（DB 表名、文件路径、堆栈信息）给外部调用方

---

## 4. 性能

- [ ] ★ 无 N+1 查询：循环内不发起 DB 查询；批量操作使用 `IN` 查询或批量 insert
- [ ] ★ 高频读取路径使用 Redis 缓存（权益查询等），缓存键命名规范，TTL 合理
- [ ] ★ 数据库查询有对应索引支持（新增查询条件的字段在 migration 中已建索引）
- [ ] 无大对象在内存中完整加载（分页查询使用 limit；大文件流式处理）
- [ ] 锁粒度最小化：mutex 仅保护共享变量，不包裹 I/O 操作
- [ ] 长时 Cron/批处理任务不阻塞主请求路径（独立 goroutine，有执行超时限制）
- [ ] HTTP 连接池已配置（MaxIdleConns、IdleConnTimeout）；DB 连接池参数已审查
- [ ] 避免频繁的小内存分配：热路径避免 `interface{}` boxing，考虑对象池（`sync.Pool`）

---

## 5. 测试质量

- [ ] ★ `go test ./internal/app/...` 覆盖率 ≥ 80%；PR 描述中附有实际覆盖率输出
- [ ] ★ 测试命名规范：`Test<Subject>_<Method>_<Behavior>`；测试函数名可直接理解其验证目标
- [ ] ★ 边界条件有测试覆盖：nil、空值、零值、极大值、并发调用
- [ ] ★ 错误路径有测试覆盖：DB 失败、Redis 不可用、第三方服务超时
- [ ] Mock 使用合理：只 mock 外部依赖（DB、Redis、外部 API），不 mock 内部业务逻辑
- [ ] 测试之间相互独立：无共享全局状态，测试顺序不影响结果
- [ ] Table-driven test 用于多输入场景，避免重复测试代码
- [ ] 集成测试与单元测试分离（build tag 或独立目录），CI 可选择性运行

---

## 6. 前端规范

> 仅适用于 Epic 10 及前端相关 Story；纯后端 Story 可跳过此节。

- [ ] ★ API 调用统一通过 `api/` 模块发起，不在组件中直接使用 `fetch`/`axios`
- [ ] ★ 所有 API 错误均有用户可见的错误提示；不显示原始 HTTP 错误码或服务器错误信息
- [ ] ★ 用户输入在提交前进行客户端校验（格式、必填、长度）；服务端校验错误在表单上精确显示
- [ ] 无 `dangerouslySetInnerHTML` 使用（除非内容来源可信且已 sanitize）
- [ ] 异步操作有 loading 状态处理，防止重复提交（按钮 disabled 或请求去重）
- [ ] 敏感信息不存入 localStorage；token 存储遵循项目约定（httpOnly cookie 或内存）
- [ ] `bun run build` 无 TypeScript 类型错误，无 lint 错误
- [ ] 新增页面/组件有基本的单元测试（render + 关键交互）

---

## 7. 运维要求

- [ ] ★ 服务支持 graceful shutdown：监听 SIGTERM，停止接受新请求，等待处理中请求完成（超时可配置）
- [ ] ★ 所有新增环境变量在 `.env.example` 中有记录（含默认值和用途注释）；配置项在启动时 fast-fail 校验
- [ ] ★ 新增数据库 migration 文件幂等（`IF NOT EXISTS`、`ON CONFLICT DO NOTHING`）；可在 `BEGIN...COMMIT` 中安全执行
- [ ] K8s manifests 安全基线：`readOnlyRootFilesystem: true`，`runAsNonRoot: true`，`runAsUser: 65534`，`allowPrivilegeEscalation: false`
- [ ] Liveness 和 Readiness probe 配置正确：readiness 检查 DB/Redis 连通性，liveness 仅检查进程存活
- [ ] 日志格式为 JSON（生产）或文本（开发），通过环境变量切换；日志级别可配置
- [ ] 无 `log.Fatal` / `os.Exit` 在非 main 函数中使用；错误向上返回，由 main 决定退出
- [ ] ConfigMap 和 Secret 分离：非敏感配置走 ConfigMap，敏感配置走 Secret

---

## Review 结论

| 项目 | 结果 |
|------|------|
| 阻塞项（★）全部通过 | `[ ] Yes  [ ] No` |
| 非阻塞项问题数 | ___ 个（需在 2 个工作日内修复或记入 backlog） |
| 总体评估 | `[ ] Approve  [ ] Request Changes  [ ] Comment` |

**Reviewer**: ___________
**Review 日期**: ___________
**PR / Story**: ___________

**需要修改的问题清单**（如有）：

1.
2.
3.
