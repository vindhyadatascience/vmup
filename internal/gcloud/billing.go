package gcloud

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
)

// MachineTypeRate holds per-vCPU and per-GB hourly rates for a machine family.
type MachineTypeRate struct {
	VCPURate   float64 // per vCPU per hour
	MemoryRate float64 // per GB per hour
}

// Hardcoded fallback rates (us-central1, on-demand, as of 2025).
var fallbackRates = map[string]MachineTypeRate{
	"e2":  {VCPURate: 0.03351, MemoryRate: 0.004491},
	"n2":  {VCPURate: 0.03510, MemoryRate: 0.004699},
	"n2d": {VCPURate: 0.03060, MemoryRate: 0.004099},
	"c3":  {VCPURate: 0.03710, MemoryRate: 0.004967},
	"n1":  {VCPURate: 0.03322, MemoryRate: 0.004450},
}

var (
	cachedRates   map[string]MachineTypeRate
	cachedRatesMu sync.Mutex
)

const computeServiceID = "6F81-5844-456A"

// skuListResponse is the JSON shape from services.skus.list.
type skuListResponse struct {
	Skus          []skuEntry `json:"skus"`
	NextPageToken string     `json:"nextPageToken"`
}

type skuEntry struct {
	Description string `json:"description"`
	Category    struct {
		ResourceGroup string `json:"resourceGroup"`
		UsageType     string `json:"usageType"`
	} `json:"category"`
	ServiceRegions []string    `json:"serviceRegions"`
	PricingInfo    []priceInfo `json:"pricingInfo"`
}

type priceInfo struct {
	PricingExpression struct {
		UsageUnit   string       `json:"usageUnit"`
		TieredRates []tieredRate `json:"tieredRates"`
	} `json:"pricingExpression"`
}

type tieredRate struct {
	UnitPrice struct {
		CurrencyCode string `json:"currencyCode"`
		Units        string `json:"units"`
		Nanos        int64  `json:"nanos"`
	} `json:"unitPrice"`
}

// FetchComputeRates fetches on-demand per-vCPU and per-GB hourly rates
// from the Cloud Billing Catalog API, grouped by machine family.
// Falls back to hardcoded rates on failure.
func FetchComputeRates(region string) map[string]MachineTypeRate {
	cachedRatesMu.Lock()
	if cachedRates != nil {
		defer cachedRatesMu.Unlock()
		return cachedRates
	}
	cachedRatesMu.Unlock()

	rates, err := fetchRatesFromAPI(region)
	if err != nil || len(rates) == 0 {
		return fallbackRates
	}

	cachedRatesMu.Lock()
	cachedRates = rates
	cachedRatesMu.Unlock()

	return rates
}

func fetchRatesFromAPI(region string) (map[string]MachineTypeRate, error) {
	// Accumulate all relevant SKUs across pages
	type rateEntry struct {
		vcpu   float64
		memory float64
	}
	collected := make(map[string]*rateEntry) // keyed by family

	pageToken := ""
	for {
		url := fmt.Sprintf(
			"https://cloudbilling.googleapis.com/v1/services/%s/skus?currencyCode=USD&pageSize=5000",
			computeServiceID,
		)
		if pageToken != "" {
			url += "&pageToken=" + pageToken
		}

		body, err := apiGet(url)
		if err != nil {
			return nil, err
		}

		var resp skuListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing billing response: %w", err)
		}

		for _, sku := range resp.Skus {
			// Only on-demand compute
			if sku.Category.UsageType != "OnDemand" {
				continue
			}
			group := strings.ToUpper(sku.Category.ResourceGroup)
			if group != "CPU" && group != "RAM" {
				continue
			}

			// Check region match
			if !skuMatchesRegion(sku, region) {
				continue
			}

			// Extract family from description (e.g., "E2 Instance Core running in Americas")
			family := extractFamily(sku.Description)
			if family == "" {
				continue
			}

			rate := extractHourlyRate(sku)
			if rate <= 0 {
				continue
			}

			if collected[family] == nil {
				collected[family] = &rateEntry{}
			}
			if group == "CPU" {
				collected[family].vcpu = rate
			} else {
				collected[family].memory = rate
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	// Convert to result map
	result := make(map[string]MachineTypeRate)
	for family, entry := range collected {
		if entry.vcpu > 0 && entry.memory > 0 {
			result[family] = MachineTypeRate{
				VCPURate:   entry.vcpu,
				MemoryRate: entry.memory,
			}
		}
	}

	return result, nil
}

func skuMatchesRegion(sku skuEntry, region string) bool {
	for _, r := range sku.ServiceRegions {
		if r == region {
			return true
		}
		// Some SKUs use broad regions like "us" or "americas"
		if strings.HasPrefix(strings.ToLower(region), strings.ToLower(r)) {
			return true
		}
	}
	return false
}

func extractFamily(description string) string {
	desc := strings.ToLower(description)
	// Match known families from the description prefix
	families := []string{"e2", "n2d", "n2", "n1", "c3d", "c3", "c4", "m3", "m2", "m1", "a2", "a3", "g2", "t2d", "t2a"}
	for _, f := range families {
		if strings.HasPrefix(desc, f+" ") {
			return f
		}
	}
	return ""
}

func extractHourlyRate(sku skuEntry) float64 {
	if len(sku.PricingInfo) == 0 {
		return 0
	}
	rates := sku.PricingInfo[0].PricingExpression.TieredRates
	if len(rates) == 0 {
		return 0
	}
	tr := rates[len(rates)-1] // use the last tier (typically the standard rate)
	units := 0.0
	fmt.Sscanf(tr.UnitPrice.Units, "%f", &units)
	return units + float64(tr.UnitPrice.Nanos)/1e9
}

// CalculateHourlyRate computes the estimated hourly cost for a machine type.
func CalculateHourlyRate(family string, vcpus int, memoryGB float64, rates map[string]MachineTypeRate) float64 {
	rate, ok := rates[family]
	if !ok {
		return 0
	}
	return float64(vcpus)*rate.VCPURate + memoryGB*rate.MemoryRate
}

// MachineFamily extracts the family prefix from a machine type name (e.g., "e2" from "e2-highmem-2").
func MachineFamily(machineType string) string {
	parts := strings.SplitN(machineType, "-", 2)
	if len(parts) == 0 {
		return ""
	}
	// Handle families like "n2d" — check if the second part starts with a letter
	family := parts[0]
	if len(parts) > 1 && len(parts[1]) > 0 {
		rest := strings.SplitN(parts[1], "-", 2)
		// e.g., "n2d-standard-2" → parts[0]="n2d"
		// but "e2-highmem-2" → parts[0]="e2"
		// The family is always the first token if it matches a known pattern
		_ = rest
	}
	// For "n2d-standard-2", SplitN("-", 2) gives ["n2d", "standard-2"]
	// For "e2-highmem-2", gives ["e2", "highmem-2"]
	return strings.ToLower(family)
}

// --- Machine Types API ---

// MachineTypeInfo holds specs for a machine type from the Compute API.
type MachineTypeInfo struct {
	Name     string
	GuestCpus int
	MemoryMB  int
}

type machineTypesListResponse struct {
	Items         []machineTypeItem `json:"items"`
	NextPageToken string            `json:"nextPageToken"`
}

type machineTypeItem struct {
	Name      string `json:"name"`
	GuestCpus int    `json:"guestCpus"`
	MemoryMb  int    `json:"memoryMb"`
}

// FetchMachineTypes returns all machine types available in a zone.
func FetchMachineTypes(projectID, zone string) ([]MachineTypeInfo, error) {
	var allTypes []MachineTypeInfo
	pageToken := ""

	for {
		url := fmt.Sprintf(
			"https://compute.googleapis.com/compute/v1/projects/%s/zones/%s/machineTypes?maxResults=500",
			projectID, zone,
		)
		if pageToken != "" {
			url += "&pageToken=" + pageToken
		}

		body, err := apiGet(url)
		if err != nil {
			return nil, err
		}

		var resp machineTypesListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing machine types: %w", err)
		}

		for _, item := range resp.Items {
			allTypes = append(allTypes, MachineTypeInfo{
				Name:      path.Base(item.Name),
				GuestCpus: item.GuestCpus,
				MemoryMB:  item.MemoryMb,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return allTypes, nil
}
