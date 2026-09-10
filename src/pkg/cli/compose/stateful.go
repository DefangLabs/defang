package compose

import "strings"

var statefulImages = []string{
	"cassandra",
	"couchdb",
	"elasticsearch",
	"etcd",
	"influxdb",
	"mariadb",
	"minio", // could be stateless
	"mongo",
	"mssql/server",
	"mysql",
	"nats",
	"neo4j",
	"oracle/database",
	"percona",
	"pgvector",
	"postgres",
	"rabbitmq",
	"redis",
	"redis-stack",
	"rethinkdb",
	"scylla",
	"timescaledb",
	"valkey",
	"valkey-bundle",
	"vault",
	"zookeeper",
}

func isStatefulImage(image string) bool {
	repo := GetImageRepo(image)
	for _, statefulImage := range statefulImages {
		if strings.HasSuffix(repo, statefulImage) {
			return true
		}
	}
	return false
}
