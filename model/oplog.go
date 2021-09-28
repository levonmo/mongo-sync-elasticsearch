package model

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func ValidOps() bson.M {
	return bson.M{"op": bson.M{"$in": OpCodes}}
}

var OpCodes = [...]string{"c", "i", "u", "d"}

type OpLog struct {
	Timestamp    primitive.Timestamp    "ts"
	HistoryID    int64                  "h"
	MongoVersion int                    "v"
	Operation    string                 "op"
	Namespace    string                 "ns"
	Doc          map[string]interface{} "o"
	Update       map[string]interface{} "o2"
}

type MongoDoc struct {
	Op     string
	Doc    map[string]interface{}
	Update map[string]interface{}
}

type OplogTimestamp struct {
	LatestOplogTimestamp primitive.Timestamp `json:"latest_oplog_timestamp"`
}
