# mongo-sync-elasticsearch 使用说明


## 1.快速开始
* linux:  mongo-sync-elasticsearch-linux -f config.json
* windows：mongo-sync-elasticsearch-windows.exe -f config.json
* mac:  mongo-sync-elasticsearch-mac -f config.json
### 注：bin目录下的：
* mongo-sync-elasticsearch-linux是linux的可执行文件
* mongo-sync-elasticsearch-windows.exe是windows的可执行文件
* mongo-sync-elasticsearch-mac是mac的可执行文件

## 2.配置文件 config.json 内容如下
```
{
  "mongo_db_name": "mydb",
  "mongo_coll_name": "mycoll",
  "mongodb_url": "mongodb://127.0.0.1:27017/?connect=direct",
  "elasticsearch_url": "http://127.0.0.1:9200",
  "elastic_index_name": "",
  "check_point_path": "",
  "sync_type": ""
}

```
* 更多参数看第五点

### 必填参数:
* mongo_db_name: 数据库名字
* mongo_coll_name: 集合名字
* mongodb_url: 连接数据库的url
* elasticsearch_url: 连接es的url

* tspath: 

### 非必填参数:
* elastic_index_name：同步之后elasticsearch的名字，不填默认是：数据库名小写 + "__" + 集合名字小写，例如：mydb__mycoll
* check_point_path: 用于服务意外停止做数据恢复的，或断点续传时使用 (默认是程序执行所在的路径下oplogts文件夹保存同步状态)（1）当已经完成全量同步的时候，程序会在tspath路径下创建 oplogts/mydb_mycoll_latestoplog.log 文件，纪录下时间节点，意味着在该时间节点之前的数据都已完成同步，但当全量同步失败不会创建该文件 （2）每隔1个小时就会更新 mydb_mycoll_latestoplog.log 文件里面的时间节点 （3）当服务意外停止时，并且不愿意再进行一次全量同步，只需同步服务停止之后还没同步的数据，则服务再次启动时check_point_path不能改变，让数据从check_point_path中恢复 （4）当服务意外停止时，并希望从0开始重新同步一次，则可以在oplogts文件夹下面删除对应的log文件 或 重新选择一个check_point_path即可
* sync_type：同步类型。默认或不填表示：全量+增量，填 full 表示：只进行全量同步，填 incr 表示：只进行增量同步 

### 4.备注:
* 1.涉及增量同步的部分，需要mongodb的部署形式是副本集 或者 单实例开启了oplog
* 2.启动服务的用户需要拥有参数tspath路径下文件的创建查看删除权限
* 3.参数mongodburl中的mongodb用户需要拥有local库下的oplog.rs查询权限
* 4.在es中创建的索引名字是 mongodb+'__'+mongocoll，即: mongodb__mongocoll (大写全部转换为小写)

****

# mongo-sync-elasticsearch instructions
## tips：bin folder：mongo-sync-elasticsearch is linux executable， mongo-sync-elasticsearch.exe is windows executable

## 1.Quickstart
* linux: bin/mongo-sync-elasticsearch -f config.json
* windows：bin/mongo-sync-elasticsearch.exe -f config.json

## 2.configuration file config.json content:
```
{
  "mongodb": "mydb",
  "mongocoll": "mycoll",
  "mongodburl": "mongodb://myroot:mypwd@localhost:27017",
  "esurl": "http://127.0.0.1:9200",
  "tspath": "./"
}
```
* See the fifth point for more parameters

### param explain:
* mongodb: db name
* mongocoll: collection name
* mongodburl: mongodb url
* esurl: es url
* tspath: non required parameter。For data recovery when the service stops unexpectedly，Or when resuming a breakpoint(The default is to save the synchronization status in the oplogts folder under the path where the program is executed)

## 3.tspath param explain
* When full synchronization has been completed，The program will be created under the path of tspath:oplogts/mydb_mycoll_latestoplog.log file，record time node，It means that the data before the time node has been synchronized，But when full synchronization fails, the file will not be created
* Update every hour mydb_mycoll_latestoplog.log Time node in the file
* When the service stops unexpectedly，and I don't want to have another full synchronization，Just synchronize the data that has not been synchronized since the service stopped，The tspath cannot be changed when the service starts again，Recover data from tspath
* When the service stops unexpectedly,and want to resynchronize from earliest，You can delete the corresponding log file under the tspath or reselect a tspath


### 4.Remarks:
* 1.Involving incremental synchronization, the deployment form of mongodb needs to be a replica set or a single instance with oplog enabled
* 2.The user who starts the service needs to have the permission to create, view and delete the file under the parameter tspath path
* 3.The mongodb user in the parameter mongodb URL needs to have the query permission of oplog.rs under the admin Library
* 4.The index name created in ES is mongodb+'__'+mongocoll，eg: mydb__mycoll

### 5.More optional parameters:
* 1.syncType: Synchronization type
```
There are three values 0, 1, 2 optional, respectively:
0:The default value,means full + incremental synchronization
1:It means only full synchronization, the program will exit after the full synchronization
2:Indicates only incremental synchronization
eg ：config.json:
{
    "syncType":1 // Only full sync
}
```


