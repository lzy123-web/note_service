package main

import (
	"context"
	"reflect"
	"testing"

	pb "git.woa.com/trpcprotocol/demo/note"
	"go.uber.org/mock/gomock"
	_ "trpc.group/trpc-go/trpc-go/http"
)

//go:generate go mod tidy
//go:generate mockgen -destination=stub/git.woa.com/trpcprotocol/demo/note/note_mock.go -package=note -self_package=git.woa.com/trpcprotocol/demo/note --source=stub/git.woa.com/trpcprotocol/demo/note/note.trpc.go

func Test_noteServiceImpl_CreateNote(t *testing.T) {
	// Start writing mock logic.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	// Expected behavior.
	m := noteServiceService.EXPECT().CreateNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.CreateNoteRequest) (*pb.CreateNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.CreateNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	// Start writing unit test logic.
	type args struct {
		ctx context.Context
		req *pb.CreateNoteRequest
		rsp *pb.CreateNoteResponse
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
			if !reflect.DeepEqual(rsp, tt.args.rsp) {
				t.Errorf("noteServiceImpl.CreateNote() rsp got = %v, want %v", rsp, tt.args.rsp)
			}
		})
	}
}

func Test_noteServiceImpl_GetNote(t *testing.T) {
	// Start writing mock logic.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	// Expected behavior.
	m := noteServiceService.EXPECT().GetNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.GetNoteRequest) (*pb.GetNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.GetNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	// Start writing unit test logic.
	type args struct {
		ctx context.Context
		req *pb.GetNoteRequest
		rsp *pb.GetNoteResponse
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
			if !reflect.DeepEqual(rsp, tt.args.rsp) {
				t.Errorf("noteServiceImpl.GetNote() rsp got = %v, want %v", rsp, tt.args.rsp)
			}
		})
	}
}

func Test_noteServiceImpl_ListNotes(t *testing.T) {
	// Start writing mock logic.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	// Expected behavior.
	m := noteServiceService.EXPECT().ListNotes(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.ListNotesRequest) (*pb.ListNotesResponse, error) {
		s := &noteServiceImpl{}
		return s.ListNotes(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	// Start writing unit test logic.
	type args struct {
		ctx context.Context
		req *pb.ListNotesRequest
		rsp *pb.ListNotesResponse
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
			if !reflect.DeepEqual(rsp, tt.args.rsp) {
				t.Errorf("noteServiceImpl.ListNotes() rsp got = %v, want %v", rsp, tt.args.rsp)
			}
		})
	}
}

func Test_noteServiceImpl_DeleteNote(t *testing.T) {
	// Start writing mock logic.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	noteServiceService := pb.NewMockNoteServiceService(ctrl)
	var inorderClient []any
	// Expected behavior.
	m := noteServiceService.EXPECT().DeleteNote(gomock.Any(), gomock.Any()).AnyTimes()
	m.DoAndReturn(func(ctx context.Context, req *pb.DeleteNoteRequest) (*pb.DeleteNoteResponse, error) {
		s := &noteServiceImpl{}
		return s.DeleteNote(ctx, req)
	})
	gomock.InOrder(inorderClient...)

	// Start writing unit test logic.
	type args struct {
		ctx context.Context
		req *pb.DeleteNoteRequest
		rsp *pb.DeleteNoteResponse
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
			if !reflect.DeepEqual(rsp, tt.args.rsp) {
				t.Errorf("noteServiceImpl.DeleteNote() rsp got = %v, want %v", rsp, tt.args.rsp)
			}
		})
	}
}
