package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/conts"
	"github.com/levonmo/mongo-sync-elasticsearch/log"
	"github.com/levonmo/mongo-sync-elasticsearch/model"
	"github.com/levonmo/mongo-sync-elasticsearch/repo"
	"github.com/olivere/elastic"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"
)

func StartIncrDataSync2Elastic(markCheckoutTs *primitive.Timestamp) {
	//根据上面获取的ts，开始重放oplog
	query := bson.M{
		"ts":          bson.M{"$gt": markCheckoutTs},
		"op":          bson.M{"$in": model.OpCodes},
		"ns":          config.GetInstance().MongoDbName + "." + config.GetInstance().MongoCollName,
		"fromMigrate": bson.M{"$exists": false},
	}
	optss := &options.FindOptions{}
	optss.SetSort(bson.M{"$natural": 1})
	optss.SetCursorType(options.TailableAwait)
	local := repo.GetMongoClient().Database("local").Collection("oplog.rs")
	cursor, err := local.Find(context.Background(), query, optss)
	if err != nil {
		log.Err.Printf("tail oplog.rs based latestoplog timestamp err:%v", err)
		return
	}
	incrSyncMongoDocChan := make(chan model.MongoDoc, conts.IncrSyncBufferedChannelMaxSize)
	curCheckoutTs := *markCheckoutTs
	defer close(incrSyncMongoDocChan)
	go listenSyncIncrData2Elastic(incrSyncMongoDocChan)
	go markCheckoutPoint(&curCheckoutTs)
	for cursor.Next(context.Background()) {
		o := model.OpLog{}
		if err = cursor.Decode(&o); err != nil {
			log.Err.Printf("tail decode oplog.rs err:%v", err)
			continue
		}
		mongoDoc := model.MongoDoc{
			Op:     o.Operation,
			Doc:    o.Doc,
			Update: o.Update,
		}
		incrSyncMongoDocChan <- mongoDoc
		curCheckoutTs = o.Timestamp
	}
}

func listenSyncIncrData2Elastic(incrSyncMongoDocChan chan model.MongoDoc) {
	log.Info.Printf("start sync increment data...")
	go func() {
		bulk := repo.GetElasticClient().Bulk()
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
						log.Info.Printf("batch processing, bulk do err:%v count:%d\n", err, len(bulks))
						bulksLock.Unlock()
						continue
					}
					for _, v := range bulkResponse.Failed() {
						log.Err.Printf("index: %s, type: %s, _id: %s, error: %+v", v.Index, v.Type, v.Id, *v.Error)
					}
					log.Info.Printf("sync incr data successCount:%d, faildCount:%d", len(bulks)-len(bulkResponse.Failed()), len(bulkResponse.Failed()))
					bulk.Reset()
					bulks = make([]elastic.BulkableRequest, 0)
					bulksLock.Unlock()
				}
			}
		}()
		for mongoDoc := range incrSyncMongoDocChan {
			docId, doc := getDocId(mongoDoc)
			switch mongoDoc.Op {
			case conts.OperationInsert:
				bulksLock.Lock()
				doc := elastic.NewBulkIndexRequest().Index(config.GetInstance().ElasticIndexName).Type("_doc").Id(docId).Doc(doc).RetryOnConflict(conts.ElasticMaxRetryOnConflict)
				bulks = append(bulks, doc)
				bulksLock.Unlock()
			case conts.OperationUpdate:
				// Fix bug that program won't work for mongo 4.4
				bulksLock.Lock()
				latestDocsMatchingUpdateQuery := findLatestDocs(&mongoDoc)
				for _, theDoc := range latestDocsMatchingUpdateQuery {
					doc := elastic.NewBulkUpdateRequest().Index(config.GetInstance().ElasticIndexName).Type("_doc").Id(docId).Doc(theDoc).RetryOnConflict(conts.ElasticMaxRetryOnConflict)
					bulks = append(bulks, doc)
				}
				bulksLock.Unlock()
			case conts.OperationDelete:
				_, err := repo.GetElasticClient().Delete().Index(config.GetInstance().ElasticIndexName).Type("_doc").Id(docId).Do(context.Background())
				if err != nil {
					log.Err.Printf("es delete doc fail,err:%v,index:%s,id:%s", err, config.GetInstance().ElasticIndexName, docId)
				} else {
					log.Info.Printf("es delete doc success,index:%s,id:%s", config.GetInstance().ElasticIndexName, docId)
				}
			default:
				log.Err.Printf("unknown op type:%s", mongoDoc.Op)
			}
		}
	}()
}

func findLatestDocs(doc *model.MongoDoc) []map[string]interface{} {
	query := doc.Update
	db := repo.GetMongoClient().Database(config.GetInstance().MongoDbName)

	var cursor *mongo.Cursor
	var err error

	cursor, err = db.Collection(config.GetInstance().MongoCollName).Find(context.TODO(), query)
	if err != nil {
		log.Err.Println("Querying mongodb db error when syncing updates.")
		return make([]map[string]interface{}, 0)
	}

	// TODO: stupid code, I don't know how to use a dynamic arr in go
	nonIdCarryingDocs := make([]map[string]interface{}, 0)
	for cursor.Next(context.TODO()) {
		var res = make(map[string]interface{})
		if err = cursor.Decode(&res); err != nil {
			log.Err.Println("Error decoding ")
		}
		delete(res, "_id")
		nonIdCarryingDocs = append(nonIdCarryingDocs, res)
	}

	return nonIdCarryingDocs

}

func getDocId(mongoDoc model.MongoDoc) (string, map[string]interface{}) {
	var obj map[string]interface{}
	switch mongoDoc.Op {
	case conts.OperationUpdate:
		obj = mongoDoc.Update
	default:
		obj = mongoDoc.Doc
	}
	var docId string
	switch reflect.TypeOf(obj["_id"]).String() {
	case "primitive.ObjectID":
		docId = obj["_id"].(primitive.ObjectID).Hex()
	case "string":
		docId = obj["_id"].(string)
	case "int32":
		docId = strconv.Itoa(int(obj["_id"].(int32)))
	case "int64":
		docId = strconv.Itoa(int(obj["_id"].(int64)))
	default:
		docId = fmt.Sprintf("%d", obj["_id"])
	}

	switch mongoDoc.Op {
	case conts.OperationUpdate:
		diff := mongoDoc.Doc["diff"]
		if diff != nil {
			diffType := reflect.TypeOf(diff)
			if diffType != nil && diffType == reflect.TypeOf(map[string]interface{}{}) {
				dNew := make(map[string]interface{})
				diffMap := mongoDoc.Doc["diff"].(map[string]interface{})
				if u := diffMap["u"]; u != nil {
					u := diffMap["u"]
					uType := reflect.TypeOf(u)
					if uType != nil && uType == reflect.TypeOf(map[string]interface{}{}) {
						for k, v := range u.(map[string]interface{}) {
							dNew[k] = v
						}
						obj = dNew
					}
				}
				if i := diffMap["i"]; i != nil {
					i := diffMap["i"]
					iType := reflect.TypeOf(i)
					if iType != nil && iType == reflect.TypeOf(map[string]interface{}{}) {
						for k, v := range i.(map[string]interface{}) {
							dNew[k] = v
						}
						obj = dNew
					}
				}
				if d := diffMap["d"]; d != nil {
					d := diffMap["d"]
					dType := reflect.TypeOf(d)
					if dType != nil && dType == reflect.TypeOf(map[string]interface{}{}) {
						for k, _ := range d.(map[string]interface{}) {
							dNew[k] = nil
						}
						obj = dNew
					}
				}

			}
		}
	default:
		delete(obj, "_id")
	}
	return docId, obj
}

func markCheckoutPoint(currentOpLogTimestamp *primitive.Timestamp) {
	go func() {
		for {
			select {
			case <-time.After(time.Second * 3):
				var filePath string
				if config.GetInstance().CheckPointPath == "" {
					filePath = "./oplogts/" + config.GetInstance().ElasticIndexName + "_latestoplog.log"
				} else {
					filePath = config.GetInstance().CheckPointPath + "/oplogts/" + config.GetInstance().ElasticIndexName + "_latestoplog.log"
				}
				f, err := os.Create(filePath)
				if err != nil {
					log.Err.Printf("os create oplog file err:%v", err)
					return
				}
				bytes, err := json.Marshal(currentOpLogTimestamp)
				if err != nil {
					log.Err.Printf("oplog json marshal err:%v,ts:%v", err, currentOpLogTimestamp)
				}
				_, err = f.Write(bytes)
				if err != nil {
					log.Err.Printf("oplog file writer err:%v", err)
					return
				}
				err = f.Close()
				if err != nil {
					return
				}
			}
		}
	}()
}
