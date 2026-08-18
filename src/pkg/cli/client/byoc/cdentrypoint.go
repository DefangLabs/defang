package byoc

import (
	"os"
	"strings"
)

// GoCDEntrypoint is the CD program inside the Go CD image built by
// DefangLabs/pulumi-defang: a single static binary, with no shell or runtime
// around it.
var GoCDEntrypoint = []string{"/app/cd"}

// NodeCDEntrypoint is the CD program inside the legacy TypeScript CD image
// built by defang-mvp's cd.Dockerfile, which runs under Node.
var NodeCDEntrypoint = []string{"node", "lib/index.js"}

// legacyCDImageMarker appears in the tag of every CD image built by defang-mvp
// (pulumi/ecs/cd_images.ts builds "public-cd-image" and "nodejs-cd-image",
// each tagged "<name>-<sha>-<arch>"). The Go CD images published by
// DefangLabs/pulumi-defang are tagged by version instead (v2.6.0-aws,
// 2.6.0-alpha-<sha>, pr-<number>).
const legacyCDImageMarker = "cd-image"

// CDEntrypoint returns the argv prefix that starts the CD program in cdImage.
//
// Drivers that launch the CD image directly (Azure Container Apps, GCP Cloud
// Build) use the image's own ENTRYPOINT and never need this. CodeBuild does:
// it runs the command through a buildspec shell, which bypasses ENTRYPOINT, so
// the CLI has to name the program itself — and the two CD flavours name it
// differently.
//
// The image reference is the only signal available: Fabric returns a bare URI
// in CanIUseResponse.cd_image, and both flavours are published to the same
// repositories, so neither the registry nor the repository path distinguishes
// them. The tag does: legacy images carry legacyCDImageMarker. Anything else is
// treated as the Go image, so newly published tags need no CLI change.
//
// DEFANG_CD_ENTRYPOINT overrides the choice (space-separated argv), which is
// what to reach for when testing an image whose tag says nothing about its
// flavour.
func CDEntrypoint(cdImage string) []string {
	if override := os.Getenv("DEFANG_CD_ENTRYPOINT"); override != "" {
		return strings.Fields(override)
	}
	if strings.Contains(cdImageTag(cdImage), legacyCDImageMarker) {
		return NodeCDEntrypoint
	}
	return GoCDEntrypoint
}

// cdImageTag returns the tag of an image reference, without the digest, or ""
// if the reference is untagged. The digest is stripped first so its "sha256:"
// separator is not mistaken for the tag separator, and only the final path
// element is considered so a registry's ":port" is not either.
func cdImageTag(image string) string {
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	if i := strings.LastIndex(image, ":"); i >= 0 {
		return image[i+1:]
	}
	return ""
}
