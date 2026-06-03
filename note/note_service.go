package main

import (
	"context"
	"errors"
	"time"

	pb "git.woa.com/trpcprotocol/demo/note"
	"golang.org/x/sync/singleflight"
	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/log"
)

const (
	// 延迟双删的延迟时长。
	// 经验值：要 >= 一次"读 DB + 回填 cache"的耗时，覆盖住 read-after-write 的竞态窗口。
	delayedDoubleDeleteInterval = 500 * time.Millisecond

	// 第二次删 cache 的超时时间，避免请求结束后 goroutine 永远挂着。
	delayedDoubleDeleteTimeout = 2 * time.Second

	// singleflight fn 内的 DB 加载超时。
	sfLoadTimeout = 3 * time.Second
)

// noteServiceImpl 是 NoteService 的实现。
type noteServiceImpl struct {
	pb.UnimplementedNoteService
	mongo *MongoClient
	cache *RedisCache
	// sf 用于合并同一 note_id 的并发回源请求，防缓存击穿。
	// 零值即可用，key 维度是 note_id。
	sf singleflight.Group
}

// 创建笔记
func (s *noteServiceImpl) CreateNote(
	ctx context.Context, req *pb.CreateNoteRequest,
) (*pb.CreateNoteResponse, error) {
	if req.GetUserId() == "" || req.GetTitle() == "" {
		return nil, errs.New(ErrCodeInvalidParam, "user_id and title required")
	}

	now := nowMs()
	doc := &noteDoc{
		NoteID:    genNoteID(),
		UserID:    req.GetUserId(),
		Title:     req.GetTitle(),
		Content:   req.GetContent(),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.mongo.Insert(ctx, doc); err != nil {
		log.ErrorContextf(ctx, "mongo insert fail: %v", err)
		return nil, errs.New(ErrCodeInternal, "db error")
	}

	return &pb.CreateNoteResponse{NoteId: doc.NoteID}, nil
}

// 查询单条笔记
func (s *noteServiceImpl) GetNote(
	ctx context.Context, req *pb.GetNoteRequest,
) (*pb.GetNoteResponse, error) {
	if req.GetNoteId() == "" {
		return nil, errs.New(ErrCodeInvalidParam, "note_id required")
	}

	// step 1: 查 cache
	doc, err := s.cache.GetNote(ctx, req.GetNoteId())
	switch {
	case err == nil:
		// cache hit
		return &pb.GetNoteResponse{Note: docToPB(doc)}, nil
	case errors.Is(err, ErrCacheNegative):
		// 空值缓存命中直接返回not found（缓存穿透）
		return nil, errs.New(ErrCodeNoteNotFound, "note not found")
	case errors.Is(err, ErrCacheMiss):
		// 正常 miss，走 step 2
	default:
		// redis 自身故障
		log.WarnContextf(ctx, "cache get fail, fallthrough to db, err=%v", err)
	}

	// step 2 + 3: 回源 + 回填，用 singleflight 合并同 note_id 的并发请求，防缓存击穿。
	v, err, _ := s.sf.Do(req.GetNoteId(), func() (any, error) {
		// ctx 解耦：保留上游 trace/logger 等 value，但剥离 cancel/deadline。
		fnCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sfLoadTimeout)
		defer cancel()

		doc, err := s.mongo.FindByID(fnCtx, req.GetNoteId())
		if err != nil {
			log.ErrorContextf(fnCtx, "mongo find fail: %v", err)
			return nil, errs.New(ErrCodeInternal, "db error")
		}
		if doc == nil {
			// DB 中也没有，写空值缓存防穿透
			if err := s.cache.SetNegative(fnCtx, req.GetNoteId()); err != nil {
				log.WarnContextf(fnCtx, "cache set negative fail: %v", err)
			}
			return nil, errs.New(ErrCodeNoteNotFound, "note not found")
		}
		// 回填 cache
		if err := s.cache.SetNote(fnCtx, doc); err != nil {
			log.WarnContextf(fnCtx, "cache set fail: %v", err)
		}
		return doc, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetNoteResponse{Note: docToPB(v.(*noteDoc))}, nil
}

// 查询用户笔记列表
func (s *noteServiceImpl) ListNotes(
	ctx context.Context, req *pb.ListNotesRequest,
) (*pb.ListNotesResponse, error) {
	if req.GetUserId() == "" {
		return nil, errs.New(ErrCodeInvalidParam, "user_id required")
	}

	page, size := req.GetPage(), req.GetPageSize()
	if size <= 0 || size > 50 {
		size = 20 // 兜底默认值，防止恶意大 size 拖垮 DB
	}

	docs, total, err := s.mongo.ListByUser(ctx, req.GetUserId(), page, size)
	if err != nil {
		log.ErrorContextf(ctx, "mongo list fail: %v", err)
		return nil, errs.New(ErrCodeInternal, "db error")
	}

	notes := make([]*pb.Note, 0, len(docs))
	for _, d := range docs {
		notes = append(notes, docToPB(d))
	}
	return &pb.ListNotesResponse{Notes: notes, Total: total}, nil
}

// 删除笔记
func (s *noteServiceImpl) DeleteNote(
	ctx context.Context, req *pb.DeleteNoteRequest,
) (*pb.DeleteNoteResponse, error) {
	if req.GetNoteId() == "" || req.GetUserId() == "" {
		return nil, errs.New(ErrCodeInvalidParam, "note_id and user_id required")
	}

	doc, err := s.mongo.FindByID(ctx, req.GetNoteId())
	if err != nil {
		log.ErrorContextf(ctx, "mongo find fail: %v", err)
		return nil, errs.New(ErrCodeInternal, "db error")
	}
	if doc == nil {
		return nil, errs.New(ErrCodeNoteNotFound, "note not found")
	}
	if doc.UserID != req.GetUserId() { // 二次校验 user_id：避免拿到 note_id 就能删别人的笔记
		return nil, errs.New(ErrCodePermissionDenied, "permission denied")
	}

	// 延迟双删
	noteID := req.GetNoteId()
	if _, err := s.mongo.DeleteByID(ctx, noteID); err != nil {
		log.ErrorContextf(ctx, "mongo delete fail: %v", err)
		return nil, errs.New(ErrCodeInternal, "db error")
	}
	// 第一次删 cache：同步执行
	if err := s.cache.DelNote(ctx, noteID); err != nil {
		log.WarnContextf(ctx, "cache del fail (1st): %v", err)
	}
	// 第二次删 cache：异步延迟
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("delayed cache del panic, note_id=%s, recover=%v", noteID, r)
			}
		}()
		time.Sleep(delayedDoubleDeleteInterval)
		bgCtx, cancel := context.WithTimeout(context.Background(), delayedDoubleDeleteTimeout)
		defer cancel()
		if err := s.cache.DelNote(bgCtx, noteID); err != nil {
			log.WarnContextf(bgCtx, "cache del fail (2nd, delayed): note_id=%s, err=%v", noteID, err)
		}
	}()

	return &pb.DeleteNoteResponse{}, nil
}

// docToPB 把内部存储模型转为对外协议结构。
func docToPB(d *noteDoc) *pb.Note {
	if d == nil {
		return nil
	}
	return &pb.Note{
		NoteId:    d.NoteID,
		UserId:    d.UserID,
		Title:     d.Title,
		Content:   d.Content,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}
