package helm

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/blang/semver"
	"github.com/containers/image/v5/docker"
	imagetypes "github.com/containers/image/v5/types"
	"github.com/pkg/errors"
	apptypes "github.com/replicatedhq/kots/pkg/app/types"
	helmgetter "helm.sh/helm/v3/pkg/getter"
)

type ChartUpdate struct {
	Tag     string
	Version semver.Version
}

type ChartUpdates []ChartUpdate

var (
	updateCacheMutex sync.Mutex
	updateCache      map[string]ChartUpdates // available updates sorted in descending order for each chart
)

func init() {
	updateCache = make(map[string]ChartUpdates)
}

func (v ChartUpdates) Len() int {
	return len(v)
}

func (v ChartUpdates) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v ChartUpdates) Less(i, j int) bool {
	return v[i].Version.LT(v[j].Version)
}

func (u ChartUpdates) ToTagList() []string {
	tags := []string{}
	for _, update := range u {
		tags = append(tags, update.Tag)
	}
	return tags
}

func GetCachedUpdates(chartPath string) ChartUpdates {
	updateCacheMutex.Lock()
	defer updateCacheMutex.Unlock()

	return updateCache[chartPath]
}

func setCachedUpdates(chartPath string, updates ChartUpdates) {
	updateCacheMutex.Lock()
	defer updateCacheMutex.Unlock()

	updateCache[chartPath] = updates
}

// Removes this tag from cache and also every tag that is less than this one according to semver ordering
func removeFromCachedUpdates(chartPath string, tag string) {
	updateCacheMutex.Lock()
	defer updateCacheMutex.Unlock()

	version, parseErr := semver.ParseTolerant(tag)

	existingList := updateCache[chartPath]
	newList := ChartUpdates{}
	for _, update := range existingList {
		// If tag cannot be parsed, fall back on string comparison.
		// This should never happen for versions that are on the list because we only include valid semvers and Helm chart versions are valid semvers.
		if parseErr != nil {
			if update.Tag != tag {
				newList = append(newList, update)
			}
		} else if update.Version.GT(version) {
			newList = append(newList, update)
		}
	}
	updateCache[chartPath] = newList
}

func deleteUpdateCacheForChart(chartPath string) {
	updateCacheMutex.Lock()
	defer updateCacheMutex.Unlock()

	delete(updateCache, chartPath)
}

func CheckForUpdates(chartPath string, licenseID string, currentVersion *semver.Version) (ChartUpdates, error) {
	availableUpdates := ChartUpdates{}

	imageName := strings.TrimLeft(chartPath, "oci:")
	ref, err := docker.ParseReference(imageName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse image ref %q", imageName)
	}

	sysCtx := &imagetypes.SystemContext{
		DockerInsecureSkipTLSVerify: imagetypes.OptionalBoolTrue,
		DockerDisableV1Ping:         true,
		DockerAuthConfig: &imagetypes.DockerAuthConfig{
			Username: licenseID,
			Password: licenseID,
		},
	}

	tags, err := docker.GetRepositoryTags(context.TODO(), sysCtx, ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get repo tags")
	}

	tags = removeDuplicates(tags) // registry should not be returning duplicate tags

	for _, tag := range tags {
		v, err := semver.ParseTolerant(tag)
		if err != nil {
			// TODO: log
			continue
		}

		if currentVersion != nil && v.LE(*currentVersion) {
			continue
		}

		availableUpdates = append(availableUpdates, ChartUpdate{
			Tag:     tag,
			Version: v,
		})
	}

	sort.Sort(sort.Reverse(ChartUpdates(availableUpdates)))

	setCachedUpdates(chartPath, availableUpdates)

	return availableUpdates, nil
}

func removeDuplicates(tags []string) []string {
	m := map[string]struct{}{}
	for _, tag := range tags {
		m[tag] = struct{}{}
	}

	u := []string{}
	for k := range m {
		u = append(u, k)
	}

	return u
}

// TODO: Add caching
func PullChartVersion(helmApp *apptypes.HelmApp, licenseID string, version string) (*bytes.Buffer, error) {
	err := CreateHelmRegistryCreds(licenseID, licenseID, helmApp.ChartPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create helm credentials file")
	}
	chartGetter, err := helmgetter.NewOCIGetter()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create chart getter")
	}

	imageName := fmt.Sprintf("%s:%s", helmApp.ChartPath, version)
	data, err := chartGetter.Get(imageName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get chart %q", imageName)
	}

	return data, nil
}
