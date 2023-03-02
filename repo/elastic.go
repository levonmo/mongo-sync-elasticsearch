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

	var cli *elastic.Client
	var err error

	if len(config.GetInstance().ElasticsearchUsername) == 0 {
		cli, err = elastic.NewClient(
			elastic.SetSniff(false),
			elastic.SetURL(config.GetInstance().ElasticsearchUrl))
	} else {
		cli, err = elastic.NewClient(
			elastic.SetSniff(false),
			elastic.SetURL(config.GetInstance().ElasticsearchUrl),
			elastic.SetBasicAuth(config.GetInstance().ElasticsearchUsername, config.GetInstance().ElasticsearchPassword))
	}

	if err != nil {
		msg := fmt.Sprintf("connect elastic fail， err:%v", err)
		log.Err.Println(msg)
		panic(msg)
	}
	client = cli
}
