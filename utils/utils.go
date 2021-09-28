package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/conts"
	"github.com/levonmo/mongo-sync-elasticsearch/log"
	"github.com/levonmo/mongo-sync-elasticsearch/model"
	"github.com/levonmo/mongo-sync-elasticsearch/repo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"io/ioutil"
	"os"
)

// 判断所给路径文件/文件夹是否存在
func exists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func IsOpenOplogTsFile() {
	var oplogTsPath string
	if config.GetInstance().CheckPointPath == "" {
		oplogTsPath = "./oplogts"
	} else {
		oplogTsPath = config.GetInstance().CheckPointPath + "/oplogts"
	}
	if config.GetInstance().SyncType != conts.SyncTypeFull {
		if !exists(oplogTsPath) {
			if err := os.Mkdir(oplogTsPath, os.ModePerm); err != nil {
				msg := fmt.Sprintf("os mkdir folder fail,err:%v", err)
				log.Err.Printf(msg)
				panic(msg)
			}
		}
		oplogTsFile := oplogTsPath + "/test_mkdir_" + config.GetInstance().ElasticIndexName + ".log"
		f, err := os.Create(oplogTsFile)
		if err != nil {
			msg := fmt.Sprintf("create file fail in path err:%v", err)
			log.Err.Printf(msg)
			panic(msg)
		}
		f.Close()
		err = os.Remove(oplogTsFile)
		if err != nil {
			msg := fmt.Sprintf("remove file fail in path err:%v", err)
			log.Err.Printf(msg)
			panic(msg)
		}
	}
}

func GetCheckPointFromHistory() *primitive.Timestamp {
	var checkPoint *primitive.Timestamp
	var filePath string
	if config.GetInstance().CheckPointPath == "" {
		filePath = "./oplogts/" + config.GetInstance().ElasticIndexName + "_latestoplog.log"
	} else {
		filePath = config.GetInstance().CheckPointPath + "/oplogts/" + config.GetInstance().ElasticIndexName + "_latestoplog.log"
	}
	if exists(filePath) {
		f, err := os.Open(filePath)
		if err != nil {
			msg := fmt.Sprintf("open oplogfile err:%v", err)
			log.Err.Printf(msg)
			panic(err)
		}
		bytes, err := ioutil.ReadAll(f)
		if err != nil {
			msg := fmt.Sprintf("open oplogfile err:%v", err)
			log.Err.Printf(msg)
			panic(msg)
		}
		var oplogts = model.OplogTimestamp{}
		err = json.Unmarshal(bytes, &oplogts.LatestOplogTimestamp)
		if err != nil {
			msg := fmt.Sprintf("json unmarshal oplogfile err:%v", err)
			log.Err.Printf(msg)
			panic(msg)
		}
		checkPoint = &oplogts.LatestOplogTimestamp
		f.Close()
	}
	return checkPoint
}

func GetNewestOplogTsFromMongo() *primitive.Timestamp {
	var latestoplog *model.OpLog
	filter := model.ValidOps()
	opts := &options.FindOneOptions{}
	opts.SetSort(bson.M{"$natural": -1})
	if err := repo.GetMongoClient().Database("local").Collection("oplog.rs").FindOne(context.Background(), filter, opts).Decode(&latestoplog); err != nil {
		msg := fmt.Sprintf("find latest oplog.rs err:%v", err)
		log.Err.Printf(msg)
		panic(msg)
	}
	return &latestoplog.Timestamp
}
