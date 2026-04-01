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
// Fallback rates (us-central1, on-demand). Used when billing API is unavailable.
var fallbackRates = map[string]MachineTypeRate{
	// General purpose
	"e2":  {VCPURate: 0.02181, MemoryRate: 0.002924},
	"n1":  {VCPURate: 0.031611, MemoryRate: 0.004237},
	"n2":  {VCPURate: 0.03161, MemoryRate: 0.009550},
	"n2d": {VCPURate: 0.02750, MemoryRate: 0.000922},
	"n4":  {VCPURate: 0.03275, MemoryRate: 0.003717},
	"n4a": {VCPURate: 0.02778, MemoryRate: 0.006770},
	"n4d": {VCPURate: 0.02911, MemoryRate: 0.000074},
	// Compute optimized
	"c2":  {VCPURate: 0.02956, MemoryRate: 0.003959}, // same as c2d
	"c2d": {VCPURate: 0.02956, MemoryRate: 0.003959},
	"c3":  {VCPURate: 0.03465, MemoryRate: 0.003938},
	"c3d": {VCPURate: 0.02956, MemoryRate: 0.003959},
	"c4":  {VCPURate: 0.00347, MemoryRate: 0.003938},
	"c4a": {VCPURate: 0.03086, MemoryRate: 0.000350},
	"c4d": {VCPURate: 0.03270, MemoryRate: 0.000350},
	// Memory optimized
	"m1":  {VCPURate: 0.034800, MemoryRate: 0.005100},
	"m2":  {VCPURate: 0.034800, MemoryRate: 0.005100},
	"m3":  {VCPURate: 0.034800, MemoryRate: 0.005100},
	"m4":  {VCPURate: 0.00183, MemoryRate: 0.004570},
	// Accelerator optimized
	"a2":  {VCPURate: 0.01739, MemoryRate: 0.002330},
	"a3":  {VCPURate: 0.01189, MemoryRate: 0.000952},
	"g2":  {VCPURate: 0.02499, MemoryRate: 0.002927},
	// Tau (ARM/AMD)
	"t2a": {VCPURate: 0.02490, MemoryRate: 0.003400},
	"t2d": {VCPURate: 0.02750, MemoryRate: 0.003686},
	// Storage optimized
	"h3":  {VCPURate: 0.04411, MemoryRate: 0.002960},
	"h4d": {VCPURate: 0.00361, MemoryRate: 0.001288},
	"z3":  {VCPURate: 0.04965, MemoryRate: 0.006655},
}

// Flat-rate instances (not per-vCPU/memory).
var flatRateInstances = map[string]float64{
	"f1-micro":  0.0076,
	"g1-small":  0.0257,
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
			descUp := strings.ToUpper(sku.Description)
			isCPU := group == "CPU" || strings.Contains(group, "CORE") || strings.Contains(group, "VCPU") ||
				(strings.Contains(descUp, "CORE") && !strings.Contains(descUp, "RAM"))
			isRAM := group == "RAM" || strings.Contains(group, "MEMORY") ||
				(strings.Contains(descUp, "RAM") && !strings.Contains(descUp, "CORE"))

			// Handle non-standard resource groups (N1Standard, F1Micro, G1Small)
			if !isCPU && !isRAM {
				groupL := strings.ToLower(group)
				if groupL == "n1standard" || groupL == "f1micro" || groupL == "g1small" {
					isCPU = strings.Contains(descUp, "CORE") || strings.Contains(descUp, "VCPU") || strings.Contains(descUp, "CPU") || strings.Contains(descUp, "MICRO") || strings.Contains(descUp, "SMALL")
					isRAM = strings.Contains(descUp, "RAM")
				}
			}
			if !isCPU && !isRAM {
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
			if isCPU {
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
	region = strings.ToLower(region)
	for _, r := range sku.ServiceRegions {
		r = strings.ToLower(r)
		if r == region {
			return true
		}
	}
	// Fallback: accept broad regions like "us", "europe", "asia", "americas"
	for _, r := range sku.ServiceRegions {
		r = strings.ToLower(r)
		if strings.HasPrefix(region, r) {
			return true
		}
		// "americas" covers all US/SA regions
		if r == "americas" && (strings.HasPrefix(region, "us") || strings.HasPrefix(region, "southamerica") || strings.HasPrefix(region, "northamerica")) {
			return true
		}
	}
	return false
}

func extractFamily(description string) string {
	desc := strings.ToLower(description)

	// Try to find a known machine family pattern anywhere in the description.
	// SKU descriptions vary:
	//   "E2 Instance Core running in Americas"
	//   "N2D AMD Instance Core running in Americas"
	//   "Compute optimized Instance Core running in Americas" (no family prefix)
	for _, token := range strings.Fields(desc) {
		if len(token) < 2 {
			continue
		}
		// Must start with a letter and contain a digit (e.g., e2, n2d, c3, m1, a2, t2a, h3)
		if token[0] < 'a' || token[0] > 'z' {
			continue
		}
		hasDigit := false
		allAlnum := true
		for _, c := range token {
			if c >= '0' && c <= '9' {
				hasDigit = true
			} else if c < 'a' || c > 'z' {
				allAlnum = false
				break
			}
		}
		if hasDigit && allAlnum {
			return token
		}
	}

	// Handle descriptions without a family-prefixed token
	if strings.Contains(desc, "memory-optimized") && !strings.Contains(desc, "m3") && !strings.Contains(desc, "m4") {
		return "m1" // M1/M2 use generic "Memory-optimized" descriptions
	}
	if strings.Contains(desc, "micro instance") || strings.Contains(desc, "micro") {
		return "f1"
	}
	if strings.Contains(desc, "small instance") {
		return "g1"
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
// For flat-rate instances (f1-micro, g1-small), pass the full machine type name.
func CalculateHourlyRate(machineType, family string, vcpus int, memoryGB float64, rates map[string]MachineTypeRate) float64 {
	// Check flat-rate instances first
	if flat, ok := flatRateInstances[machineType]; ok {
		return flat
	}
	rate, ok := rates[family]
	if !ok {
		// Try fallback rates
		rate, ok = fallbackRates[family]
		if !ok {
			return 0
		}
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
