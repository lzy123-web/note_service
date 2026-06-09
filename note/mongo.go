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
	mongoDB      = "note_db"
	mongoColl    = "notes"
	mongoVerColl = "note_versions"
)

// noteDoc 是 MongoDB 中存储的笔记文档结构
type noteDoc struct {
	NoteID    string `bson:"note_id"    json:"note_id"`
	UserID    string `bson:"user_id"    json:"user_id"`
	Title     string `bson:"title"      json:"title"`
	Content   string `bson:"content"    json:"content"`
	CreatedAt int64  `bson:"created_at" json:"created_at"`
	UpdatedAt int64  `bson:"updated_at" json:"updated_at"`
	Version   int32  `bson:"version"    json:"version"`
}

// noteVersionDoc 是 note_versions 集合中不可变的历史版本快照。
type noteVersionDoc struct {
	NoteID    string `bson:"note_id"    json:"note_id"`
	Version   int32  `bson:"version"    json:"version"`
	UserID    string `bson:"user_id"    json:"user_id"`
	Title     string `bson:"title"      json:"title"`
	Content   string `bson:"content"    json:"content"`
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

// versionColl 拿到 note_versions 集合。
func (m *MongoClient) versionColl(ctx context.Context) (*mongo.Collection, error) {
	return m.proxy.Collection(ctx, mongoDB, mongoVerColl)
}

// InsertVersion 向 note_versions 集合插入一条不可变的历史快照。
func (m *MongoClient) InsertVersion(ctx context.Context, v *noteVersionDoc) error {
	coll, err := m.versionColl(ctx)
	if err != nil {
		return err
	}
	_, err = coll.InsertOne(ctx, v)
	return err
}

// FindVersion 按 note_id + version 查询单条历史版本。
func (m *MongoClient) FindVersion(ctx context.Context, noteID string, version int32) (*noteVersionDoc, error) {
	coll, err := m.versionColl(ctx)
	if err != nil {
		return nil, err
	}
	var doc noteVersionDoc
	err = coll.FindOne(ctx, bson.M{"note_id": noteID, "version": version}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// ListVersionsByNoteID 按 note_id 分页查询历史版本，按 version 倒序。
func (m *MongoClient) ListVersionsByNoteID(
	ctx context.Context, noteID string, page, pageSize int32,
) ([]*noteVersionDoc, int64, error) {
	coll, err := m.versionColl(ctx)
	if err != nil {
		return nil, 0, err
	}
	filter := bson.M{"note_id": noteID}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	skip := int64((page - 1) * pageSize)
	opts := options.Find().
		SetSort(bson.D{{Key: "version", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(pageSize))

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	out := make([]*noteVersionDoc, 0, pageSize)
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// FindOneAndUpdateVersion 原子操作：仅当 current version == expectedVersion 时更新 title/content，
// version +1，updated_at 刷新，并返回更新前的文档快照。
func (m *MongoClient) FindOneAndUpdateVersion(
	ctx context.Context, noteID string, expectedVersion int32, title, content string,
) (*noteDoc, error) {
	coll, err := m.coll(ctx)
	if err != nil {
		return nil, err
	}
	filter := bson.M{
		"note_id": noteID,
		"version": expectedVersion,
	}
	update := bson.M{
		"$set": bson.M{
			"title":      title,
			"content":    content,
			"updated_at": nowMs(),
			"version":    expectedVersion + 1,
		},
	}
	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.Before)

	var before noteDoc
	err = coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&before)
	if err != nil {
		return nil, err
	}
	return &before, nil
}

// EnsureIndexes 确保 note_versions 集合的必要索引存在。
func (m *MongoClient) EnsureIndexes(ctx context.Context) error {
	coll, err := m.versionColl(ctx)
	if err != nil {
		return err
	}
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "note_id", Value: 1},
			{Key: "version", Value: -1},
		},
		Options: options.Index().SetUnique(true),
	})
	return err
}
