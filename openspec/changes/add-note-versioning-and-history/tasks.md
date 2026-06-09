# 实施任务：笔记版本管理与历史恢复

## Proto 与代码生成

- [x] T1: 修改 `note.proto` — `Note` message 增加 `version int32 = 7`；新增 `UpdateNote`/`ListNoteVersions`/`RestoreNoteVersion` 三个 rpc 及其 request/response message
- [ ] T2: 运行 `trpc create -p note.proto -o note --mock=true --rpconly` 重新生成桩代码（需本地 tRPC 环境）
- [ ] T3: 运行 `cd note && go generate ./...` 重新生成 mock（需本地 Go 环境）

## 数据模型

- [x] T4: `note/mongo.go` — `noteDoc` 结构体增加 `Version int32` 字段（bson tag: `version`）
- [x] T5: `note/mongo.go` — 新增 `noteVersionDoc` 结构体，包含 `NoteID`/`Version`/`UserID`/`Title`/`Content`/`UpdatedAt`
- [x] T6: `note/mongo.go` — 新增 `versionColl()` 方法，返回 `note_versions` 集合
- [x] T7: `note/mongo.go` — 新增 `InsertVersion(ctx, *noteVersionDoc) error` 方法
- [x] T8: `note/mongo.go` — 新增 `FindVersion(ctx, noteID string, version int32) (*noteVersionDoc, error)` 方法
- [x] T9: `note/mongo.go` — 新增 `ListVersionsByNoteID(ctx, noteID string, page, pageSize int32) ([]*noteVersionDoc, int64, error)` 方法，按 version 倒序分页
- [x] T10: `note/mongo.go` — 新增 `FindOneAndUpdateVersion(ctx, noteID string, expectedVersion int32, title, content string) (*noteDoc, error)` 方法，原子校验版本 + 更新 + 返回 Before 文档
- [x] T11: `note/mongo.go` — 新增 `EnsureIndexes(ctx) error` 方法，创建 `note_versions` 集合的复合唯一索引 `{note_id: 1, version: -1}`

## 错误码

- [x] T12: `note/errs.go` — 新增 `ErrCodeVersionConflict = 10005` 和 `ErrCodeVersionNotFound = 10006`

## 缓存

- [x] T13: `note/redis.go` 或 `note/note_service.go` — 提取延迟双删为公共函数 `delayedDoubleDelete(ctx context.Context, noteID string)`，复用 `DeleteNote` 中的逻辑（同步删 + 500ms 后异步删）
- [x] T14: 重构 `DeleteNote` 使用 `delayedDoubleDelete` 替代内嵌的双删逻辑

## 业务实现

- [x] T15: `note/note_service.go` — `CreateNote` 中 `noteDoc` 初始化 `Version: 1`
- [x] T16: `note/note_service.go` — `docToPB` 增加 `Version` 字段映射
- [x] T17: `note/note_service.go` — 实现 `UpdateNote` RPC：参数校验 → 查笔记校验所有权 → 写历史快照到 `note_versions` → `FindOneAndUpdate` 更新主文档（含版本校验）→ 延迟双删缓存
- [x] T18: `note/note_service.go` — 实现 `ListNoteVersions` RPC：参数校验 → 查笔记校验所有权 → `ListVersionsByNoteID` 分页查询 → 转换为 Proto 返回
- [x] T19: `note/note_service.go` — 实现 `RestoreNoteVersion` RPC：参数校验 → 查笔记校验所有权 → 查目标版本是否存在 → 写当前版本历史快照 → `FindOneAndUpdate` 恢复内容（含版本校验）→ 延迟双删缓存
- [x] T20: `note/main.go` — 服务启动时调用 `EnsureIndexes` 确保 `note_versions` 索引存在

## 测试

- [x] T21: `note/errs.go` 测试 — 验证新增错误码值正确
- [x] T22: `note/mongo.go` 测试 — 验证 `noteVersionDoc` 结构体字段和 bson tag
- [x] T23: `note/note_service_test.go` — UpdateNote 单元测试：正常更新、版本冲突、权限拒绝、笔记不存在、参数校验
- [x] T24: `note/note_service_test.go` — ListNoteVersions 单元测试：正常分页、权限拒绝、笔记不存在、默认 page_size、上限 50、空历史
- [x] T25: `note/note_service_test.go` — RestoreNoteVersion 单元测试：正常恢复、版本冲突、版本不存在、权限拒绝、笔记不存在
- [ ] T26: 运行 `cd note && gofmt -w . && go test ./...` 确保编译通过且现有测试不受影响（需本地 Go 环境）

## 文档与客户端

- [x] T27: 更新 `note/cmd/client/main.go` — 新增 `callNoteServiceUpdateNote`、`callNoteServiceListNoteVersions`、`callNoteServiceRestoreNoteVersion` 示例函数，并在 `main` 中调用
- [x] T28: 更新 `README.md` — 接口列表增加 3 个新 RPC、错误码表增加 2 个新错误码、缓存策略说明增加 UpdateNote/RestoreNoteVersion 的双删描述
