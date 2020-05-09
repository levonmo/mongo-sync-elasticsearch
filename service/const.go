package service

const (

	//syncType参数类型
	SyncTypeDefault = 0 //默认，全量+增量
	SyncTypeFull    = 1 //只进行全量同步
	SyncTypeIncr    = 2 //只进行增量同步
)
