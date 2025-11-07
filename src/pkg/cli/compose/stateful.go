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
	"rethinkdb",
	"scylla",
	"timescaledb",
	"valkey",
	"vault",
	"zookeeper",
}

func isStatefulImage(image string) bool {
	repo := strings.ToLower(strings.SplitN(image, ":", 2)[0])
	for _, statefulImage := range statefulImages {
		if strings.HasSuffix(repo, statefulImage) {
			return true
		}
	}
	return false
}
