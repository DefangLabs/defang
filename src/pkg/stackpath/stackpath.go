// Package stackpath holds the stack-qualified naming contract that the Defang
// CLI shares with the Pulumi provider (DefangLabs/pulumi-defang) and with the
// TypeScript stack that preceded it.
//
// These names are a wire contract, not labels. The provider creates the
// CloudWatch log groups; the CLI subscribes to them by name and routes each
// event to a parser by the group's last path segment. If the two sides
// disagree on a single segment the CLI silently tails a log group nobody
// writes to, and `defang compose up` never sees its services come up.
//
// The package deliberately depends on the standard library only, so that the
// provider can import it without pulling in the rest of the CLI.
package stackpath

import "strings"

// Well-known last path segments of the per-stack CloudWatch log groups.
// Each names one event source the CLI tails and parses differently.
const (
	// LogGroupECS carries ECS lifecycle events forwarded from EventBridge.
	// The CLI routes an event to the ECS parser only when its log group name
	// ends in "/" + LogGroupECS.
	LogGroupECS = "ecs"

	// LogGroupBuilds carries CodeBuild output for image builds.
	LogGroupBuilds = "builds"

	// LogGroupServices carries the services' own stdout and stderr.
	LogGroupServices = "logs"
)

// StackDir builds the stack-qualified path "/<prefix>/<project>/<stack>/<name>".
// An empty prefix drops its segment.
func StackDir(prefix, projectName, stack, name string) string {
	segments := []string{""} // leading slash
	if prefix != "" {
		segments = append(segments, prefix)
	}
	return strings.Join(append(segments, projectName, stack, name), "/")
}

// IsLogGroup reports whether logGroupIdentifier names the well-known log group
// name, which is true when it ends in "/" + name. The identifier may be a bare
// log group name, an ARN, or the "<account>:<name>" form CloudWatch returns in
// a live tail, so only the trailing segment can be compared.
func IsLogGroup(logGroupIdentifier, name string) bool {
	return strings.HasSuffix(logGroupIdentifier, "/"+name)
}
