package byoc

import (
	_ "crypto/sha256" // registers the "sha256" digest algorithm reference.Parse needs to parse digests
	"strings"

	"github.com/distribution/reference"
)

// IsDefangMvpCDImage reports whether cdImage was built by defang-mvp rather than
// DefangLabs/pulumi-defang. defang-mvp tags every CD image "<name>-<sha>-<arch>"
// with a name that starts with "public-" or "nodejs-" (public-cd-image and
// nodejs-cd-image for AWS, public-gcp for GCP), and its fallback tags are
// public-beta and public-gcp-beta. pulumi-defang CD images are tagged by
// version instead (v2.6.0-aws, 2.6.0-alpha-<sha>, pr-<number>), or pinned by a
// bare digest, so an untagged image is never a defang-mvp image.
//
// The image reference is the only signal available: Fabric returns a bare URI
// in CanIUseResponse.cd_image, and both flavours are published to the same
// repositories, so neither the registry nor the repository path tells them apart.
func IsDefangMvpCDImage(cdImage string) bool {
	tag := cdImageTag(cdImage)
	return strings.HasPrefix(tag, "public-") || strings.HasPrefix(tag, "nodejs-")
}

// DefangMvpCD reports whether this client runs a defang-mvp CD image; see IsDefangMvpCDImage.
func (b *ByocBaseClient) DefangMvpCD() bool {
	return IsDefangMvpCDImage(b.CDImage)
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
