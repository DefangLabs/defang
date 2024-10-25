package quota

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
