package quota

const Mode_INGRESS = "ingress"
const Mode_HOST = "host"

const Protocol_TCP = "tcp"
const Protocol_UDP = "udp"
const Protocol_HTTP = "http"

const MiB = 1024 * 1024

type Quotas struct {
	ServiceQuotas
	ConfigCount int
	ConfigSize  int
	Ingress     int
	Services    int
}

func getOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}
