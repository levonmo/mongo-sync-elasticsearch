package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"mongo-sync-elasticsearch/config"
	"mongo-sync-elasticsearch/log"
	"mongo-sync-elasticsearch/model"
	"mongo-sync-elasticsearch/service"
	"mongo-sync-elasticsearch/utils"

	"github.com/olivere/elastic"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func validOps() bson.M {
	return bson.M{"op": bson.M{"$in": opCodes}}
}

var opCodes = [...]string{"c", "i", "u", "d"}

type OplogTimestamp struct {
	LatestOplogTimestamp primitive.Timestamp `json:"latest_oplog_timestamp"`
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	filePath := config.GetConfigPath()
	if strings.TrimSpace(filePath) == "" {
		molog.Errorf("config path is illegal")
		return
	}
	config := config.InitConfig(filePath)
	var opLogTs string
	if config.Tspath == "" {
		opLogTs = "./oplogts"
	} else {
		opLogTs = config.Tspath + "/oplogts"
	}

	//尝试有没有打开tspath权限
	var testoplogFile string
	switch config.SyncType {
	case service.SyncTypeDefault, service.SyncTypeIncr:
		if !utils.Exists(opLogTs) {
			if err := os.Mkdir(opLogTs, os.ModePerm); err != nil {
				molog.Errorf("os mkdir folder fail,err:%v", err)
				return
			}
		}
		if config.Tspath == "" {
			testoplogFile = "./oplogts/test_mongodb_sync_es.log"
		} else {
			testoplogFile = config.Tspath + "/oplogts/" + "test_mongodb_sync_es.log"
		}
		f, err := os.Create(testoplogFile)
		if err != nil {
			molog.Errorf("create file fail in tspath err:%v", err)
			return
		}
		f.Close()
		err = os.Remove(testoplogFile)
		if err != nil {
			molog.Errorf("remove file fail in tspath err:%v", err)
			return
		}
	case service.SyncTypeFull:
	}

	//连接es和mongodb
	esCli, err := elastic.NewClient(elastic.SetSniff(false), elastic.SetURL(config.EsUrl))
	if err != nil {
		molog.Errorf("connect es  err:%v", err)
		return
	}
	molog.Infoln("connect es success")
	cli, err := mongo.Connect(context.Background(), options.Client().ApplyURI(config.MongodbUrl))
	if err != nil {
		molog.Errorf("mongo connect err:%v", err)
		return
	}
	defer cli.Disconnect(context.Background())
	molog.Infoln("connect mongodb success")

	localColl := cli.Database("local").Collection("oplog.rs")
	dbColl := cli.Database(config.MongoDB).Collection(config.MongoColl)

	//获取latestoplog
	// 1.从mongodb数据库中获取
	// 2.从oplog日志文件获取
	var oplogFile string
	if config.Tspath == "" {
		oplogFile = "./oplogts/" + config.ElasticIndex + "_latestoplog.log"
	} else {
		oplogFile = config.Tspath + "/oplogts/" + config.ElasticIndex + "_latestoplog.log"
	}
	exists := utils.Exists(oplogFile)

	var latestoplog model.OpLog
	if !exists || config.SyncType == service.SyncTypeFull {
		if config.SyncType != service.SyncTypeFull {
			//开始同步之前找到最新的ts
			filter := validOps()
			opts := &options.FindOneOptions{}
			opts.SetSort(bson.M{"$natural": -1})
			err = localColl.FindOne(context.Background(), filter, opts).Decode(&latestoplog)
			if err != nil {
				molog.Errorf("find latest oplog.rs err:%v", err)
				return
			}
			molog.Infof("get latest oplog.rs ts: %v", latestoplog.Timestamp)
		}

		//进行全量同步
		coll := cli.Database(config.MongoDB).Collection(config.MongoColl)
		find, err := coll.Find(context.Background(), bson.M{})
		if err != nil {
			molog.Errorf("find %s err:%v", config.MongoDB+"."+config.MongoColl, err)
			return
		}
		molog.Infoln("start sync historical data...")

		mapchan := make(chan map[string]interface{}, 10000)
		syncg := sync.WaitGroup{}

		// 统计同步了给
		for i := 0; i < 3; i++ {
			syncg.Add(1)
			go func(i int) {
				molog.Infof("sync historical data goroutine: %d start ", i)
				defer molog.Infof("sync historical data goroutine: %d exit ", i)
				defer syncg.Done()
				bulks := make([]elastic.BulkableRequest, service.MaxFullSyncBatchInsertCount)
				bulk := esCli.Bulk()
				for {
					count := 0
					for i := 0; i < service.MaxFullSyncBatchInsertCount; i++ {
						select {
						case obj, ok := <-mapchan:
							if !ok {
								break
							}
							var id string
							idTypeOf := reflect.TypeOf(obj["_id"])
							switch idTypeOf.String() {
							case "primitive.ObjectID":
								id = obj["_id"].(primitive.ObjectID).Hex()
							case "string":
								id = obj["_id"].(string)
							case "int32":
								id = strconv.Itoa(int(obj["_id"].(int32)))
							case "int64":
								id = strconv.Itoa(int(obj["_id"].(int64)))
							}
							delete(obj, "_id")
							bytes, err := json.Marshal(obj)
							if err != nil {
								molog.Errorf("sync historical data json marshal err:%v id:%s", err, id)
								continue
							}
							doc := elastic.NewBulkIndexRequest().Index(config.ElasticIndex).Id(id).Doc(string(bytes))
							bulks[count] = doc
							count++
						}
					}

					if count != 0 {
						bulk.Add(bulks[:count]...)
						bulkResponse, err := bulk.Do(context.Background())
						if err != nil {
							molog.Errorf("batch processing, bulk do err:%v count:%d", err, len(bulks))
							//很可能是es挂了，等待十秒再重试
							time.Sleep(time.Second * 10)
							continue
						}
						for _, v := range bulkResponse.Failed() {
							molog.Errorf("index: %s, type: %s, _id: %s, error: %+v", v.Index, v.Type, v.Id, *v.Error)
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
				molog.Errorf("sync historical data decode %s db err:%v", config.MongoDB+"."+config.MongoColl, err)
				break
			}
			mapchan <- a
		}
		close(mapchan)
		syncg.Wait()
		molog.Infoln("sync historical data success")
		if config.SyncType == service.SyncTypeFull {
			return
		}
		// 全量数据同步完成
		f, err := os.Create(oplogFile)
		if err != nil {
			molog.Errorf("os create oplog file err:%v", err)
			return
		}
		var op OplogTimestamp
		op.LatestOplogTimestamp = latestoplog.Timestamp
		bytes, err := json.Marshal(op)
		if err != nil {
			molog.Errorf("oplog json marshal err:%v,ts:%v", err, latestoplog)
		}
		_, err = f.Write(bytes)
		if err != nil {
			molog.Errorf("oplog file writer err:%v", err)
			return
		}
		err = f.Close()
		if err != nil {
			return
		}
	} else {
		f, err := os.Open(oplogFile)
		if err != nil {
			molog.Errorf("open oplogfile err:%v", err)
			return
		}
		bytes, err := ioutil.ReadAll(f)
		if err != nil {
			molog.Errorf("open oplogfile err:%v", err)
			return
		}
		var oplogts = OplogTimestamp{}
		err = json.Unmarshal(bytes, &oplogts)
		if err != nil {
			molog.Errorf("json unmarshal oplogfile err:%v", err)
			return
		}
		latestoplog.Timestamp = oplogts.LatestOplogTimestamp
		molog.Infof("get oplog ts success from file,ts:%v", latestoplog.Timestamp)
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
		molog.Errorf("tail oplog.rs based latestoplog timestamp err:%v", err)
		return
	}

	molog.Infoln("start sync increment data...")
	incrSyncChan := make(chan model.MongoDoc, service.MaxIncrSyncBatchInsertCount)

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
						molog.Errorf("batch processing, bulk do err:%v count:%d", err, len(bulks))
						bulksLock.Unlock()
						continue
					}
					for _, v := range bulkResponse.Failed() {
						molog.Errorf("index: %s, type: %s, _id: %s, error: %+v", v.Index, v.Type, v.Id, *v.Error)
					}
					bulk.Reset()
					bulks = make([]elastic.BulkableRequest, 0)
					bulksLock.Unlock()
				}
			}
		}()

		for {
			select {
			case obj := <-incrSyncChan:
				doc := elastic.NewBulkIndexRequest().Index(config.ElasticIndex).Id(obj.ID).Doc(obj.Doc)
				bulksLock.Lock()
				bulks = append(bulks, doc)
				bulksLock.Unlock()
			}
		}
	}()

	var ts OplogTimestamp

	go func() {
		for {
			// 每20s同步一次到数据库 checkoutpoint
			time.Sleep(time.Second * 20)
			if ts.LatestOplogTimestamp.T > latestoplog.Timestamp.T {
				f, err := os.Create(oplogFile)
				if err != nil {
					molog.Errorf("os create oplog file err:%v", err)
					return
				}
				bytes, err := json.Marshal(ts)
				if err != nil {
					molog.Errorf("oplog json marshal err:%v,ts:%v", err, ts)
				}
				_, err = f.Write(bytes)
				if err != nil {
					molog.Errorf("oplog file writer err:%v", err)
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
		o := model.OpLog{}
		err = cursor.Decode(&o)
		if err != nil {
			molog.Errorf("tail decode oplog.rs err:%v", err)
			continue
		}
		ts.LatestOplogTimestamp = o.Timestamp
		var id string
		idTypeOf := reflect.TypeOf(o.Doc["_id"])
		switch idTypeOf.String() {
		case "primitive.ObjectID":
			id = o.Doc["_id"].(primitive.ObjectID).Hex()
		case "string":
			id = o.Doc["_id"].(string)
		case "int32":
			id = strconv.Itoa(int(o.Doc["_id"].(int32)))
		case "int64":
			id = strconv.Itoa(int(o.Doc["_id"].(int64)))
		}
		switch o.Operation {
		case "i":
			delete(o.Doc, "_id")
			var doc model.MongoDoc
			doc.ID = id
			doc.Doc = o.Doc
			incrSyncChan <- doc
		case "d":
			_, err := esCli.Delete().Index(config.ElasticIndex).Id(id).Do(context.Background())
			if err != nil {
				molog.Errorf("delete document in es err:%v id:%s", err, id)
				continue
			}
		case "u":
			f := bson.M{"_id": id}
			if idTypeOf.String() == "primitive.ObjectID" {
				id = o.Update["_id"].(primitive.ObjectID).Hex()
				objId, err := primitive.ObjectIDFromHex(id)
				if err != nil {
					molog.Errorf("objectid id err:%v id:%s", err, id)
					continue
				}
				f = bson.M{"_id": objId}
			} else if idTypeOf.String() == "int32" {
				f = bson.M{"_id": o.Doc["_id"].(int32)}
			} else if idTypeOf.String() == "int64" {
				f = bson.M{"_id": o.Doc["_id"].(int64)}
			}
			obj := make(map[string]interface{})
			err = dbColl.FindOne(context.Background(), f).Decode(&obj)
			if err != nil {
				molog.Errorf("find document from mongodb  err:%v id:%s", err, id)
				continue
			}
			delete(obj, "_id")
			var mongoDoc model.MongoDoc
			mongoDoc.ID = id
			mongoDoc.Doc = obj
			incrSyncChan <- mongoDoc
		}
	}
}
