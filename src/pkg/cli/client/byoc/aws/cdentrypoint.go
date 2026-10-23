package aws

import (
	_ "crypto/sha256" // registers the "sha256" digest algorithm reference.Parse needs to parse digests
	"strings"

	"github.com/distribution/reference"
)

// goCDEntrypoint is the CD program inside the Go CD image built by
// DefangLabs/pulumi-defang: a single static binary, with no shell or runtime
// around it.
var goCDEntrypoint = []string{"/app/cd"}

// nodeCDEntrypoint is the CD program inside the legacy TypeScript CD image
// built by defang-mvp's cd.Dockerfile, which runs under Node.
var nodeCDEntrypoint = []string{"node", "lib/index.js"}

// legacyCDImageMarker appears in the tag of every CD image built by defang-mvp
// (pulumi/ecs/cd_images.ts builds "public-cd-image" and "nodejs-cd-image",
// each tagged "<name>-<sha>-<arch>"). The Go CD images published by
// DefangLabs/pulumi-defang are tagged by version instead (v2.6.0-aws,
// 2.6.0-alpha-<sha>, pr-<number>), or left untagged: every ref Fabric pins
// through its cd_image_* config is a bare digest, and only the Go CD pipeline
// writes those config keys, so untagged also means Go.
const legacyCDImageMarker = "cd-image"

// cdEntrypoint returns the argv prefix that starts the CD program in cdImage.
//
// CodeBuild is the only driver that needs this: it runs the build command
// through a buildspec shell, which bypasses the image's own ENTRYPOINT, so
// the CLI has to name the program itself — and the two CD flavours name it
// differently. Azure Container Apps and GCP Cloud Build launch the image's
// ENTRYPOINT directly and never call this.
//
// The image reference is the only signal available: Fabric returns a bare URI
// in CanIUseResponse.cd_image, and both flavours are published to the same
// repositories, so neither the registry nor the repository path distinguishes
// them. The tag does, via legacyCDImageMarker.
func cdEntrypoint(cdImage string) []string {
	if strings.Contains(cdImageTag(cdImage), legacyCDImageMarker) {
		return nodeCDEntrypoint
	}
	return goCDEntrypoint
}

// cdImageTag returns the tag of an image reference, or "" if it has none
// (untagged, or a bare digest reference).
func cdImageTag(image string) string {
	ref, err := reference.Parse(image)
	if err != nil {
		return ""
	}
	tagged, ok := ref.(reference.Tagged)
	if !ok {
		return ""
	}
	return tagged.Tag()
}
