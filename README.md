# mongo-sync-elastic 使用说明
## 注：bin目录下的：mongo-sync-elastic是linux的可执行文件， mongo-sync-elastic.exe是windows的可执行文件

## 1.快速开始
* linux: bin/mongo-sync-elastic -f config.json
* windows：bin/mongo-sync-elastic.exe -f config.json
* 即可

## 2.配置文件 config.json 内容如下
```
{
  "mongodb": "mydb",
  "mongocoll": "mycoll",
  "mongodburl": "mongodb://wpsroot:wpspwd@localhost:27017",
  "esurl": "http://127.0.0.1:9200",
  "tspath": "./"
}
```
### 参数说明:
* mongodb: 数
