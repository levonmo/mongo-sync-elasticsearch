package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/olivere/elastic"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	infolog  = log.New(os.Stdout, "INFO ", log.Flags())
	errorlog = log.New(os.Stderr, "ERROR ", log.Flags())
)

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

type Config struct {
	MongoDB    string `json:"mongodb"`
	MongoColl  string `json:"mongocoll"`
	MongodbUrl string `json:"mongodburl"`
	EsUrl      string `json:"esurl"`
	EsIndex    string `json:"esindex"`
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
		errorlog.Printf("init config err:%v\n", time.Now().String(), err)
		return
	}
	infolog.Printf(" init config success %+v \n", config)

	//connect elstic mongodb
	esCli, err := elastic.NewClient(elastic.SetURL(config.EsUrl))
	if err != nil {
		errorlog.Printf("connect es  err:%v\n", err)
		return
	}
	infolog.Printf("connect es success\n")
	cli, err := mongo.Connect(context.Background(), options.Client().ApplyURI(config.MongodbUrl))
	if err != nil {
		errorlog.Printf("mongo connect err:%v\n", err)
		return
	}
	defer cli.Disconnect(context.Background())
	infolog.Printf("connect mongodb success\n")

	go func() {
		time.Sleep(time.Second * 60 * 2)
		runtime.GC()
	}()

	//开始同步之前找到最新的ts
	localColl := cli.Database("local").Collection("oplog.rs")
	dbColl := cli.Database(config.MongoDB).Collection(config.MongoColl)
	filter := bson.M{
		"op": bson.M{
			"$in": opCodes,
		},
	}
	opts := &options.FindOneOptions{}
	opts.SetSort(bson.M{"$natural": -1})

	var latestoplog OpLog
	err = localColl.FindOne(context.Background(), filter, opts).Decode(&latestoplog)
	if err != nil {
		errorlog.Printf("find latest oplog.rs err:%v\n", err)
		return
	}
	infolog.Printf("get latest oplog.rs ts: %v", latestoplog.Timestamp)

	//start sync historical data
	coll := cli.Database(config.MongoDB).Collection(config.MongoColl)
	find, err := coll.Find(context.Background(), bson.M{})
	if err != nil {
		errorlog.Printf("find %s err:%v\n", config.MongoDB+"."+config.MongoColl, err)
		return
	}
	infolog.Printf("start sync historical data...")

	syncHisChan := make(chan map[string]interface{}, 10000)
	syncWG := sync.WaitGroup{}

	for i := 0; i < 3; i++ {
		syncWG.Add(1)
		go func(i int) {
			infolog.Printf("sync historical data goroutine: %d start \n", i)
			defer infolog.Printf("sync historical data goroutine: %d exit \n", i)
			defer syncWG.Done()
			bulks := make([]elastic.BulkableRequest, 5000)
			bulk := esCli.Bulk()
			for {
				count := 0
				for i := 0; i < 5000; i++ {
					select {
					case obj, ok := <-syncHisChan:
						if !ok {
							break
						}
						id := obj["_id"].(primitive.ObjectID).Hex()
						delete(obj, "_id")

						bytes, err := json.Marshal(obj)
						if err != nil {
							errorlog.Printf("sync historical data json marshal err:%v id:%s\n", err, id)
							continue
						}
						doc := elastic.NewBulkIndexRequest().Index(config.EsIndex).Type("_doc").Id(id).Doc(string(bytes))
						bulks[count] = doc
						count++
					}
				}

				if count != 0 {
					bulk.Add(bulks[:count]...)
					bulkResponse, err := bulk.Do(context.Background())
					if err != nil {
						errorlog.Printf("batch processing, bulk do err:%v count:%d\n", err, len(bulks))
						//很可能是es挂了，等待十秒再重试
						time.Sleep(time.Second * 10)
						continue
					}
					for _, v := range bulkResponse.Failed() {
						errorlog.Printf("index: %s, type: %s, _id: %s, error: %+v\n", v.Index, v.Type, v.Id, *v.Error)
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
			errorlog.Printf("sync historical data decode %s db err:%v\n", config.MongoDB+"."+config.MongoColl, err)
			break
		}
		syncHisChan <- a
	}
	close(syncHisChan)
	syncWG.Wait()
	errorlog.Printf("sync historical data success")
	// 以上是同步历史数据，下面是同步增量数据

	//todo 每个小时纪录一次最新的oplog

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
		errorlog.Printf("tail oplog.rs based latestoplog timestamp err:%v\n", err)
		return
	}

	infolog.Printf("start sync increment data...")
	syncIncrChan := make(chan ElasticObj, 10000)

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
						errorlog.Printf("batch processing, bulk do err:%v count:%d\n", err, len(bulks))
						bulksLock.Unlock()
						continue
					}
					for _, v := range bulkResponse.Failed() {
						errorlog.Printf("index: %s, type: %s, _id: %s, error: %+v\n", v.Index, v.Type, v.Id, *v.Error)
					}
					bulk.Reset()
					bulks = make([]elastic.BulkableRequest, 0)
					bulksLock.Unlock()
				}
			}
		}()

		for {
			select {
			case obj := <-syncIncrChan:
				doc := elastic.NewBulkIndexRequest().Index(config.EsIndex).Type("_doc").Id(obj.ID).Doc(obj.Obj)
				bulksLock.Lock()
				bulks = append(bulks, doc)
				bulksLock.Unlock()
			}
		}
	}()

	for cursor.Next(context.Background()) {
		o := OpLog{}
		err = cursor.Decode(&o)
		if err != nil {
			errorlog.Printf("tail decode oplog.rs err:%v\n", err)
			continue
		}
		switch o.Operation {
		case "i":
			id := o.Doc["_id"].(primitive.ObjectID).Hex()
			delete(o.Doc, "_id")
			var obj ElasticObj
			obj.ID = id
			obj.Obj = o.Doc
			syncIncrChan <- obj
		case "d":
			id := o.Doc["_id"].(primitive.ObjectID).Hex()
			_, err := esCli.Delete().Index(config.EsIndex).Type("_doc").Id(id).Do(context.Background())
			if err != nil {
				errorlog.Printf("delete document in es err:%v id:%s\n", err, id)
				continue
			}
		case "u":
			id := o.Update["_id"].(primitive.ObjectID).Hex()
			objId, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				errorlog.Printf("objectid id err:%v id:%s\n", err, id)
				continue
			}
			f := bson.M{
				"_id": objId,
			}
			obj := make(map[string]interface{})
			err = dbColl.FindOne(context.Background(), f).Decode(&obj)
			if err != nil {
				errorlog.Printf("find document from mongodb  err:%v id:%s\n", err, id)
				continue
			}
			delete(obj, "_id")
			var elasticObj ElasticObj
			elasticObj.ID = id
			elasticObj.Obj = obj
			syncIncrChan <- elasticObj
		}
	}
}
