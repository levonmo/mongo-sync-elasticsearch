package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"mongo-sync-elasticsearch/log"
)

var configInstance Config

type Config struct {
	MongoDB      string `json:"mongodb"`
	MongoColl    string `json:"mongocoll"`
	MongodbUrl   string `json:"mongodburl"`
	EsUrl        string `json:"esurl"`
	Tspath       string `json:"tspath"`
	SyncType     int    `json:"syncType"`
	ElasticIndex string
}

func GetConfigInstance() Config {
	return configInstance
}

func InitConfig(path string) Config {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	bytes := make([]byte, 1024)
	n, err := file.Read(bytes)
	if err != nil {
		panic(err)
	}
	var config Config
	err = json.Unmarshal(bytes[:n], &config)
	if err != nil {
		panic(err)
	}
	config.ElasticIndex = strings.ToLower(config.MongoDB + "__" + config.MongoColl)
	configInstance = config
	molog.Infof("init config success: %+v", config)
	molog.Infof("elastic index is : %s", config.ElasticIndex)
	return configInstance
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
