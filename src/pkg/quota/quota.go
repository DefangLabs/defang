package quota

type Quotas struct {
	ServiceQuotas
	ConfigCount int
	ConfigSize  int
	Ingress     int
	Services    int
}
