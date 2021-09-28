package repo

import (
	"context"
	"github.com/levonmo/mongo-sync-elasticsearch/config"
	"github.com/levonmo/mongo-sync-elasticsearch/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client

func GetMongoClient() *mongo.Client {
	return mongoClient
}

func InitMongo() {
	cli, err := mongo.Connect(context.Background(), options.Client().ApplyURI(config.GetInstance().MongodbUrl))
	if err != nil {
		log.Err.Printf("mongo connect err:%v", err)
		return
	}
	mongoClient = cli
}
