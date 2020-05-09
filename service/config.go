package service

import (
	"encoding/json"
	"os"
)

type Config struct {
	MongoDB    string `json:"mongodb"`
	MongoColl  string `json:"mongocoll"`
	MongodbUrl string `json:"mongodburl"`
	EsUrl      string `json:"esurl"`
	Tspath     string `json:"tspath"`
	SyncType   int    `json:"syncType"`
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

