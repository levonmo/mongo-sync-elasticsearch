package conts

import "time"

const (
	// 一次oplog查询的超时时间
	OplogQueryTailTimeoutDuration = time.Second * 10
	// 增量同步channel队列的最大容量
	IncrSyncBufferedChannelMaxSize = 1000
	// 全量同步channel队列的最大容量
	FullSyncBufferedChannelMaxSize = 2000
	// 全量同步开启的goroutine默认数量
	FullSyncGoroutineDefaultCount = 3
	// 全量同步开启的goroutine最大数量
	FullSyncGoroutineMaxCount = 10

	// 最大批量同步条数
	MaxFullSyncBatchInsertCount = 5000
	MaxIncrSyncBatchInsertCount = 2000

	OperationInsert = "i"
	OperationDelete = "d"
	OperationUpdate = "u"

	ElasticMaxRetryOnConflict = 20

	SyncTypeDefault = "default" // 全量同步+增加同步
	SyncTypeFull    = "full"    // 仅全量同步
	SyncIncr        = "incr"    // 仅增量同步
)
