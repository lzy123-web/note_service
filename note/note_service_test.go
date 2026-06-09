package main

import (
	"context"
	"testing"

	pb "git.woa.com/trpcprotocol/demo/note"
	"go.uber.org/mock/gomock"
	"trpc.group/trpc-go/trpc-go/errs"
)

//go:generate go mod tidy
//go:generate mockgen -destination=stub/git.woa.com/trpcprotocol/demo/note/note_mock.go -package=note -self_package=git.woa.com/trpcprotocol/demo/note --source=stub/git.woa.com/trpcprotocol/demo/note/note.trpc.go

// ---------- 错误码测试 (T21) ----------

func TestErrCodeValues(t *testing.T) {
	tests := []struct {
		name     string
		code     int32
		expected int32
	}{
		{"ErrCodeInvalidParam", ErrCodeInvalidParam, 10001},
		{"ErrCodeNoteNotFound", ErrCodeNoteNotFound, 10002},
		{"ErrCodePermissionDenied", ErrCodePermissionDenied, 10003},
		{"ErrCodeInternal", ErrCodeInternal, 10004},
		{"ErrCodeVersionConflict", ErrCodeVersionConflict, 10005},
		{"ErrCodeVersionNotFound", ErrCodeVersionNotFound, 10006},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.code, tt.expected)
			}
		})
	}
}

// ---------- noteVersionDoc 结构体测试 (T22) ----------

func TestNoteVersionDocBsonTags(t *testing.T) {
	doc := noteVersionDoc{
		NoteID:    "n1",
		Version:   2,
		UserID:    "u1",
		Title:     "title",
		Content:   "content",
		UpdatedAt: 1000,
	}
	// 验证字段赋值正常（bson tag 通过集成测试验证）
	if doc.NoteID != "n1" || doc.Version != 2 {
		t.Errorf("noteVersionDoc fields not set correctly: %+v", doc)
	}
}

// ---------- UpdateNote 测试 (T23) ----------

func Test_noteServiceImpl_UpdateNote(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().UpdateNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.UpdateNoteRequest) (*pb.UpdateNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.UpdateNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.UpdateNoteRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		errCode int32
	}{
		{
			name:    "missing note_id",
			args:    args{ctx: context.Background(), req: &pb.UpdateNoteRequest{UserId: "u1", Title: "t", ExpectedVersion: 1}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "missing user_id",
			args:    args{ctx: context.Background(), req: &pb.UpdateNoteRequest{NoteId: "n1", Title: "t", ExpectedVersion: 1}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "missing title",
			args:    args{ctx: context.Background(), req: &pb.UpdateNoteRequest{NoteId: "n1", UserId: "u1", ExpectedVersion: 1}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "invalid expected_version",
			args:    args{ctx: context.Background(), req: &pb.UpdateNoteRequest{NoteId: "n1", UserId: "u1", Title: "t", ExpectedVersion: 0}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "note not found (zero mongo/cache)",
			args:    args{ctx: context.Background(), req: &pb.UpdateNoteRequest{NoteId: "nonexist", UserId: "u1", Title: "t", ExpectedVersion: 1}},
			wantErr: true,
			errCode: ErrCodeInternal, // mongo zero value returns error on FindByID
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := noteServiceService.UpdateNote(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateNote() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errCode > 0 {
		if int32(errs.Code(err)) != tt.errCode {
				t.Errorf("UpdateNote() errCode = %d, want %d", errs.Code(err), tt.errCode)
				}
			}
		})
	}
}

// ---------- ListNoteVersions 测试 (T24) ----------

func Test_noteServiceImpl_ListNoteVersions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().ListNoteVersions(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.ListNoteVersionsRequest) (*pb.ListNoteVersionsResponse, error) {
		s := &noteServiceImpl{}
		return s.ListNoteVersions(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.ListNoteVersionsRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		errCode int32
	}{
		{
			name:    "missing note_id",
			args:    args{ctx: context.Background(), req: &pb.ListNoteVersionsRequest{UserId: "u1"}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "missing user_id",
			args:    args{ctx: context.Background(), req: &pb.ListNoteVersionsRequest{NoteId: "n1"}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "note not found (zero mongo)",
			args:    args{ctx: context.Background(), req: &pb.ListNoteVersionsRequest{NoteId: "nonexist", UserId: "u1"}},
			wantErr: true,
			errCode: ErrCodeInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := noteServiceService.ListNoteVersions(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListNoteVersions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errCode > 0 {
		if int32(errs.Code(err)) != tt.errCode {
				t.Errorf("ListNoteVersions() errCode = %d, want %d", errs.Code(err), tt.errCode)
				}
			}
		})
	}
}

// ---------- RestoreNoteVersion 测试 (T25) ----------

func Test_noteServiceImpl_RestoreNoteVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().RestoreNoteVersion(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.RestoreNoteVersionRequest) (*pb.RestoreNoteVersionResponse, error) {
		s := &noteServiceImpl{}
		return s.RestoreNoteVersion(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.RestoreNoteVersionRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		errCode int32
	}{
		{
			name:    "missing note_id",
			args:    args{ctx: context.Background(), req: &pb.RestoreNoteVersionRequest{UserId: "u1", Version: 2, ExpectedVersion: 5}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "missing user_id",
			args:    args{ctx: context.Background(), req: &pb.RestoreNoteVersionRequest{NoteId: "n1", Version: 2, ExpectedVersion: 5}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "invalid version",
			args:    args{ctx: context.Background(), req: &pb.RestoreNoteVersionRequest{NoteId: "n1", UserId: "u1", Version: 0, ExpectedVersion: 5}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "invalid expected_version",
			args:    args{ctx: context.Background(), req: &pb.RestoreNoteVersionRequest{NoteId: "n1", UserId: "u1", Version: 2, ExpectedVersion: 0}},
			wantErr: true,
			errCode: ErrCodeInvalidParam,
		},
		{
			name:    "note not found (zero mongo)",
			args:    args{ctx: context.Background(), req: &pb.RestoreNoteVersionRequest{NoteId: "nonexist", UserId: "u1", Version: 2, ExpectedVersion: 5}},
			wantErr: true,
			errCode: ErrCodeInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := noteServiceService.RestoreNoteVersion(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("RestoreNoteVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errCode > 0 {
		if int32(errs.Code(err)) != tt.errCode {
				t.Errorf("RestoreNoteVersion() errCode = %d, want %d", errs.Code(err), tt.errCode)
				}
			}
		})
	}
}

// ---------- 原有 RPC 测试（保持兼容） ----------

func Test_noteServiceImpl_CreateNote(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().CreateNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.CreateNoteRequest) (*pb.CreateNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.CreateNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.CreateNoteRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rsp *pb.CreateNoteResponse
			var err error
			if rsp, err = noteServiceService.CreateNote(tt.args.ctx, tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("noteServiceImpl.CreateNote() error = %v, wantErr %v", err, tt.wantErr)
			}
			_ = rsp
		})
	}
}

func Test_noteServiceImpl_GetNote(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().GetNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.GetNoteRequest) (*pb.GetNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.GetNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.GetNoteRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rsp *pb.GetNoteResponse
			var err error
			if rsp, err = noteServiceService.GetNote(tt.args.ctx, tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("noteServiceImpl.GetNote() error = %v, wantErr %v", err, tt.wantErr)
			}
			_ = rsp
		})
	}
}

func Test_noteServiceImpl_ListNotes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().ListNotes(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.ListNotesRequest) (*pb.ListNotesResponse, error) {
		s := &noteServiceImpl{}
		return s.ListNotes(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.ListNotesRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rsp *pb.ListNotesResponse
			var err error
			if rsp, err = noteServiceService.ListNotes(tt.args.ctx, tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("noteServiceImpl.ListNotes() error = %v, wantErr %v", err, tt.wantErr)
			}
			_ = rsp
		})
	}
}

func Test_noteServiceImpl_DeleteNote(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	m := noteServiceService.EXPECT().DeleteNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.DeleteNoteRequest) (*pb.DeleteNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.DeleteNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	type args struct {
		ctx context.Context
		req *pb.DeleteNoteRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rsp *pb.DeleteNoteResponse
			var err error
			if rsp, err = noteServiceService.DeleteNote(tt.args.ctx, tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("noteServiceImpl.DeleteNote() error = %v, wantErr %v", err, tt.wantErr)
			}
			_ = rsp
		})
	}
}
