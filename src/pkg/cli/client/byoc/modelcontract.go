package byoc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	defanghttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/oci"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/distribution/reference"
)

// modelContractLabel is the OCI image config label of a CD image that resolves
// managed model requests itself. Its value is the version of the x-defang-llm
// payload contract the image implements; an older image ignores the request
// fields and would deploy a gateway that cannot route.
const (
	modelContractLabel   = "io.defang.model-contract"
	modelContractVersion = "1"
)

// publicCDRepo receives every released CD image in the same build-push step as
// the cloud registries, so it serves the same manifest digests. Scaleway's
// registry refuses anonymous reads, so a digest is verified here instead; the
// content digest makes it the identical image.
var publicCDRepo = "ghcr.io/defanglabs/cd"

// modelContractHTTPClient is replaced by tests that serve images over TLS.
var modelContractHTTPClient = defanghttp.DefaultClient

// VerifyModelContract reads the label from the registry of the CD image this
// client will run, which is the image after any DEFANG_CD_IMAGE override or pin.
func (b *ByocBaseClient) VerifyModelContract(ctx context.Context) error {
	return verifyModelContract(ctx, modelContractHTTPClient, b.CDImage)
}

func verifyModelContract(ctx context.Context, httpClient *http.Client, cdImage string) error {
	if cdImage == "" {
		return modelContractError(cdImage, "is not set")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	labels, err := oci.FetchImageLabels(ctx, httpClient, cdImage)
	if mirror := publicCDDigest(cdImage); err != nil && mirror != "" {
		term.Debugf("reading CD image %q failed (%v); reading the same digest from %q", cdImage, err, mirror)
		labels, err = oci.FetchImageLabels(ctx, httpClient, mirror)
	}
	if err != nil {
		return modelContractError(cdImage, fmt.Sprintf("could not be verified (%v)", err))
	}
	switch version := labels[modelContractLabel]; version {
	case modelContractVersion:
		term.Debugf("CD image %q implements model contract %s", cdImage, version)
		return nil
	case "":
		return modelContractError(cdImage, "does not support managed model requests")
	default:
		return fmt.Errorf("CD image %q implements model contract %s, but this Defang CLI requires %s; update the Defang CLI", cdImage, version, modelContractVersion)
	}
}

// publicCDDigest returns the public CD repo reference for a digest reference
// elsewhere, or "" when a tag alone cannot identify the image.
func publicCDDigest(cdImage string) string {
	named, err := reference.ParseNormalizedNamed(cdImage)
	if err != nil || named.Name() == publicCDRepo {
		return ""
	}
	digested, ok := named.(reference.Digested)
	if !ok {
		return ""
	}
	return publicCDRepo + "@" + digested.Digest().String()
}

func modelContractError(cdImage, reason string) error {
	remedy := "If this stack is pinned to the CD engine of an earlier deployment, re-run with --allow-upgrade (or set DEFANG_ALLOW_UPGRADE=1)"
	if os.Getenv("DEFANG_CD_IMAGE") != "" {
		remedy = "DEFANG_CD_IMAGE is set: unset it, or point it at a CD image that supports managed models"
	}
	return fmt.Errorf("managed models are resolved by the CD engine, but CD image %q %s (it needs the %s=%s image label). %s", cdImage, reason, modelContractLabel, modelContractVersion, remedy)
}
