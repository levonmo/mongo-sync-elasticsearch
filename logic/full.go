package logic

import (
	"context"
	"encoding/json"
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/conts"
	"github.com/levonmo/mongo-sync-elasticsearch/log"
	"github.com/levonmo/mongo-sync-elasticsearch/repo"
	"github.com/olivere/elastic"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"reflect"
	"strconv"
	"sync"
	"time"
)

func StartFullDataSync2Elastic() {
	startTime := time.Now()
	//进行全量同步
	coll := repo.GetMongoClient().Database(config.GetInstance().MongoDbName).Collection(config.GetInstance().MongoCollName)
	cursor, err := coll.Find(context.Background(), bson.M{})
	if err != nil {
		log.Err.Printf("coll.Find err:%v", err)
		return
	}
	defer cursor.Close(context.Background())
	log.Info.Println("start sync historical data...")
	// 创建带缓存channel，由mongo生产数据 ,es消费
	fullSyncChan := make(chan map[string]interface{}, conts.FullSyncBufferedChannelMaxSize)
	fullSyncWG := sync.WaitGroup{}
	for gorouNumber := 0; gorouNumber < conts.FullSyncGoroutineDefaultCount; gorouNumber++ {
		fullSyncWG.Add(1)
		// 启动协程消费
		go listenFullSyncData(fullSyncChan, &fullSyncWG, gorouNumber)
	}
	for {
		ok := cursor.Next(context.Background())
		if !ok {
			break
		}
		mongoDoc := make(map[string]interface{})
		err = cursor.Decode(&mongoDoc)
		if err != nil {
			log.Err.Printf("sync historical data decode err:%v", err)
			break
		}
		fullSyncChan <- mongoDoc
	}
	close(fullSyncChan)
	fullSyncWG.Wait()
	log.Info.Printf("sync full data success,cost: %.3fms", float32(time.Since(startTime))/float32(time.Millisecond))
}

func listenFullSyncData(fullSyncMongoDocChan chan map[string]interface{}, fullSyncGoroutineWG *sync.WaitGroup, gorouNumber int, ) {
	defer fullSyncGoroutineWG.Done()
	defer log.Info.Printf("full sync goroutine number: %d exit", gorouNumber)
	log.Info.Printf("full sync goroutine number: %d start", gorouNumber)
	bulks := make([]elastic.BulkableRequest, conts.MaxFullSyncBatchInsertCount)
	bulk := repo.GetElasticClient().Bulk()
	for {
		count := 0
		for i := 0; i < conts.MaxFullSyncBatchInsertCount; i++ {
			select {
			case mongoDoc, ok := <-fullSyncMongoDocChan:
				if !ok {
					break
				}
				idTypeOf := reflect.TypeOf(mongoDoc["_id"])
				var id string
				switch idTypeOf.String() {
				case "primitive.ObjectID":
					id = mongoDoc["_id"].(primitive.ObjectID).Hex()
				case "string":
					id = mongoDoc["_id"].(string)
				case "int32":
					id = strconv.Itoa(int(mongoDoc["_id"].(int32)))
				case "int64":
					id = strconv.Itoa(int(mongoDoc["_id"].(int64)))
				}
				delete(mongoDoc, "_id")
				bytes, err := json.Marshal(mongoDoc)
				if err != nil {
					log.Info.Printf("sync full data json marshal err:%v id:%s", err, id)
					continue
				}
				doc := elastic.NewBulkIndexRequest().Index(config.GetInstance().ElasticIndexName).Id(id).Doc(string(bytes))
				bulks[count] = doc
				count++
			}
		}
		if count != 0 {
			bulk.Add(bulks[:count]...)
			bulkResponse, err := bulk.Do(context.Background())
			if err != nil {
				log.Info.Printf("batch processing, bulk do err:%v count:%d", err, len(bulks))
				//很可能是es挂了，等待5秒再重试
				time.Sleep(time.Second * 5)
				continue
			}
			for _, v := range bulkResponse.Failed() {
				log.Info.Printf("index: %s, type: %s, _id: %s, error: %+v", v.Index, v.Type, v.Id, *v.Error)
			}
			bulk.Reset()
			count = 0
		} else {
			break
		}
	}
}
