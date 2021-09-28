package main

import (
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/logic"
	"github.com/levonmo/mongo-sync-elasticsearch/repo"
	"github.com/levonmo/mongo-sync-elasticsearch/utils"
	"runtime"
	"strings"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	filePath := config.GetConfigPath()
	if strings.TrimSpace(filePath) == "" {
		return
	}
	config.InitConfig(filePath)
	utils.IsOpenOplogTsFile()
	repo.InitElastic()
	repo.InitMongo()
	logic.Run()
}
