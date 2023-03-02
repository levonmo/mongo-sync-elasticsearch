package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/levonmo/mongo-sync-elasticsearch/log"
)

var instance *Config

type Config struct {
	MongoDbName      string `json:"mongo_db_name"`
	MongoCollName    string `json:"mongo_coll_name"`
	MongodbUrl       string `json:"mongodb_url"`

	ElasticsearchUrl string `json:"elasticsearch_url"`
	ElasticsearchUsername string `json:"elasticsearch_username"`
	ElasticsearchPassword string `json:"elasticsearch_password"`

	ElasticIndexName string `json:"elastic_index_name"`
	CheckPointPath   string `json:"check_point_path"`
	SyncType         string `json:"sync_type"`
}

func GetInstance() *Config {
	return instance
}

func InitConfig(path string) *Config {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	bytes := make([]byte, 1024)
	n, err := file.Read(bytes)
	if err != nil {
		panic(err)
	}
	var config *Config
	err = json.Unmarshal(bytes[:n], &config)
	if err != nil {
		panic(err)
	}
	if config.ElasticIndexName == "" {
		config.ElasticIndexName = strings.ToLower(config.MongoDbName + "__" + config.MongoCollName)
	}
	instance = config
	log.Info.Println("init config success")
	return instance
}

func GetConfigPath() string {
	filePath := ""
	argsCount := len(os.Args)
	if argsCount == 1 {
		fmt.Println("-h/-help for help")
	} else if argsCount == 2 {
		if os.Args[1] == "-h" || os.Args[1] == "-help" {
			fmt.Println(` -f + config.json , for example: ./mongo-sync-elasticsearch -f config.json`)
		} else {
			fmt.Println("-h/-help for help")
		}
	} else if argsCount == 3 {
		if os.Args[1] == "-f" {
			filePath = os.Args[2]
		}
	} else {
		fmt.Println("-h/-help for help")
	}
	return filePath
}
