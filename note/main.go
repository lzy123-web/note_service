package main

import (
	pb "git.woa.com/trpcprotocol/demo/note"
	_ "trpc.group/trpc-go/trpc-filter/debuglog"
	_ "trpc.group/trpc-go/trpc-filter/recovery"
	trpc "trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/log"
)

func main() {
	// trpc.NewServer() 必须放在 main 第一行：
	//   - 内部会读取 trpc_go.yaml
	//   - 加载日志 / 配置 / registry 等插件
	//   - 创建 service 对象
	s := trpc.NewServer()

	// 创建业务实现对象
	impl := &noteServiceImpl{
		mongo: NewMongoClient("trpc.demo.note.mongodb"),
		cache: NewRedisCache("trpc.demo.note.redis"),
	}

	// 把 impl 注册到指定 service name；
	pb.RegisterNoteServiceService(s.Service("trpc.demo.note.NoteService"), impl)

	// Serve 阻塞，直到返回错误就会打印fatal日志并退出
	if err := s.Serve(); err != nil {
		log.Fatal(err)
	}
}
