package upcloudapi

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/service"
	"github.com/suruaku/upcloud-app-platform/internal/infra"
)

var uuidPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

type Provider struct {
	svc *service.Service
}

func NewProvider(token string, timeout time.Duration) (*Provider, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("upcloud token is required")
	}

	cl := client.New("", "", client.WithBearerAuth(token), client.WithTimeout(timeout))
	return &Provider{svc: service.New(cl)}, nil
}

func (p *Provider) Provision(ctx context.Context, req infra.ProvisionRequest) (infra.ProvisionResult, error) {
	templateUUID, err := p.resolveTemplateUUID(ctx, req.Zone, req.Template)
	if err != nil {
		return infra.ProvisionResult{}, err
	}

	created, err := p.svc.CreateServer(ctx, &request.CreateServerRequest{
		Zone:             req.Zone,
		Title:            req.Hostname,
		Hostname:         req.Hostname,
		Plan:             req.Plan,
		PasswordDelivery: request.PasswordDeliveryNone,
		Metadata:         upcloud.True,
		NICModel:         upcloud.NICModelVirtio,
		UserData:         string(req.CloudInitRaw),
		Networking: &request.CreateServerNetworking{
			Interfaces: []request.CreateServerInterface{
				{
					Type: upcloud.IPAddressAccessPublic,
					IPAddresses: []request.CreateServerIPAddress{
						{Family: upcloud.IPAddressFamilyIPv4},
					},
				},
			},
		},
		StorageDevices: []request.CreateServerStorageDevice{
			{
				Action:  request.CreateServerStorageDeviceActionClone,
				Storage: templateUUID,
				Title:   req.Hostname + "-disk-1",
			},
		},
	})
	if err != nil {
		return infra.ProvisionResult{}, fmt.Errorf("create upcloud server: %w", err)
	}

	return infra.ProvisionResult{ServerID: created.UUID, Hostname: created.Hostname}, nil
}

func (p *Provider) Get(ctx context.Context, serverID string) (infra.ServerInfo, error) {
	details, err := p.svc.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverID})
	if err != nil {
		return infra.ServerInfo{}, fmt.Errorf("get server details %q: %w", serverID, err)
	}

	return toServerInfo(details), nil
}

func (p *Provider) WaitReady(ctx context.Context, serverID string, timeout time.Duration) (infra.ServerInfo, error) {
	waitCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	details, err := p.svc.WaitForServerState(waitCtx, &request.WaitForServerStateRequest{
		UUID:         serverID,
		DesiredState: upcloud.ServerStateStarted,
	})
	if err != nil {
		return infra.ServerInfo{}, fmt.Errorf("wait for server %q to become started: %w", serverID, err)
	}

	return toServerInfo(details), nil
}

func (p *Provider) EstimateServerCost(ctx context.Context, zone, plan string) (infra.ServerCostEstimate, error) {
	zone = strings.TrimSpace(zone)
	plan = strings.TrimSpace(plan)
	if zone == "" {
		return infra.ServerCostEstimate{}, fmt.Errorf("zone is required")
	}
	if plan == "" {
		return infra.ServerCostEstimate{}, fmt.Errorf("plan is required")
	}

	pricesByZone, err := p.svc.GetPricesByZone(ctx)
	if err != nil {
		return infra.ServerCostEstimate{}, fmt.Errorf("get prices by zone: %w", err)
	}

	zonePrices, ok := (*pricesByZone)[zone]
	if !ok {
		return infra.ServerCostEstimate{}, fmt.Errorf("no pricing data for zone %q", zone)
	}

	planCandidates := buildPlanPriceKeys(plan)
	for _, key := range planCandidates {
		price, found := zonePrices[key]
		if !found {
			continue
		}

		monthly := price.Price
		hourly := monthly / priceCentsDivisor
		if hourly < 0 {
			hourly = 0
		}

		return infra.ServerCostEstimate{
			Currency: "EUR",
			Monthly:  hourly * billedHoursPerMonth,
			Hourly:   hourly,
			Source:   key,
		}, nil
	}

	return infra.ServerCostEstimate{}, fmt.Errorf("no pricing data for plan %q in zone %q", plan, zone)
}

func (p *Provider) Destroy(ctx context.Context, serverID string) error {
	if err := p.svc.DeleteServer(ctx, &request.DeleteServerRequest{UUID: serverID}); err != nil {
		if !isDeleteWhileStartedError(err) {
			return fmt.Errorf("delete server %q: %w", serverID, err)
		}

		if stopErr := p.stopBeforeDelete(ctx, serverID); stopErr != nil {
			return fmt.Errorf("stop server %q before delete: %w", serverID, stopErr)
		}

		if retryErr := p.svc.DeleteServer(ctx, &request.DeleteServerRequest{UUID: serverID}); retryErr != nil {
			return fmt.Errorf("delete server %q after stop: %w", serverID, retryErr)
		}
	}
	return nil
}

func (p *Provider) stopBeforeDelete(ctx context.Context, serverID string) error {
	_, err := p.svc.StopServer(ctx, &request.StopServerRequest{
		UUID:     serverID,
		StopType: upcloud.StopTypeHard,
		Timeout:  5 * time.Minute,
	})
	if err != nil && !isAlreadyStoppedError(err) {
		return err
	}

	_, err = p.svc.WaitForServerState(ctx, &request.WaitForServerStateRequest{
		UUID:         serverID,
		DesiredState: upcloud.ServerStateStopped,
	})
	if err != nil && !isLikelyNotFoundError(err) {
		return err
	}

	return nil
}

func isDeleteWhileStartedError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "server_state_illegal") && strings.Contains(msg, "state 'started'")
}

func isAlreadyStoppedError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "server_state_illegal") && strings.Contains(msg, "state 'stopped'")
}

func isLikelyNotFoundError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "status=404") || strings.Contains(msg, "status code 404")
}

func (p *Provider) resolveTemplateUUID(ctx context.Context, zone string, template string) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", fmt.Errorf("template is required")
	}
	if uuidPattern.MatchString(template) {
		return template, nil
	}

	storages, err := p.svc.GetStorages(ctx, &request.GetStoragesRequest{})
	if err != nil {
		return "", fmt.Errorf("list storages: %w", err)
	}

	for _, s := range storages.Storages {
		if s.Access == upcloud.StorageAccessPublic && s.Type == upcloud.StorageTypeTemplate && zoneMatchesTemplate(zone, s.Zone) && s.Title == template && s.State == upcloud.StorageStateOnline {
			return s.UUID, nil
		}
	}

	needle := strings.ToLower(template)
	for _, s := range storages.Storages {
		if s.Access == upcloud.StorageAccessPublic && s.Type == upcloud.StorageTypeTemplate && zoneMatchesTemplate(zone, s.Zone) && strings.Contains(strings.ToLower(s.Title), needle) && s.State == upcloud.StorageStateOnline {
			return s.UUID, nil
		}
	}

	normalizedNeedle := normalizeTemplateName(template)
	for _, s := range storages.Storages {
		if s.Access != upcloud.StorageAccessPublic || s.Type != upcloud.StorageTypeTemplate || !zoneMatchesTemplate(zone, s.Zone) || s.State != upcloud.StorageStateOnline {
			continue
		}
		normalizedTitle := normalizeTemplateName(s.Title)
		if strings.Contains(normalizedTitle, normalizedNeedle) || strings.Contains(normalizedNeedle, normalizedTitle) {
			return s.UUID, nil
		}
	}

	matchingZones := collectMatchingTemplateZones(storages.Storages, normalizedNeedle)
	if len(matchingZones) > 0 {
		return "", fmt.Errorf("no storage template match found in zone %q for template %q; matching templates exist in zones: %s", zone, template, strings.Join(matchingZones, ", "))
	}

	return "", fmt.Errorf("no storage template match found in zone %q for template %q", zone, template)
}

func normalizeTemplateName(v string) string {
	v = strings.ToLower(v)
	v = strings.ReplaceAll(v, ".", " ")
	v = strings.ReplaceAll(v, "-", " ")
	v = strings.Join(strings.Fields(v), " ")
	return strings.TrimSpace(v)
}

func collectMatchingTemplateZones(storages []upcloud.Storage, normalizedNeedle string) []string {
	if normalizedNeedle == "" {
		return nil
	}

	zones := make(map[string]struct{})
	for _, s := range storages {
		if s.Access != upcloud.StorageAccessPublic || s.Type != upcloud.StorageTypeTemplate || s.State != upcloud.StorageStateOnline {
			continue
		}
		normalizedTitle := normalizeTemplateName(s.Title)
		if strings.Contains(normalizedTitle, normalizedNeedle) || strings.Contains(normalizedNeedle, normalizedTitle) {
			zoneLabel := strings.TrimSpace(s.Zone)
			if zoneLabel == "" {
				zoneLabel = "<all-zones>"
			}
			zones[zoneLabel] = struct{}{}
		}
	}

	out := make([]string, 0, len(zones))
	for zone := range zones {
		out = append(out, zone)
	}
	sort.Strings(out)
	return out
}

func zoneMatchesTemplate(requestedZone, templateZone string) bool {
	requestedZone = strings.TrimSpace(requestedZone)
	templateZone = strings.TrimSpace(templateZone)
	if templateZone == "" {
		return true
	}
	return requestedZone == templateZone
}

func toServerInfo(details *upcloud.ServerDetails) infra.ServerInfo {
	info := infra.ServerInfo{
		ServerID:  details.UUID,
		Hostname:  details.Hostname,
		State:     details.State,
		Plan:      strings.TrimSpace(details.Plan),
		Zone:      strings.TrimSpace(details.Zone),
		CoreCount: details.CoreNumber,
		MemoryMB:  details.MemoryAmount,
	}

	for _, ip := range details.IPAddresses {
		if ip.Access == upcloud.IPAddressAccessPublic && ip.Family == upcloud.IPAddressFamilyIPv4 {
			info.PublicIPv4 = ip.Address
			break
		}
	}

	return info
}

func buildPlanPriceKeys(plan string) []string {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return nil
	}

	candidates := make([]string, 0, 3)
	candidates = append(candidates, "server_plan_"+plan)

	lowerPlan := strings.ToLower(plan)
	if lowerPlan != plan {
		candidates = append(candidates, "server_plan_"+lowerPlan)
	}

	normalizedPlan := strings.ReplaceAll(lowerPlan, "xcpu", "xCPU")
	if normalizedPlan != plan && normalizedPlan != lowerPlan {
		candidates = append(candidates, "server_plan_"+normalizedPlan)
	}

	unique := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, v := range candidates {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		unique = append(unique, v)
	}

	return unique
}

const (
	priceCentsDivisor   = 100.0
	billedHoursPerMonth = 24.0 * 28.0
)
