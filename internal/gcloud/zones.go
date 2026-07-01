package gcloud

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
)

// FetchZonesByRegion returns a map of region name → sorted UP zone names for the
// project, using the Compute zones API. Any project the caller can access works
// (the zone topology is the same); it is only needed to authorize the request.
func FetchZonesByRegion(project string) (map[string][]string, error) {
	url := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/zones", project)
	body, err := apiGet(url)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []struct {
			Name   string `json:"name"`
			Region string `json:"region"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing zones response: %w", err)
	}

	out := make(map[string][]string)
	for _, z := range resp.Items {
		if z.Status != "" && z.Status != "UP" {
			continue
		}
		region := path.Base(z.Region)
		out[region] = append(out[region], z.Name)
	}
	for r := range out {
		sort.Strings(out[r])
	}
	return out, nil
}
