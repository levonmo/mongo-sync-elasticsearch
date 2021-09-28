package logic

import (
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/conts"
	"github.com/levonmo/mongo-sync-elasticsearch/utils"
)

func Run() {
	if config.GetInstance().SyncType == conts.SyncTypeFull {
		StartFullDataSync2Elastic()
		return
	}
	historyCheckPoint := utils.GetCheckPointFromHistory()
	if historyCheckPoint == nil {
		newestCheckPoint := utils.GetNewestOplogTsFromMongo()
		StartFullDataSync2Elastic()
		StartIncrDataSync2Elastic(newestCheckPoint)
	} else {
		StartIncrDataSync2Elastic(historyCheckPoint)
	}
}
