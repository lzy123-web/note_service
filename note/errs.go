package main

// 业务错误码集中定义。
//
// tRPC 框架对错误码的约定：
//   - 0      ：成功
//   - 1~999  ：框架错误（连接/超时/序列化等，由框架自动塞入 trpc 协议头）
//   - >=10000：业务错误，由业务自定义
//
// 使用 errs.New(code, msg) 返回的 error，框架会把 code/msg 写入响应头，
// 调用方通过 errs.Code(err) / errs.Msg(err) 拿到。
const (
	ErrCodeInvalidParam     = 10001 // 参数校验失败
	ErrCodeNoteNotFound     = 10002 // 笔记不存在
	ErrCodePermissionDenied = 10003 // 越权操作（如删除别人的笔记）
	ErrCodeInternal         = 10004 // 服务器内部错误（DB/Cache 故障等）
)
