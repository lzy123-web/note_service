# note_service

基于 [tRPC-Go](https://github.com/trpc-group/trpc-go) 的笔记服务 Demo，演示了一个生产级 RPC 服务在 **MongoDB + Redis** 之上的常见缓存最佳实践：**空值缓存防穿透 / TTL 抖动防雪崩 / singleflight 防击穿 / 延迟双删保障一致性**。

## 项目结构

```
note_service/
├── note.proto                           # 协议定义（顶层副本，方便阅读）
└── note/
    ├── main.go                          # 服务入口
    ├── note_service.go                  # 4 个 RPC 的业务实现
    ├── note_service_test.go             # 单元测试骨架（gomock）
    ├── mongo.go                         # MongoDB 数据访问封装
    ├── redis.go                         # Redis 缓存封装（含空值/抖动/双删）
    ├── errs.go                          # 业务错误码常量
    ├── trpc_go.yaml                     # tRPC 框架配置
    ├── cmd/client/main.go               # 简易测试客户端
    └── stub/git.woa.com/trpcprotocol/demo/note/
        ├── note.pb.go                   # protoc 生成
        ├── note.trpc.go                 # trpc-cmdline 生成
        └── note_mock.go                 # mockgen 生成
```

## 接口列表

`service NoteService`（包名 `trpc.demo.note`）：

| RPC          | 入参                                                         | 出参                                | 说明                                                                |
| ------------ | ------------------------------------------------------------ | ----------------------------------- | ------------------------------------------------------------------- |
| `CreateNote` | `user_id` / `title` / `content`                              | `note_id`                           | 校验非空 → 写 Mongo                                                 |
| `GetNote`    | `note_id`                                                    | `Note`                              | 先查 Redis；未命中走 singleflight 回源 Mongo 并回填，DB 不存在写空值缓存 |
| `ListNotes`  | `user_id` / `page` / `page_size`                             | `notes` / `total`                   | 按 `created_at` 倒序分页；`page_size` 越界兜底为 20                 |
| `DeleteNote` | `note_id` / `user_id`                                        | 空                                  | 二次校验所有权 → 删 Mongo → 同步删 cache → 500ms 后异步再删一次     |

完整定义见 [`note.proto`](./note.proto)。

## 缓存策略

`note/redis.go` + `note/note_service.go` 中实现了下列细节：

- **缓存穿透**：DB 查不到时写入 `__NULL__` 空值标记，TTL 60 秒；后续相同 `note_id` 直接返回 `NoteNotFound`。
- **缓存雪崩**：正向缓存 TTL 为 `30min + rand(0, 5min)`，避免大批 key 同时过期。
- **缓存击穿**：用 `golang.org/x/sync/singleflight` 按 `note_id` 维度合并并发回源请求。
- **强一致**：删除接口采用**延迟双删** —— 先删 DB，立即删一次 cache，500ms 后再异步删一次，覆盖 `read-after-write` 竞态窗口。
- **降级**：Redis 自身故障时不阻塞主流程，记 warn 日志后直查 DB。
- **ctx 解耦**：singleflight `fn` 内用 `context.WithoutCancel(ctx)` 剥离上游 cancel/deadline，仅保留 trace、logger 等 value，防止首请求被取消时连带影响并发等待者。

## 错误码

业务错误码在 `note/errs.go`，从 `10000` 起：

| 常量                       | 值      | 含义               |
| -------------------------- | ------- | ------------------ |
| `ErrCodeInvalidParam`      | 10001   | 参数校验失败       |
| `ErrCodeNoteNotFound`      | 10002   | 笔记不存在         |
| `ErrCodePermissionDenied`  | 10003   | 越权操作           |
| `ErrCodeInternal`          | 10004   | 服务器内部错误     |

通过 `errs.New(code, msg)` 返回；调用方用 `errs.Code(err)` / `errs.Msg(err)` 解析。

## 快速启动

### 1. 准备依赖

```bash
# MongoDB
docker run -d --name note-mongo -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=root \
  -e MONGO_INITDB_ROOT_PASSWORD=example \
  mongo:6

# Redis
docker run -d --name note-redis -p 6379:6379 redis:7
```

### 2. 安装 Go 依赖

```bash
cd note
go mod tidy
```

### 3. 启动服务

```bash
cd note
go run .
```

服务监听 `127.0.0.1:8000`（tcp / trpc 协议），配置见 [`note/trpc_go.yaml`](./note/trpc_go.yaml)。

### 4. 调用测试客户端

```bash
cd note
go run ./cmd/client
```

## 配置说明

`note/trpc_go.yaml` 关键段：

```yaml
server.service:
  - name: trpc.demo.note.NoteService
    ip: 127.0.0.1
    port: 8000
    network: tcp
    protocol: trpc
    timeout: 1000

client.service:
  - name: trpc.demo.note.mongodb
    target: mongodb://root:example@127.0.0.1:27017/note_db
  - name: trpc.demo.note.redis
    target: redis://127.0.0.1:6379/0?pool_size=100&min_idle_conns=20&...
```

业务代码通过这两个 `name` 拿到 client proxy，改环境只需改 `target`，代码不动。

## 重新生成桩代码

修改 `note.proto` 后重新生成：

```bash
# 服务桩
trpc create -p note.proto -o note --mock=true --rpconly

# mock（也可以直接 go generate ./...）
cd note
go generate ./...
```

`note/note_service_test.go` 顶部的 `//go:generate` 指令会触发 `mockgen`，把 `note.trpc.go` 里的接口重新生成到 `stub/.../note_mock.go`。

## 运行测试

```bash
cd note
go test ./...
```

> 当前 `note_service_test.go` 是 trpc-cmdline 生成的骨架，`tests` 切片为空，`go test` 会直接 PASS。把真实用例填进去即可启用断言；`noteServiceImpl` 内部的 `mongo` / `cache` 均为零值，写真实用例时建议把它们抽成接口注入 mock，或使用 mtest / miniredis。

## 技术栈

| 类别       | 选型                                                 |
| ---------- | ---------------------------------------------------- |
| RPC 框架   | tRPC-Go                                              |
| 序列化     | Protobuf 3                                           |
| 存储       | MongoDB 6（`go.mongodb.org/mongo-driver`）           |
| 缓存       | Redis 7（`github.com/redis/go-redis/v9`）            |
| 并发控制   | `golang.org/x/sync/singleflight`                     |
| Mock       | `go.uber.org/mock`                                   |
| 日志       | `trpc.group/trpc-go/trpc-go/log`                     |

## License

MIT
