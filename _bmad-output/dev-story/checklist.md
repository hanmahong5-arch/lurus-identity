# Definition of Done — lurus-identity

> 每个 Story 提交前，开发者必须逐项确认。未通过所有必填项（★）不得标记 Done。
> 标记方式：`- [x]` 已确认，`- [ ]` 未完成或不适用（需注明原因）。

---

## 1. 代码质量

- [ ] ★ 无硬编码魔法数字、字符串、URL、端口、超时值；所有可变值提取为常量或配置项
- [ ] ★ 无 `_ = fn()` 吞错误；每处错误均用 `fmt.Errorf("context: %w", err)` 包装后向上传递
- [ ] ★ 所有对外暴露的函数和类型有 godoc 注释；注释语言为英文
- [ ] ★ 无 dead code、无注释掉的旧代码块、无 TODO 残留（已追踪到 backlog 的除外并注明 issue）
- [ ] ★ 代码经过 `gofmt` 格式化；`go vet ./...` 无 warning
- [ ] `golangci-lint run` 无新增 lint 错误（相比 main 分支）
- [ ] 函数单一职责：单个函数不超过 60 行，复杂逻辑提取为子函数并命名清晰
- [ ] 并发代码：goroutine 有明确退出机制（`select` + done channel 或 context 取消）；无 goroutine 泄漏

---

## 2. 测试覆盖

- [ ] ★ `go test ./internal/app/...` 覆盖率 ≥ 80%（新增代码不得降低已有覆盖率基线）
- [ ] ★ 新增业务逻辑有对应单元测试；测试命名遵循 `Test<Subject>_<Method>_<Behavior>`
- [ ] ★ 边界条件已测试：nil 输入、空字符串、零值、超大值、并发调用
- [ ] ★ 错误路径已测试：外部依赖失败（DB down、Redis 不可用、第三方 API 超时）时的行为
- [ ] Repository 层有集成测试或 mock 测试，覆盖率 ≥ 60%
- [ ] Handler 层有 HTTP 测试（`httptest`），覆盖核心请求/响应路径，覆盖率 ≥ 50%
- [ ] 测试运行命令已验证并记录实际输出（粘贴到 PR 描述或 process.md）

---

## 3. 接口设计

- [ ] ★ `app/` 层依赖仅通过 Go interface 注入，未直接导入 `adapter/` 包或 Gin/HTTP 类型
- [ ] ★ API 请求/响应结构体字段扁平化，无不必要的嵌套；Optional 字段明确标注 `omitempty`
- [ ] ★ 所有接口出入参在 handler 层边界校验（必填字段、格式、长度），内部代码信任已校验数据
- [ ] ★ 错误响应格式统一：`{"code": "ERR_XXX", "message": "...", "details": {}}`；message 说清发生了什么/期望什么/如何处理
- [ ] 新增端点已在 API 文档（或 OpenAPI spec）中更新
- [ ] 分页接口使用游标或 offset+limit，响应含 `total` 或 `has_more` 字段
- [ ] 破坏性变更（字段重命名/删除）已评估对 lurus-api identity_client.go 的影响

---

## 4. 安全检查

- [ ] ★ 所有受保护端点通过 JWT 中间件验证；`/internal/v1/*` 通过 `INTERNAL_API_KEY` Bearer 验证
- [ ] ★ 无 SQL 拼接；所有数据库查询使用参数化查询（`$1`, `$2` 占位符）
- [ ] ★ 无敏感信息写入日志（密码、token、信用卡号、完整 DSN）；DSN 日志中掩码处理
- [ ] ★ 外部输入（用户 ID、金额、数量）的上下界已校验；金额使用整数分（int64）存储，无浮点运算
- [ ] Webhook 端点验证签名（Creem HMAC、支付宝签名）；签名验证失败返回 400
- [ ] 权限检查：操作他人资源时验证当前 actor 是否具备权限（防越权 IDOR）
- [ ] 依赖无已知高危 CVE：`go list -m all` 与 `govulncheck ./...` 无高危告警

---

## 5. 可观测性

- [ ] ★ 关键操作（认证成功/失败、订阅变更、支付结果、钱包扣减）输出结构化 JSON 日志，包含字段：`timestamp`、`actor_id`、`action`、`resource_type`、`resource_id`、`result`
- [ ] ★ 错误日志包含足够上下文（request_id、user_id、error chain），可凭日志独立排查问题
- [ ] ★ 新增业务指标已在 Prometheus handler 中注册（计数器/直方图），并附说明注释
- [ ] 日志不含个人身份信息（PII）的明文；用户邮箱等字段哈希或截断处理
- [ ] 长时操作（Cron、批量处理）在开始/结束时输出进度日志（处理数量、耗时）

---

## 6. 文档更新

- [ ] ★ 新增或变更的公开端点已更新 `doc/api/` 中对应文档（或注明无文档目录时在 PR 中描述）
- [ ] ★ 新增环境变量已更新 `.env.example`（含默认值和说明注释）
- [ ] ★ `doc/process.md` 追加本次变更的极简摘要（≤ 15 行，含验证命令和实际输出）
- [ ] 如有架构决策变更，已更新 `doc/decisions/` 下对应 ADR 或新建 ADR
- [ ] README 功能列表已更新（如新增用户可见功能）

---

## 7. 部署就绪

- [ ] ★ `CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o app ./cmd/server` 编译通过，无 warning
- [ ] ★ 所有新增配置项在进程启动阶段校验（缺失或非法则 fast-fail，输出明确错误信息）
- [ ] ★ 新增数据库变更已写成幂等 migration 文件（`BEGIN ... COMMIT`，可重复执行）；migration 文件已加入 `scripts/` 且编号正确
- [ ] K8s manifests 已更新（如新增 Secret、ConfigMap key、环境变量）
- [ ] 服务支持 graceful shutdown：收到 SIGTERM 后停止接受新请求，完成处理中的请求后退出（超时可配置）
- [ ] 健康检查端点（`/health/live`、`/health/ready`）正确反映服务状态（DB、Redis 连通性）

---

## 验证记录（必填）

Story ID: ___________
完成日期: ___________

```
# 必须粘贴实际命令输出（不可省略）
$ go test -v ./internal/app/... -count=1 2>&1 | tail -20
<paste actual output here>

$ go build ./cmd/server
<paste actual output here>
```

遗留问题（如有）：
- 描述遗留问题并注明追踪方式（backlog item ID 或 GitHub issue）

确认人: ___________
