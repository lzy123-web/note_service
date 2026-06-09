# 设计：笔记版本管理与历史恢复

## Context

当前 `note_service` 是基于 tRPC-Go 的单服务 MongoDB + Redis 笔记服务，提供 4 个基础 RPC（CreateNote / GetNote / ListNotes / DeleteNote）。数据模型为 `noteDoc`（`notes` 集合），缓存策略为延迟双删 + singleflight 防击穿 + 空值缓存防穿透 + TTL 抖动防雪崩。

本变更新增乐观锁版本控制和历史快照能力，涉及 3 个新 RPC、2 个集合变更、2 个新错误码，以及 Proto message 扩展。

### 现有代码关键约束

- `noteDoc` 结构体用于 MongoDB 存储和 Redis JSON 序列化（`redis.go` 的 `SetNote`/`GetNote`），新增字段对缓存反序列化向后兼容（JSON 多字段时旧缓存无 `version` 则零值为 0）。
- `docToPB` 是唯一的内部模型→Proto 转换函数，新增字段只需修改此处。
- 延迟双删逻辑在 `DeleteNote` 中实现，UpdateNote/RestoreNoteVersion 需复用同一模式。
- `MongoClient` 当前所有方法直接操作 `*mongo.Collection`，新增 `note_versions` 集合需要独立的 `coll()` 方法。

## Goals / Non-Goals

**Goals:**

- 通过乐观锁（`expected_version`）防止并发更新覆盖。
- 每次更新/恢复产生不可变历史快照，可查询可恢复。
- 恢复操作产生新版本号，不覆盖旧历史。
- 与现有 RPC 完全向后兼容。
- 复用现有缓存策略（延迟双删、singleflight、空值缓存）。

**Non-Goals:**

- 实时协同编辑、自动合并冲突。
- 软删除和回收站。
- 历史列表 Redis 缓存。
- 分享权限、请求幂等键。
- 生产环境数据迁移脚本。
- DeleteNote 级联删除历史。
- 跨集合事务。

## Decisions

### D1. 乐观锁实现方式：MongoDB `FindOneAndUpdate` 原子操作

**选择**：使用 `FindOneAndUpdate` 配合 filter `{note_id, version: expected_version}`，在单次原子操作中完成版本校验 + 内容更新 + 版本号递增。

**替代方案**：
- (a) 先 `FindOne` 读取当前版本，再 `UpdateOne`：非原子，两步之间版本可能变化，存在 TOCTOU 竞态。
- (b) MongoDB 事务：跨集合事务在分片场景下性能差，且本服务为单集合原子操作，无需事务。

**理由**：`FindOneAndUpdate` 是 MongoDB 单文档级别的原子操作，天然避免 TOCTOU 问题，且 `FindOneAndUpdate` 可返回更新前的文档（`options.ReturnDocument.Before`），直接作为历史快照源，省去一次额外读取。

### D2. 一致性策略：先写历史再更新主文档

**选择**：UpdateNote/RestoreNoteVersion 执行顺序为：
1. 读取当前文档（`FindByID`）。
2. 将当前文档快照写入 `note_versions` 集合。
3. `FindOneAndUpdate` 更新 `notes` 集合（含版本校验）。

**替代方案**：
- (a) 先更新主文档再写历史：若步骤 2 失败，已更新但无历史，违反"有更新必有历史"。
- (b) MongoDB 事务包裹：跨集合事务引入额外复杂度，分片环境下有性能风险。
- (c) 补偿写入（历史写失败时记日志后异步重试）：增加复杂度，本阶段不值得。

**理由**："先写历史再更新"保证：若历史写入失败 → 主文档未变，数据一致；若历史写入成功但更新失败 → 多一条冗余历史，不影响一致性。冗余历史可容忍（查询时按 version 过滤，多余记录不影响结果）。

**注意**：步骤 1 读取和步骤 3 更新之间仍存在竞态窗口——步骤 1 读到的版本可能不是最新的。但步骤 3 的 `FindOneAndUpdate` 带 `version: expected_version` 条件，如果版本已变则更新失败返回 `ErrCodeVersionConflict`，由客户端重试。这是乐观锁的标准用法。

### D3. 历史存储粒度：完整快照

**选择**：每次更新/恢复时，将旧版本的 `note_id`、`version`、`user_id`、`title`、`content`、`updated_at` 完整写入 `note_versions` 集合。

**替代方案**：
- (a) 增量 diff：节省存储但查询恢复需回放全链，实现复杂。
- (b) 只存 title + content：丢失 updated_at 等元数据，ListNoteVersions 需额外信息。

**理由**：MongoDB 存储成本可接受，完整快照实现简单、查询直接、恢复只需读一条记录。

### D4. 历史快照来源：FindOneAndUpdate 返回 Before 文档

**选择**：UpdateNote 使用 `FindOneAndUpdate` 返回更新前文档，直接作为历史快照；步骤 1 的 `FindByID` 仅用于所有权校验和预检查。

**替代方案**：
- (a) 依赖步骤 1 的 `FindByID` 结果作为快照源：步骤 1 和步骤 3 之间可能被其他请求修改，快照可能不是实际被替换的版本。

**理由**：`FindOneAndUpdate` 返回的 Before 文档是实际被替换的版本，保证历史快照与实际更新一一对应。即使步骤 2 写入的历史版本号与步骤 3 的 `FindOneAndUpdate` 实际更新的版本号不一致（因并发），`note_versions` 唯一索引会拒绝重复，且 `FindOneAndUpdate` 失败时历史已写入但可容忍。

**修正方案**（综合 D2 + D4）：
1. `FindByID`：校验所有权 + 笔记存在性。
2. 写入 `note_versions`：将 `FindByID` 返回的当前文档作为预写历史（version 值来自当前文档）。
3. `FindOneAndUpdate`：带 `version: expected_version` 条件更新。
   - 成功：返回 Before 文档。若 Before 文档的 version 与步骤 2 写入的历史 version 一致 → 完美。
   - 若不一致（理论上不会，因为步骤 1 读到的就是 expected_version）：冗余历史可容忍。
   - 失败（版本不匹配）：返回 VersionConflict，历史记录为冗余但无害。

### D5. 版本号策略

**选择**：`int32` 单调递增，创建时 `version=1`，每次更新/恢复 +1。

**替代方案**：
- (a) `version=0` 起步：语义不直观（v0 不是真正的版本）。
- (b) 使用时间戳作版本号：不保证单调递增（时钟回拨），且不利于人类理解。

**理由**：整数版本号简单直观，`FindOneAndUpdate` 的 `$inc: {version: 1}` 原子递增，天然避免版本号冲突。

### D6. Redis 缓存失效策略

**选择**：UpdateNote/RestoreNoteVersion 复用 `DeleteNote` 的延迟双删模式——同步删一次 + 500ms 后异步再删一次。

**理由**：
- 更新后缓存已过时，必须失效。
- 延迟双删覆盖 read-after-write 竞态窗口（更新请求删缓存期间，另一读请求可能回填旧值）。
- GetNote 回填时 `noteDoc` 已含 `version` 字段，无需额外逻辑。

**注意**：延迟双删需提取为公共函数（当前逻辑内嵌在 `DeleteNote` 中）。

### D7. ListNoteVersions 不加缓存

**选择**：本阶段 `ListNoteVersions` 直接查 MongoDB，不加 Redis 缓存。

**理由**：历史版本列表访问频率远低于单条笔记详情，缓存收益不大；历史写入后不可变，但分页查询的缓存失效策略复杂度高于收益。后续如需优化可单独加缓存。

### D8. DeleteNote 不级联删除历史

**选择**：`DeleteNote` 只删 `notes` 集合文档，不删 `note_versions` 中的历史记录。

**理由**：历史记录作为审计轨迹保留；级联删除增加 `DeleteNote` 的复杂度和故障面；本阶段保持简单。

## 8 维声明

### 安全

- 所有新 RPC 均校验 `user_id` 与笔记所有权（`noteDoc.UserID != req.GetUserId()` → `ErrCodePermissionDenied`）。
- `expected_version` 校验防止并发篡改，间接防止"覆盖他人修改"。
- 输入校验：`note_id`、`user_id` 非空；`title` 非空；`version > 0`；`page_size` 上限 50。
- 无 SQL 注入风险（MongoDB 使用参数化查询）。
- 无 XSS 风险（RPC 接口不渲染 HTML）。

### 契约（API 兼容性）

- Proto 变更：
  - `Note` message 新增 `version int32 = 7`（字段号 7，不破坏现有字段）。
  - 新增 3 个 rpc 方法 + 6 个 message 类型。
  - 现有 4 个 RPC 的请求/响应 message 不变。
- 向后兼容：
  - 旧客户端不传 `version` 字段（proto3 默认零值），`GetNote`/`ListNotes` 返回中多一个 `version` 字段，旧客户端忽略未知字段，无影响。
  - `docToPB` 增加 `Version` 字段映射。
  - Redis 缓存中旧格式 JSON（无 `version` 字段）反序列化后 `version=0`（Go 零值），对旧数据无影响；更新后写入的缓存含 `version` 字段。

### 数据

- `notes` 集合：`noteDoc` 增加 `version int32` 字段。现有文档 `version` 为零值（0），GetNote 返回 `version=0`。新创建笔记 `version=1`。
- `note_versions` 集合（新增）：`noteVersionDoc` 结构，字段包括 `note_id`、`version`、`user_id`、`title`、`content`、`updated_at`。
- 索引：`note_versions` 集合添加复合唯一索引 `{note_id: 1, version: -1}`，保证每篇笔记的版本号唯一。
- 一致性：采用"先写历史再更新主文档"策略（见 D2），不使用跨集合事务。冗余历史可容忍，缺失历史不可容忍。

### 回滚

- 代码回滚：新 RPC 是增量添加，回滚只需移除新增代码和 Proto 定义，现有 RPC 不受影响。
- 数据回滚：`notes` 集合新增的 `version` 字段回滚后为零值，不影响旧逻辑。`note_versions` 集合可整个丢弃。
- 缓存：回滚后旧版本代码不写 `version` 字段到缓存，但旧缓存中可能有 `version` 字段——JSON 反序列化多字段自动忽略，无影响。
- Proto：旧客户端不识别新字段自动忽略；新客户端连旧服务端时新 RPC 返回 `Unimplemented` 错误。

### 迁移

- **本阶段不提供生产环境数据迁移脚本**（Non-goal）。
- 开发环境：可直接清库重建，或手动为现有文档添加 `version: 1`。
- `note_versions` 集合在首次写入时自动创建（MongoDB 特性）。
- 索引需手动创建或通过代码在服务启动时 `EnsureIndex`。

### 监控

- 新增 RPC 的调用量、延迟、错误率由 tRPC 框架自动采集。
- 关键业务指标建议监控：
  - `UpdateNote` 版本冲突率（`ErrCodeVersionConflict` 占比）：过高可能暗示前端重试逻辑缺失或并发度过高。
  - `ListNoteVersions` 查询延迟：MongoDB 查询性能基线。
  - `RestoreNoteVersion` 调用频率：异常高频可能暗示误操作过多。
- 日志：版本冲突、历史写入失败、缓存删除失败等关键路径已有 `log.WarnContextf`/`log.ErrorContextf`。

### 性能

- `FindOneAndUpdate`：单文档原子操作，性能等同于普通 `UpdateOne`，无额外开销。
- `note_versions` 插入：单次 `InsertOne`，与主文档更新解耦，不增加更新操作的关键路径延迟（先写历史，但历史写入通常 <5ms）。
- `ListNoteVersions`：`note_versions` 集合按 `{note_id: 1, version: -1}` 索引查询，分页性能 O(1)（索引覆盖）。
- 存储增长估算：每篇笔记每更新一次增加一条历史记录（完整快照），假设平均笔记 1KB、每篇更新 10 次，10 万篇笔记约 1GB 历史数据，MongoDB 可承受。
- 无 N+1 查询风险。

### 权限

- 3 个新 RPC 均校验 `user_id` 与笔记所有权，与 `DeleteNote` 的二次校验模式一致。
- `ListNoteVersions`：校验 `note_id` 所属 `user_id` == 请求 `user_id`，否则返回 `ErrCodePermissionDenied`。
- `RestoreNoteVersion`：同上，并校验目标版本是否存在。
- 当前无 RBAC / 角色体系，权限控制基于 `user_id` 所有权。

## Risks / Trade-offs

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 历史写入失败但主文档未更新（先写历史策略下） | 用户看到错误提示，无数据不一致 | 返回 `ErrCodeInternal`，客户端可重试 |
| 历史写入成功但主文档更新失败 | 产生一条冗余历史记录 | 可容忍，查询时按实际版本号过滤；可后续定期清理 |
| `FindOneAndUpdate` 版本冲突频繁 | 用户体验差（频繁重试） | 客户端实现指数退避重试；监控冲突率 |
| `note_versions` 集合无限增长 | 存储成本上升 | 本阶段不处理；后续可加 TTL 索引或定期归档 |
| 现有文档 `version=0`（Go 零值） | GetNote 返回 `version=0` 与新建 `version=1` 不一致 | 文档说明；Non-goal 中排除数据迁移 |
| `DeleteNote` 后历史记录残留 | 孤儿历史数据占存储 | 可容忍；后续可加定期清理任务 |
| 延迟双删提取为公共函数时引入 bug | 缓存一致性被破坏 | 充分单元测试；复用现有已验证逻辑，仅提取不修改 |

## Open Questions

无。（所有设计决策已在提案确认阶段明确，默认值已采纳。）
