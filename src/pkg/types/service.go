package types

type ServiceStatus string

const (
	ServiceDeploymentStarting   ServiceStatus = "STARTING"
	ServiceDeploymentInProgress ServiceStatus = "IN_PROGRESS"
	ServiceStarted              ServiceStatus = "COMPLETED"
	ServiceStopping             ServiceStatus = "STOPPING"
	ServiceStopped              ServiceStatus = "STOPPED"
	ServiceDeactivating         ServiceStatus = "DEACTIVATING"
	ServiceDeprovisioning       ServiceStatus = "DEPROVISIONING"
	ServiceFailed               ServiceStatus = "FAILED"
	ServiceUnknown              ServiceStatus = "UNKNOWN"
)
