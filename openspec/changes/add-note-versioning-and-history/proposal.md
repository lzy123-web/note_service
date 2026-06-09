# 提案：为 note_service 增加笔记版本管理与历史恢复能力

## 1. 为什么需要这个功能？（背景与问题）

当前 `note_service` 仅提供基础 CRUD（CreateNote / GetNote / ListNotes / DeleteNote），但缺少版本控制，导致以下问题：

- **无历史追溯**：用户误改笔记后无法查看或恢复到之前的版本。
- **并发覆盖丢失**：采用"最后写入获胜"模式，两个客户端同时编辑同一篇笔记时，先提交者的修改会被静默覆盖，无任何告警。
- **无审计轨迹**：无法回溯一篇笔记的完整修改时间线和内容变化。

在协作编辑或频繁修改的个人笔记场景中，版本管理是核心需求。本变更为笔记服务增加轻量级乐观锁版本控制，提供历史追溯和恢复能力。

## 2. 不做会怎样？（不做的影响）

- 并发编辑覆盖丢失会持续发生，用户数据安全无保障。
- 误操作后无恢复手段，只能靠人工备份。
- 随着用户量增长，因并发冲突导致的数据丢失投诉会上升。
- 本阶段是最低成本的版本控制方案（乐观锁 + 完整快照），延迟实施的边际成本只会更高。

## 3. 我们要做什么？（目标）

在现有 `NoteService` 中新增三个 RPC，并调整数据模型和错误码：

### 3.1 UpdateNote — 乐观锁更新

- 入参：`note_id`、`user_id`、`title`、`content`、`expected_version`。
- 仅当当前版本号 == `expected_version` 时才执行更新（乐观锁）。
- 更新成功后版本号 +1，`updated_at` 刷新为当前时间。
- 将旧版本（更新前的完整快照）写入不可变历史集合 `note_versions`。
- 更新后使 Redis 缓存失效，复用现有延迟双删策略。
- 版本冲突时返回明确的 `ErrCodeVersionConflict` 错误。
- 校验笔记所有权：非本人笔记返回 `ErrCodePermissionDenied`。

### 3.2 ListNoteVersions — 查看历史版本列表

- 入参：`note_id`、`user_id`、`page`、`page_size`。
- 只能查询属于当前用户的笔记历史。
- 按 `version` 倒序分页，`page_size` 默认 20，上限 50。
- 返回历史版本摘要列表（`version`、`title`、`updated_at`）及总数。
- 本阶段不增加历史列表 Redis 缓存（直接查 MongoDB）。

### 3.3 RestoreNoteVersion — 恢复指定历史版本

- 入参：`note_id`、`user_id`、`version`（要恢复的目标版本号）、`expected_version`（当前版本号）。
- 恢复操作不覆盖旧历史，而是产生一个新版本：当前 v5，恢复 v2 → 产生 v6（内容来自 v2）。
- 同样校验 `expected_version`，避免并发覆盖。
- 校验笔记所有权和历史版本是否存在。
- 恢复后使 Redis 缓存失效（复用延迟双删策略）。

### 3.4 数据模型调整

- `notes` 集合（现有）：`noteDoc` 增加 `version int32` 字段，新创建笔记 `version=1`。
- 新增 `note_versions` 集合：存储不可变的历史版本快照，包含 `note_id`、`version`、`user_id`、`title`、`content`、`updated_at`。
- `note_versions` 集合添加复合唯一索引 `{note_id: 1, version: -1}`，保证每篇笔记的版本号唯一。

### 3.5 错误码扩展

- `ErrCodeVersionConflict = 10005`：版本冲突（expected_version 不匹配）。
- `ErrCodeVersionNotFound = 10006`：历史版本不存在。
- 现有 RPC（CreateNote / GetNote / ListNotes / DeleteNote）行为不变，保持向后兼容。

### 3.6 测试与文档

- 补充 UpdateNote / ListNoteVersions / RestoreNoteVersion 的单元测试。
- 更新客户端示例代码。
- 更新 README。
- 运行 `gofmt` 和 `go test ./...`，确保现有测试不受影响。

## 4. 最小可行范围（MVP）

本变更的最小可行范围 = 3 个新 RPC + 乐观锁 + 完整快照历史。只要这三个 RPC 能跑通，用户就能：
1. 安全地更新笔记（乐观锁防并发覆盖）。
2. 查看历史版本列表。
3. 恢复到任意历史版本。

## 5. 对现有能力的影响面

| 现有能力 | 影响分析 |
|---------|---------|
| CreateNote | `noteDoc` 增加 `version` 字段，创建时设为 1；`docToPB` 增加 `Version` 映射；RPC 签名不变 |
| GetNote | `docToPB` 增加 `Version` 映射；Redis 缓存的序列化/反序列化因 `noteDoc` 新增字段自动兼容（JSON 无额外字段时为零值）；RPC 签名不变 |
| ListNotes | 同 GetNote，`docToPB` 增加字段；RPC 签名不变 |
| DeleteNote | 删除笔记时是否级联删除历史？本阶段选择**不级联删除**（历史记录保留），保持简单 |
| 缓存策略 | UpdateNote / RestoreNoteVersion 写后复用延迟双删；GetNote 回填时 `noteDoc` 已含 `version`，无需额外逻辑 |
| Proto | `Note` message 增加 `version int32 = 7`；新增 3 组 request/response message 和 3 个 rpc 方法 |

## 6. 一致性风险分析

**核心风险：笔记更新与历史记录写入之间的一致性**

当前方案中，UpdateNote 需要依次执行：
1. 更新 `notes` 集合（含版本号校验 + 内容更新，原子操作 via `FindOneAndUpdate`）
2. 插入 `note_versions` 集合（历史快照写入）

**风险场景**：
- 步骤 1 成功但步骤 2 失败：笔记已更新到新版本，但旧版本历史丢失。
- 步骤 2 成功但步骤 1 回滚（理论上 MongoDB 单文档操作不会回滚，此处无风险）。

**处理方案**：
- 采用 **"先写历史，再更新"** 策略：先把当前版本快照写入 `note_versions`，再执行 `FindOneAndUpdate` 更新 `notes` 集合。
- 如果写历史成功但更新失败，多出一条历史记录但不影响一致性（历史集合本身允许冗余）。
- 如果写历史失败，直接返回错误，不执行更新，确保"有更新必有历史"。
- 不使用 MongoDB 事务（跨集合事务在分片场景下有性能风险），以最终一致性语义换取可用性。
- `note_versions` 的唯一索引 `{note_id, version}` 防止重复写入。

**补充说明**：RestoreNoteVersion 同理，先写历史（当前版本快照），再更新主文档。

## 7. 明确不做（Non-goals）

- 实时协同编辑
- 自动合并冲突
- 软删除和回收站
- 历史列表 Redis 缓存
- 分享权限
- 请求幂等键
- 生产环境数据迁移脚本
- 级联删除历史（DeleteNote 时不删 note_versions）
- 历史版本内容 diff 存储（本阶段用完整快照）

## 8. 设计决策摘要（需确认）

| 决策项 | 默认选择 | 理由 |
|-------|---------|------|
| 初始版本号 | `version=1`（创建时即为 v1） | 语义清晰，更新后变 v2 |
| 历史集合名 | `note_versions` | 与 `notes` 主集合对应 |
| 历史存储粒度 | 完整快照（title + content + updated_at + version） | 实现简单、查询直接，MongoDB 存储成本可接受 |
| updated_at 更新策略 | 每次更新/恢复时自动刷新为当前时间 | 符合语义预期 |
| 分页参数 | 沿用 `page` / `page_size`（与 ListNotes 一致） | API 风格统一 |
| 历史可删除 | 否，历史不可变 | 保持简单，审计完整性 |
| 一致性策略 | 先写历史再更新主文档，无跨集合事务 | 最终一致性，可用性优先 |
| DeleteNote 级联删除历史 | 否 | 保持简单，历史作为审计记录保留 |
