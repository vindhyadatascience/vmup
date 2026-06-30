package gcloud

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// ImageInfo identifies a bootable compute image and the project it lives in.
type ImageInfo struct {
	Name    string
	Family  string
	Project string
}

// rawImage is the subset of the Compute images resource we care about.
type rawImage struct {
	Name       string `json:"name"`
	Family     string `json:"family"`
	SelfLink   string `json:"selfLink"`
	Deprecated struct {
		State string `json:"state"`
	} `json:"deprecated"`
}

func (r rawImage) active() bool {
	// An empty state (no "deprecated" block) means the image is current.
	return r.Deprecated.State == "" || r.Deprecated.State == "ACTIVE"
}

// projectFromSelfLink extracts the project from an image self link of the form
// .../projects/<project>/global/images/<name>.
func projectFromSelfLink(selfLink string) string {
	parts := strings.Split(selfLink, "/")
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// FetchImages returns the non-deprecated images in the given project, newest
// first. It uses the Compute REST API so access errors surface as API errors
// (see IsAccessDenied).
func FetchImages(project string) ([]ImageInfo, error) {
	url := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/global/images", project)
	body, err := apiGet(url)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []rawImage `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing images response: %w", err)
	}

	var images []ImageInfo
	for _, img := range resp.Items {
		if !img.active() {
			continue
		}
		images = append(images, ImageInfo{Name: img.Name, Family: img.Family, Project: project})
	}
	sortImages(images)
	return images, nil
}

// FetchStandardImages returns the public GCP images that `gcloud compute images
// list` surfaces by default — the well-known public projects (debian-cloud,
// ubuntu-os-cloud, etc.) plus the active project. There is no single REST
// endpoint spanning those projects, so this shells out to gcloud.
func FetchStandardImages() ([]ImageInfo, error) {
	cmd := exec.Command("gcloud", "compute", "images", "list", "--format=json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing standard images: %w", err)
	}

	var raw []rawImage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing standard images: %w", err)
	}

	var images []ImageInfo
	for _, img := range raw {
		if !img.active() {
			continue
		}
		images = append(images, ImageInfo{
			Name:    img.Name,
			Family:  img.Family,
			Project: projectFromSelfLink(img.SelfLink),
		})
	}
	sortImages(images)
	return images, nil
}

// sortImages orders images by project, then family, then name (descending name
// within a family so newer dated builds sort first).
func sortImages(images []ImageInfo) {
	sort.Slice(images, func(i, j int) bool {
		if images[i].Project != images[j].Project {
			return images[i].Project < images[j].Project
		}
		if images[i].Family != images[j].Family {
			return images[i].Family < images[j].Family
		}
		return images[i].Name > images[j].Name
	})
}

// IsAccessDenied reports whether err is a permission/not-found error from the
// images API — the signal that the configured image project is unusable for
// this user (no access, or the project does not exist).
func IsAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "API error 403") || strings.Contains(s, "API error 404")
}
