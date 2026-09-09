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
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	rgt "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
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
// finishing cleanup on a subsequent run. Failures in any single category are logged and skipped so
// the remaining categories can still be reported, but if every category fails the error is returned
// rather than reported as "nothing found".
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

	// Collected so a total failure (e.g. missing permissions, throttling) is reported as an error
	// instead of an empty result, which would read as "no leftovers found".
	var errs []error
	warn := func(err error, format string, args ...any) {
		errs = append(errs, err)
		term.Warnf("cleanup: "+format+": %v", append(args, err)...)
	}

	base := b.resourceBaseName(projectName)
	lowerBase := strings.ToLower(base) // ALB/RDS names are lowercased

	// Every resource the CD creates on AWS carries defang:project/defang:stack tags
	// (pulumi-defang cd/program/aws.go, via the provider's DefaultTags), which is exact
	// regardless of naming. Prefer that over a name guess: the Go CD's ALB autonaming has gone
	// through several shapes, none of which include the b.Prefix this file used to assume (see
	// the fallback below), so a name-only match can miss the exact resource cleanup exists to
	// unblock. Name-prefix matching still runs as a fallback for pre-tagging deployments.
	rgtClient := rgt.NewFromConfig(cfg)
	taggedArns, err := aws.FindResourceArnsByTags(ctx, map[string]string{
		"defang:project": projectName,
		"defang:stack":   b.PulumiStack,
	}, []string{"elasticloadbalancing:loadbalancer", "rds:db"}, rgtClient)
	if err != nil {
		warn(err, "could not look up tagged resources")
	}

	// ALBs: any leftover load balancer is unblocked by disabling deletion protection (idempotent).
	elbClient := elbv2.NewFromConfig(cfg)
	seenALB := map[string]bool{}
	var lbs []elbv2types.LoadBalancer
	if tagged, err := aws.FindLoadBalancersByArns(ctx, arnsForService(taggedArns, "elasticloadbalancing"), elbClient); err != nil {
		warn(err, "could not describe tagged load balancers")
	} else {
		lbs = append(lbs, tagged...)
	}
	albPrefix := lowerBase
	if len(albPrefix) > maxALBNameLen {
		albPrefix = albPrefix[:maxALBNameLen]
	}
	if prefixed, err := aws.FindLoadBalancersByPrefix(ctx, albPrefix, elbClient); err != nil {
		warn(err, "could not list load balancers")
	} else {
		lbs = append(lbs, prefixed...)
	}
	for _, lb := range lbs {
		if lb.LoadBalancerArn == nil || seenALB[*lb.LoadBalancerArn] {
			continue
		}
		seenALB[*lb.LoadBalancerArn] = true
		add("alb:"+*lb.LoadBalancerArn, client.OrphanResource{
			Category: "alb",
			Name:     *lb.LoadBalancerName,
			Action:   "disable deletion protection so 'defang down' can delete the load balancer",
		}, orphanDetail{category: "alb", lbArn: *lb.LoadBalancerArn})
	}

	// RDS: same as ALBs; disabling deletion protection is idempotent.
	rdsClient := rds.NewFromConfig(cfg)
	seenRDS := map[string]bool{}
	var insts []rdstypes.DBInstance
	if tagged, err := aws.FindDBInstancesByArns(ctx, arnsForService(taggedArns, "rds"), rdsClient); err != nil {
		warn(err, "could not describe tagged RDS instances")
	} else {
		insts = append(insts, tagged...)
	}
	if prefixed, err := aws.FindDBInstancesByPrefix(ctx, lowerBase, rdsClient); err != nil {
		warn(err, "could not list RDS instances")
	} else {
		insts = append(insts, prefixed...)
	}
	for _, inst := range insts {
		if inst.DBInstanceIdentifier == nil || seenRDS[*inst.DBInstanceIdentifier] {
			continue
		}
		seenRDS[*inst.DBInstanceIdentifier] = true
		add("rds:"+*inst.DBInstanceIdentifier, client.OrphanResource{
			Category: "rds",
			Name:     *inst.DBInstanceIdentifier,
			Action:   "disable deletion protection so 'defang down' can delete the database",
		}, orphanDetail{category: "rds", dbID: *inst.DBInstanceIdentifier})
	}

	// ECR: a non-empty repository blocks deletion (RepositoryNotEmptyException); deleting its
	// images lets Pulumi remove it. Empty repositories are not blockers, so they are skipped.
	ecrClient := ecr.NewFromConfig(cfg)
	repoPrefix := b.GetProjectLabel(projectName) + "/"
	if repos, err := aws.FindRepositoriesByPrefix(ctx, repoPrefix, ecrClient); err != nil {
		warn(err, "could not list ECR repositories")
	} else {
		for _, repo := range repos {
			ids, err := aws.ListImageIDs(ctx, *repo.RepositoryName, ecrClient)
			if err != nil {
				warn(err, "could not list images for %s", *repo.RepositoryName)
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
				warn(err, "could not look up hosted zone %q", zoneName)
			}
			continue
		}
		for _, zone := range zones {
			records, err := aws.ListAllResourceRecordSets(ctx, *zone.Id, r53Client)
			if err != nil {
				warn(err, "could not list records in zone %s", zoneName)
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

	// A partial failure still yields a usable report, but if nothing was discovered the caller
	// cannot tell an empty account from a broken one, so surface the errors.
	if len(resources) == 0 && len(errs) > 0 {
		return nil, AnnotateAwsError(fmt.Errorf("failed to discover leftover resources: %w", errors.Join(errs...)))
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

// arnsForService returns the ARNs belonging to the given AWS service (e.g. "rds",
// "elasticloadbalancing"), out of a list that may mix services.
func arnsForService(arns []string, service string) []string {
	var filtered []string
	for _, arn := range arns {
		// arn:partition:service:region:account-id:resource
		if parts := strings.SplitN(arn, ":", 4); len(parts) >= 3 && parts[2] == service {
			filtered = append(filtered, arn)
		}
	}
	return filtered
}
