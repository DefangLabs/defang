package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

var _ client.OrphanCleaner = (*ByocAws)(nil)

// maxALBNameLen is the effective length AWS CD truncates load balancer names to (see alb_logs.go).
const maxALBNameLen = 31

// orphanDetail holds the cloud-specific data needed to clean up an OrphanResource. Only the
// fields relevant to the resource's category are populated.
type orphanDetail struct {
	category string
	lbArn    string
	dbID     string
	repoName string
	zoneID   string
	record   r53types.ResourceRecordSet
}

// resourceBaseName returns the dash-joined {Prefix}-{project}-{stack} base that Defang/Pulumi use
// to name resources (e.g. the ECS cluster "Defang-<project>-<stack>-cluster" and task-definition
// families). Case is preserved; callers that match case-insensitive resources (ALB, RDS) lowercase
// it themselves.
func (b *ByocAws) resourceBaseName(projectName string) string {
	base := projectName + "-" + b.PulumiStack
	if b.Prefix != "" {
		base = b.Prefix + "-" + base
	}
	return base
}

// projectZoneName returns the Defang-managed hosted zone for the project's public services
// (<projectLabel>.<tenant>.defang.app), matching ServicePublicDNS.
func (b *ByocAws) projectZoneName(projectName string) string {
	tenantLabel := dns.SafeLabel(string(b.TenantLabel))
	return b.GetProjectLabel(projectName) + "." + tenantLabel + ".defang.app"
}

// DiscoverOrphans finds AWS resources left behind by `defang down` that block Pulumi from
// finishing cleanup on a subsequent run. Failures in any single category are logged and skipped
// so the remaining categories can still be reported.
func (b *ByocAws) DiscoverOrphans(ctx context.Context, projectName string) ([]client.OrphanResource, error) {
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	b.orphans = map[string]orphanDetail{}
	var resources []client.OrphanResource
	add := func(id string, r client.OrphanResource, d orphanDetail) {
		r.ID = id
		b.orphans[id] = d
		resources = append(resources, r)
	}

	base := b.resourceBaseName(projectName)
	lowerBase := strings.ToLower(base) // ALB/RDS names are lowercased

	// ALBs: any leftover load balancer is unblocked by disabling deletion protection (idempotent).
	albPrefix := lowerBase
	if len(albPrefix) > maxALBNameLen {
		albPrefix = albPrefix[:maxALBNameLen]
	}
	elbClient := elbv2.NewFromConfig(cfg)
	if lbs, err := aws.FindLoadBalancersByPrefix(ctx, albPrefix, elbClient); err != nil {
		term.Warnf("cleanup: could not list load balancers: %v", err)
	} else {
		for _, lb := range lbs {
			add("alb:"+*lb.LoadBalancerArn, client.OrphanResource{
				Category: "alb",
				Name:     *lb.LoadBalancerName,
				Action:   "disable deletion protection so 'defang down' can delete the load balancer",
			}, orphanDetail{category: "alb", lbArn: *lb.LoadBalancerArn})
		}
	}

	// RDS: same as ALBs; disabling deletion protection is idempotent.
	rdsClient := rds.NewFromConfig(cfg)
	if insts, err := aws.FindDBInstancesByPrefix(ctx, lowerBase, rdsClient); err != nil {
		term.Warnf("cleanup: could not list RDS instances: %v", err)
	} else {
		for _, inst := range insts {
			add("rds:"+*inst.DBInstanceIdentifier, client.OrphanResource{
				Category: "rds",
				Name:     *inst.DBInstanceIdentifier,
				Action:   "disable deletion protection so 'defang down' can delete the database",
			}, orphanDetail{category: "rds", dbID: *inst.DBInstanceIdentifier})
		}
	}

	// ECR: a non-empty repository blocks deletion (RepositoryNotEmptyException); deleting its
	// images lets Pulumi remove it. Empty repositories are not blockers, so they are skipped.
	ecrClient := ecr.NewFromConfig(cfg)
	repoPrefix := b.GetProjectLabel(projectName) + "/"
	if repos, err := aws.FindRepositoriesByPrefix(ctx, repoPrefix, ecrClient); err != nil {
		term.Warnf("cleanup: could not list ECR repositories: %v", err)
	} else {
		for _, repo := range repos {
			ids, err := aws.ListImageIDs(ctx, *repo.RepositoryName, ecrClient)
			if err != nil {
				term.Warnf("cleanup: could not list images for %s: %v", *repo.RepositoryName, err)
				continue
			}
			if len(ids) == 0 {
				continue
			}
			add("ecr:"+*repo.RepositoryName, client.OrphanResource{
				Category: "ecr",
				Name:     *repo.RepositoryName,
				Action:   fmt.Sprintf("delete %d image(s) so 'defang down' can delete the repository", len(ids)),
			}, orphanDetail{category: "ecr", repoName: *repo.RepositoryName})
		}
	}

	// Route53: records left in the project's hosted zones block those zones from being deleted.
	// Both the public delegated subdomain zone and the private "<project>.internal" service-
	// discovery zone are managed by Defang and need their leftover records removed.
	r53Client := route53.NewFromConfig(cfg)
	for _, zoneName := range []string{b.projectZoneName(projectName), b.GetPrivateDomain(projectName)} {
		zones, err := aws.GetHostedZonesByName(ctx, zoneName, r53Client)
		if err != nil {
			if !errors.Is(err, aws.ErrZoneNotFound) {
				term.Warnf("cleanup: could not look up hosted zone %q: %v", zoneName, err)
			}
			continue
		}
		for _, zone := range zones {
			records, err := aws.ListAllResourceRecordSets(ctx, *zone.Id, r53Client)
			if err != nil {
				term.Warnf("cleanup: could not list records in zone %s: %v", zoneName, err)
				continue
			}
			for _, rec := range records {
				if isApexManagedRecord(rec, zoneName) {
					continue // NS/SOA at the apex are removed automatically when the zone is deleted
				}
				setID := ""
				if rec.SetIdentifier != nil {
					setID = *rec.SetIdentifier
				}
				add(fmt.Sprintf("dns:%s:%s:%s:%s", *zone.Id, *rec.Name, rec.Type, setID), client.OrphanResource{
					Category: "dns",
					Name:     fmt.Sprintf("%s %s (%s)", rec.Type, *rec.Name, zoneName),
					Action:   "delete DNS record so 'defang down' can delete the hosted zone",
				}, orphanDetail{category: "dns", zoneID: *zone.Id, record: rec})
			}
		}
	}

	return resources, nil
}

// CleanupOrphan performs the minimum action needed to unblock Pulumi cleanup of the given
// resource. It must be passed a resource from the most recent DiscoverOrphans call.
func (b *ByocAws) CleanupOrphan(ctx context.Context, r client.OrphanResource) error {
	detail, ok := b.orphans[r.ID]
	if !ok {
		return fmt.Errorf("unknown resource %q; run discovery again before cleaning up", r.ID)
	}
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return AnnotateAwsError(err)
	}

	switch detail.category {
	case "alb":
		return AnnotateAwsError(aws.SetALBDeletionProtection(ctx, detail.lbArn, false, elbv2.NewFromConfig(cfg)))
	case "rds":
		return AnnotateAwsError(aws.SetDBInstanceDeletionProtection(ctx, detail.dbID, false, rds.NewFromConfig(cfg)))
	case "ecr":
		ecrClient := ecr.NewFromConfig(cfg)
		ids, err := aws.ListImageIDs(ctx, detail.repoName, ecrClient)
		if err != nil {
			return AnnotateAwsError(err)
		}
		return AnnotateAwsError(aws.DeleteImages(ctx, detail.repoName, ids, ecrClient))
	case "dns":
		return AnnotateAwsError(aws.DeleteResourceRecordSet(ctx, detail.zoneID, detail.record, route53.NewFromConfig(cfg)))
	default:
		return fmt.Errorf("unsupported orphan category %q", detail.category)
	}
}

func isApexManagedRecord(rec r53types.ResourceRecordSet, zoneName string) bool {
	if rec.Type != r53types.RRTypeNs && rec.Type != r53types.RRTypeSoa {
		return false
	}
	return rec.Name != nil && dns.Normalize(*rec.Name) == dns.Normalize(zoneName)
}
