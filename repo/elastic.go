package repo

import (
	"fmt"
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/log"
	"github.com/olivere/elastic"
)

var client *elastic.Client

func GetElasticClient() *elastic.Client {
	return client
}

func InitElastic() {
	cli, err := elastic.NewClient(elastic.SetSniff(false), elastic.SetURL(config.GetInstance().ElasticsearchUrl))
	if err != nil {
		msg := fmt.Sprintf("connect elastic fail， err:%v", err)
		log.Err.Println(msg)
		panic(msg)
	}
	client = cli
}
