package aws

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
)

// goCDEntrypoint is the CD program inside the Go CD image built by
// DefangLabs/pulumi-defang: a single static binary, with no shell or runtime
// around it.
var goCDEntrypoint = []string{"/app/cd"}

// nodeCDEntrypoint is the CD program inside the legacy TypeScript CD image
// built by defang-mvp's cd.Dockerfile, which runs under Node.
var nodeCDEntrypoint = []string{"node", "lib/index.js"}

// cdEntrypoint returns the argv prefix that starts the CD program in cdImage.
//
// CodeBuild is the only driver that needs this: it runs the build command
// through a buildspec shell, which bypasses the image's own ENTRYPOINT, so
// the CLI has to name the program itself — and the two CD flavours name it
// differently. Azure Container Apps and GCP Cloud Build launch the image's
// ENTRYPOINT directly and never call this.
func cdEntrypoint(cdImage string) []string {
	if byoc.IsLegacyCDImage(cdImage) {
		return nodeCDEntrypoint
	}
	return goCDEntrypoint
}
