# mongo-sync-elasticsearch 使用说明


## 1.快速开始
* linux: bin/mongo-sync-elasticsearch-linux -f config.json
* windows: bin/mongo-sync-elasticsearch-windows.exe -f config.json
* mac: bin/mongo-sync-elasticsearch-mac -f config.json

## 2.配置文件 config.json 内容如下
```
{
  "mongo_db_name": "mydb",
  "mongo_coll_name": "mycoll",
  "mongodb_url": "mongodb://127.0.0.1:27017/",
  "elasticsearch_url": "http://127.0.0.1:9200",
  "elastic_index_name": "",
  "check_point_path": "",
  "sync_type": ""
}

```

### 必须参数:
* mongo_db_name: 数据库名字
* mongo_coll_name: 集合名字
* mongodb_url: 连接数据库的url
* elasticsearch_url: 连接es的url

### 非必须参数:
* elastic_index_name：同步之后elasticsearch的名字，不填默认是：数据库名小写 + "__" + 集合名字小写，例如：mydb__mycoll
* check_point_path: 用于服务意外停止做数据恢复的，或断点续传时使用 (默认是程序执行所在的路径下oplogts文件夹保存同步状态)
（1）当已经完成全量同步的时候，程序会在check_point_path路径下创建 oplogts/mydb_mycoll_latestoplog.log 文件，纪录下时间节点，意味着在该时间节点之前的数据都已完成同步，但当全量同步失败不会创建该文件 
（2）每隔3秒就会更新 mydb_mycoll_latestoplog.log 文件里面的时间节点 
（3）当服务意外停止时，并且不愿意再进行一次全量同步，只需同步服务停止之后还没同步的数据，则服务再次启动时check_point_path不能改变，让数据从check_point_path中恢复 
（4）当服务意外停止时，并希望从0开始重新同步一次，则可以在oplogts文件夹下面删除对应的log文件 或 重新选择一个check_point_path即可
* sync_type：同步类型。默认或不填表示：全量+增量，填 full 表示：只进行全量同步，填 incr 表示：只进行增量同步 


## 3.备注:
* 1.涉及增量同步的部分，需要mongodb的部署形式是副本集 或者 单实例开启了oplog，参数mongodb_url中的mongodb用户需要拥有local库下的oplog.rs查询权限
* 2.启动服务的用户需要拥有参数check_point_path路径下文件的创建查看删除权限

## 4.技术答疑和交流群
![avatar](pic/qq群.png)