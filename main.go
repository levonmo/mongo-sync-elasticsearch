package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/olivere/elastic"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var infoLog = log.New(os.Stdout, "INFO ", log.Flags())
var warnLog = log.New(os.Stdout, "WARN ", log.Flags())
var statsLog = log.New(os.Stdout, "STATS ", log.Flags())
var traceLog = log.New(os.Stdout, "TRACE ", log.Flags())
var errorLog = log.New(os.Stderr, "ERROR ", log.Flags())

func validOps() bson.M {
	return bson.M{"op": bson.M{"$in": opCodes}}
}

var opCodes = [...]string{"c", "i", "u", "d"}

type OpLog struct {
	Timestamp    primitive.Timestamp    "ts"
	HistoryID    int64                  "h"
	MongoVersion int                    "v"
	Operation    string                 "op"
	Namespace    string                 "ns"
	Doc          map[string]interface{} "o"
	Update       map[string]interface{} "o2"
}

type ElasticObj struct {
	ID  string
	Obj map[string]interface{}
}

type OplogTimestamp struct {
	LatestOplogTimestamp primitive.Timestamp `json:"latest_oplog_timestamp"`
}

type Config struct {
	MongoDB    string `json:"mongodb"`
	MongoColl  string `json:"mongocoll"`
	MongodbUrl string `json:"mongodburl"`
	EsUrl      string `json:"esurl"`
	Tspath     string `json:"tspath"`
}

func InitConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	bytes := make([]byte, 1024)
	n, err := file.Read(bytes)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(bytes[:n], &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	filePath := ""
	argsCount := len(os.Args)
	if argsCount == 1 {
		fmt.Println("-h/-help for help")
		return
	} else if argsCount == 2 {
		if os.Args[1] == "-h" || os.Args[1] == "-help" {
			fmt.Println(` -f + config.json: es: ./main -f /data/config.json`)
		} else {
			fmt.Println("-h/-help for help")
		}
		return
	} else if argsCount == 3 {
		if os.Args[1] == "-f" {
			filePath = os.Args[2]
		}
	} else {
		fmt.Println("-h/-help for help")
		return
	}

	//init config
	config, err := InitConfig(filePath)
	if err != nil {
		errorLog.Printf("init config err:%v\n", err)
		return
	}
	infoLog.Printf(" init config success %+v \n", config)

	//创建oplogts文件夹
	var oplogts string
	if config.Tspath == "" {
		oplogts = "./oplogts"
	} else {
		oplogts = config.Tspath + "/oplogts"
	}
	if !Exists(oplogts) {
		err = os.Mkdir(oplogts, os.ModePerm)
		if err != nil {
			errorLog.Printf("os mkdir folder fail,err:%v", err)
			return
		}
	}

	//尝试有没有打开tspath权限
	var testoplogFile string
	if config.Tspath == "" {
		testoplogFile = "./oplogts/test_mongodb_sync_es.log"
	} else {
		testoplogFile = config.Tspath + "/oplogts/" + "test_mongodb_sync_es.log"
	}
	f, err := os.Create(testoplogFile)
	if err != nil {
		errorLog.Printf("create file fail in tspath err:%v", err)
		return
	}
	f.Close()
	err = os.Remove(testoplogFile)
	if err != nil {
		errorLog.Printf("remove file fail in tspath err:%v", err)
		return
	}

	//连接es和mongodb
	esCli, err := elastic.NewClient(elastic.SetURL(config.EsUrl))
	if err != nil {
		errorLog.Printf("connect es  err:%v\n", err)
		return
	}
	infoLog.Printf("connect es success\n")
	cli, err := mongo.Connect(context.Background(), options.Client().ApplyURI(config.MongodbUrl))
	if err != nil {
		errorLog.Printf("mongo connect err:%v\n", err)
		return
	}
	defer cli.Disconnect(context.Background())
	infoLog.Printf("connect mongodb success\n")

	go func() {
		for {
			time.Sleep(time.Second * 60 * 2)
			runtime.GC()
		}
	}()

	localColl := cli.Database("local").Collection("oplog.rs")
	dbColl := cli.Database(config.MongoDB).Collection(config.MongoColl)

	//获取latestoplog
	// 1.从mongodb数据库中获取
	// 2.从oplog日志文件获取

	var oplogFile string
	if config.Tspath == "" {
		oplogFile = "./oplogts/" + config.MongoDB + "_" + config.MongoColl + "_latestoplog.log"
	} else {
		oplogFile = config.Tspath + "/oplogts/" + config.MongoDB + "_" + config.MongoColl + "_latestoplog.log"
	}
	exists := Exists(oplogFile)
	var latestoplog OpLog
	if !exists {
		//开始同步之前找到最新的ts
		filter := validOps()
		opts := &options.FindOneOptions{}
		opts.SetSort(bson.M{"$natural": -1})
		err = localColl.FindOne(context.Background(), filter, opts).Decode(&latestoplog)
		if err != nil {
			errorLog.Printf("find latest oplog.rs err:%v\n", err)
			return
		}
		infoLog.Printf("get latest oplog.rs ts: %v", latestoplog.Timestamp)

		//进行全量同步
		coll := cli.Database(config.MongoDB).Collection(config.MongoColl)
		find, err := coll.Find(context.Background(), bson.M{})
		if err != nil {
			errorLog.Printf("find %s err:%v\n", config.MongoDB+"."+config.MongoColl, err)
			return
		}
		infoLog.Printf("start sync historical data...")

		mapchan := make(chan map[string]interface{}, 10000)
		syncg := sync.WaitGroup{}

		for i := 0; i < 3; i++ {
			syncg.Add(1)
			go func(i int) {
				infoLog.Printf("sync historical data goroutine: %d start \n", i)
				defer infoLog.Printf("sync historical data goroutine: %d exit \n", i)
				defer syncg.Done()
				bulks := make([]elastic.BulkableRequest, 5000)
				bulk := esCli.Bulk()
				for {
					count := 0
					for i := 0; i < 5000; i++ {
						select {
						case obj, ok := <-mapchan:
							if !ok {
								break
							}
							id := obj["_id"].(primitive.ObjectID).Hex()
							delete(obj, "_id")
							bytes, err := json.Marshal(obj)
							if err != nil {
								errorLog.Printf("sync historical data json marshal err:%v id:%s\n", err, id)
								continue
							}
							doc := elastic.NewBulkIndexRequest().Index(config.MongoDB + "." + config.MongoColl).Type("_doc").Id(id).Doc(string(bytes))
							bulks[count] = doc
							count++
						}
					}

					if count != 0 {
						bulk.Add(bulks[:count]...)
						bulkResponse, err := bulk.Do(context.Background())
						if err != nil {
							errorLog.Printf("batch processing, bulk do err:%v count:%d\n", err, len(bulks))
							//很可能是es挂了，等待十秒再重试
							time.Sleep(time.Second * 10)
							continue
						}
						for _, v := range bulkResponse.Failed() {
							errorLog.Printf("index: %s, type: %s, _id: %s, error: %+v\n", v.Index, v.Type, v.Id, *v.Error)
						}
						bulk.Reset()
						count = 0
					} else {
						break
					}
				}
			}(i)
		}
		for {
			ok := find.Next(context.Background())
			if !ok {
				break
			}
			a := make(map[string]interface{})
			err = find.Decode(&a)
			if err != nil {
				errorLog.Printf("sync historical data decode %s db err:%v\n", config.MongoDB+"."+config.MongoColl, err)
				break
			}
			mapchan <- a
		}
		close(mapchan)
		syncg.Wait()
		infoLog.Printf("sync historical data success")
		// 全量数据同步完成
		f, err := os.Create(oplogFile)
		if err != nil {
			errorLog.Printf("os create oplog file err:%v", err)
			return
		}
		var op OplogTimestamp
		op.LatestOplogTimestamp = latestoplog.Timestamp
		bytes, err := json.Marshal(op)
		if err != nil {
			errorLog.Printf("oplog json marshal err:%v,ts:%v", err, latestoplog)
		}
		_, err = f.Write(bytes)
		if err != nil {
			errorLog.Printf("oplog file writer err:%v", err)
			return
		}
		err = f.Close()
		if err != nil {
			return
		}
	} else {
		f, err := os.Open(oplogFile)
		if err != nil {
			errorLog.Printf("open oplogfile err:%v", err)
			return
		}
		bytes, err := ioutil.ReadAll(f)
		if err != nil {
			errorLog.Printf("open oplogfile err:%v", err)
			return
		}
		var oplogts = OplogTimestamp{}
		err = json.Unmarshal(bytes, &oplogts)
		if err != nil {
			errorLog.Printf("json unmarshal oplogfile err:%v", err)
			return
		}
		latestoplog.Timestamp = oplogts.LatestOplogTimestamp
		infoLog.Printf("get oplog ts success from file,ts:%v", latestoplog.Timestamp)
		f.Close()
	}

	//根据上面获取的ts，开始重放oplog
	query := bson.M{
		"ts":          bson.M{"$gte": latestoplog.Timestamp},
		"op":          bson.M{"$in": opCodes},
		"ns":          config.MongoDB + "." + config.MongoColl,
		"fromMigrate": bson.M{"$exists": false},
	}

	optss := &options.FindOptions{}
	optss.SetSort(bson.M{"$natural": 1})
	optss.SetCursorType(options.TailableAwait)

	cursor, err := localColl.Find(context.Background(), query, optss)
	if err != nil {
		errorLog.Printf("tail oplog.rs based latestoplog timestamp err:%v\n", err)
		return
	}

	infoLog.Printf("start sync increment data...")
	insertes := make(chan ElasticObj, 10000)

	go func() {
		bulk := esCli.Bulk()
		bulks := make([]elastic.BulkableRequest, 0)
		bulksLock := sync.Mutex{}
		go func() {
			for {
				select {
				case <-time.After(time.Second):
					if len(bulks) == 0 {
						continue
					}
					bulksLock.Lock()
					bulk.Add(bulks...)
					bulkResponse, err := bulk.Do(context.Background())
					if err != nil {
						errorLog.Printf("batch processing, bulk do err:%v count:%d\n", err, len(bulks))
						bulksLock.Unlock()
						continue
					}
					for _, v := range bulkResponse.Failed() {
						errorLog.Printf("index: %s, type: %s, _id: %s, error: %+v\n", v.Index, v.Type, v.Id, *v.Error)
					}
					bulk.Reset()
					bulks = make([]elastic.BulkableRequest, 0)
					bulksLock.Unlock()
				}
			}
		}()

		for {
			select {
			case obj := <-insertes:

				//各个库适配
				switch config.MongoDB + "." + config.MongoColl {
				case "translation_wx.job":
					obj.Obj = FixTranslationWXJob(obj.Obj)
				case "translation.job":
					obj.Obj = FixTranslationJob(obj.Obj)
				case "ksodcapiapp.job":
					obj.Obj = FixKsodcapiappJob(obj.Obj)
				case "ksowebdcapiapp.job":
					obj.Obj = FixKsowebdcapiappJob(obj.Obj)

				}
				doc := elastic.NewBulkIndexRequest().Index(config.MongoDB + "." + config.MongoColl).Type("_doc").Id(obj.ID).Doc(obj.Obj)
				bulksLock.Lock()
				bulks = append(bulks, doc)
				bulksLock.Unlock()
			}
		}
	}()

	var ts OplogTimestamp

	//每个小时纪录一次最新的oplog
	go func() {
		for {
			time.Sleep(time.Second * 60 * 60)
			if ts.LatestOplogTimestamp.T > latestoplog.Timestamp.T {
				f, err := os.Create(oplogFile)
				if err != nil {
					errorLog.Printf("os create oplog file err:%v", err)
					return
				}
				bytes, err := json.Marshal(ts)
				if err != nil {
					errorLog.Printf("oplog json marshal err:%v,ts:%v", err, ts)
				}
				_, err = f.Write(bytes)
				if err != nil {
					errorLog.Printf("oplog file writer err:%v", err)
					return
				}
				err = f.Close()
				if err != nil {
					return
				}
			}
		}
	}()

	for cursor.Next(context.Background()) {
		o := OpLog{}
		err = cursor.Decode(&o)
		if err != nil {
			errorLog.Printf("tail decode oplog.rs err:%v\n", err)
			continue
		}
		ts.LatestOplogTimestamp = o.Timestamp
		switch o.Operation {
		case "i":
			id := o.Doc["_id"].(primitive.ObjectID).Hex()
			delete(o.Doc, "_id")
			var obj ElasticObj
			obj.ID = id
			obj.Obj = o.Doc
			insertes <- obj
		case "d":
			id := o.Doc["_id"].(primitive.ObjectID).Hex()
			_, err := esCli.Delete().Index(config.MongoDB + "." + config.MongoColl).Type("_doc").Id(id).Do(context.Background())
			if err != nil {
				errorLog.Printf("delete document in es err:%v id:%s\n", err, id)
				continue
			}
		case "u":
			id := o.Update["_id"].(primitive.ObjectID).Hex()
			objId, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				errorLog.Printf("objectid id err:%v id:%s\n", err, id)
				continue
			}
			f := bson.M{
				"_id": objId,
			}
			obj := make(map[string]interface{})
			err = dbColl.FindOne(context.Background(), f).Decode(&obj)
			if err != nil {
				errorLog.Printf("find document from mongodb  err:%v id:%s\n", err, id)
				continue
			}
			delete(obj, "_id")
			var elasticObj ElasticObj
			elasticObj.ID = id
			elasticObj.Obj = obj
			insertes <- elasticObj
		}
	}
}


// 判断所给路径文件/文件夹是否存在
func Exists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
