package gcloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"
)

// --- Token management ---

var (
	cachedToken   string
	tokenExpiry   time.Time
	tokenMu       sync.Mutex
	httpClient    = &http.Client{Timeout: 30 * time.Second}
)

func getAccessToken() (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	cmd := exec.Command("gcloud", "auth", "print-access-token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting access token: %w", err)
	}

	cachedToken = strings.TrimSpace(string(out))
	// Tokens are valid for ~60 minutes; refresh at 50 minutes to be safe
	tokenExpiry = time.Now().Add(50 * time.Minute)
	return cachedToken, nil
}

func invalidateToken() {
	tokenMu.Lock()
	cachedToken = ""
	tokenExpiry = time.Time{}
	tokenMu.Unlock()
}

// --- HTTP helpers ---

func apiGet(url string) ([]byte, error) {
	token, err := getAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Retry once on 401 with a fresh token
	if resp.StatusCode == 401 {
		invalidateToken()
		token, err = getAccessToken()
		if err != nil {
			return nil, err
		}
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// --- Instance batch query ---

type AttachedDisk struct {
	DiskName string
	Mode     string // READ_WRITE or READ_ONLY
}

type InstanceInfo struct {
	Status string
	Disks  []AttachedDisk
}

type instancesAggregatedList struct {
	Items map[string]struct {
		Instances []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Disks  []struct {
				Source string `json:"source"`
				Mode   string `json:"mode"`
			} `json:"disks"`
		} `json:"instances"`
	} `json:"items"`
}

// FetchInstancesByProject returns a map of instance name → InstanceInfo
// for all instances in the given project across all zones.
func FetchInstancesByProject(projectID string) (map[string]InstanceInfo, error) {
	url := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/aggregated/instances", projectID)
	body, err := apiGet(url)
	if err != nil {
		return nil, err
	}

	var resp instancesAggregatedList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing instances response: %w", err)
	}

	result := make(map[string]InstanceInfo)
	for _, scopedList := range resp.Items {
		for _, inst := range scopedList.Instances {
			info := InstanceInfo{Status: inst.Status}
			for _, d := range inst.Disks {
				info.Disks = append(info.Disks, AttachedDisk{
					DiskName: path.Base(d.Source),
					Mode:     d.Mode,
				})
			}
			result[inst.Name] = info
		}
	}
	return result, nil
}

// --- Disk batch query ---

type DiskInfo struct {
	Status   string
	Users    []string // instance names
	SizeGB   string
	DiskType string // short type name, e.g. "pd-ssd"
}

type disksAggregatedList struct {
	Items map[string]struct {
		Disks []struct {
			Name   string   `json:"name"`
			Status string   `json:"status"`
			Users  []string `json:"users"`
			SizeGb string   `json:"sizeGb"`
			Type   string   `json:"type"`
		} `json:"disks"`
	} `json:"items"`
}

// FetchDisksByProject returns a map of disk name → DiskInfo
// for all disks in the given project across all zones.
func FetchDisksByProject(projectID string) (map[string]DiskInfo, error) {
	url := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/aggregated/disks", projectID)
	body, err := apiGet(url)
	if err != nil {
		return nil, err
	}

	var resp disksAggregatedList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing disks response: %w", err)
	}

	result := make(map[string]DiskInfo)
	for _, scopedList := range resp.Items {
		for _, disk := range scopedList.Disks {
			var users []string
			for _, u := range disk.Users {
				users = append(users, path.Base(u))
			}
			result[disk.Name] = DiskInfo{
				Status:   disk.Status,
				Users:    users,
				SizeGB:   disk.SizeGb,
				DiskType: path.Base(disk.Type),
			}
		}
	}
	return result, nil
}
