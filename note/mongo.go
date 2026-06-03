package main

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"trpc.group/trpc-go/trpc-database/mongodb"
)

// 数据库 / 集合常量
const (
	mongoDB   = "note_db"
	mongoColl = "notes"
)

// noteDoc 是 MongoDB 中存储的笔记文档结构
type noteDoc struct {
	NoteID    string `bson:"note_id"    json:"note_id"`
	UserID    string `bson:"user_id"    json:"user_id"`
	Title     string `bson:"title"      json:"title"`
	Content   string `bson:"content"    json:"content"`
	CreatedAt int64  `bson:"created_at" json:"created_at"`
	UpdatedAt int64  `bson:"updated_at" json:"updated_at"`
}

// MongoClient 封装 trpc-database/mongodb 的 client proxy。
type MongoClient struct {
	proxy mongodb.Client
}

// NewMongoClient 创建一个 MongoClient。
func NewMongoClient(name string) *MongoClient {
	return &MongoClient{proxy: mongodb.NewClientProxy(name)}
}

// genNoteID 用 Mongo 的 ObjectID 生成 24 字符 hex 字符串作为业务主键。
func genNoteID() string {
	return primitive.NewObjectID().Hex()
}

// nowMs 返回当前时间的 unix 毫秒。
func nowMs() int64 {
	return time.Now().UnixMilli()
}

// coll 拿到 trpc-database/mongodb 封装好的 *mongo.Collection。
func (m *MongoClient) coll(ctx context.Context) (*mongo.Collection, error) {
	return m.proxy.Collection(ctx, mongoDB, mongoColl)
}

// Insert 插入一条新笔记。
func (m *MongoClient) Insert(ctx context.Context, n *noteDoc) error {
	coll, err := m.coll(ctx)
	if err != nil {
		return err
	}
	_, err = coll.InsertOne(ctx, n)
	return err
}

// FindByID 按 note_id 查询单条。
func (m *MongoClient) FindByID(ctx context.Context, noteID string) (*noteDoc, error) {
	coll, err := m.coll(ctx)
	if err != nil {
		return nil, err
	}
	var doc noteDoc
	err = coll.FindOne(ctx, bson.M{"note_id": noteID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// ListByUser 按 user_id 分页查询，按 created_at 倒序。
func (m *MongoClient) ListByUser(
	ctx context.Context, userID string, page, pageSize int32,
) ([]*noteDoc, int64, error) {
	coll, err := m.coll(ctx)
	if err != nil {
		return nil, 0, err
	}
	filter := bson.M{"user_id": userID}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	skip := int64((page - 1) * pageSize)
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(pageSize))

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	out := make([]*noteDoc, 0, pageSize)
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// DeleteByID 按 note_id 删除一条；返回受影响行数供上层判断"目标确实存在"。
func (m *MongoClient) DeleteByID(ctx context.Context, noteID string) (int64, error) {
	coll, err := m.coll(ctx)
	if err != nil {
		return 0, err
	}
	res, err := coll.DeleteOne(ctx, bson.M{"note_id": noteID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
