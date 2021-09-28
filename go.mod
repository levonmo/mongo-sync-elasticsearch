module github.com/levonmo/mongo-sync-elasticsearch

go 1.16

require (
	github.com/fortytw2/leaktest v1.3.0 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/olivere/elastic v6.2.37+incompatible
	go.mongodb.org/mongo-driver v1.7.2
)

replace go.mongodb.org/mongo-driver v1.7.2 => github.com/mongodb/mongo-go-driver v1.7.2
