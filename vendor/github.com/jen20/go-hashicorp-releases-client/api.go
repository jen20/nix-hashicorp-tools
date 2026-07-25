package releases

import (
	"fmt"
	"strings"
	"time"
)

// LicenseClass represents a class of license under which releases are offered.
type LicenseClass string

var (
	// LicenseClassAny specifies that releases are not to be filtered by license class.
	LicenseClassAny *LicenseClass = ptr[LicenseClass]("")

	// LicenseClassOSS specifies that releases should be filtered to only include those under an OSS or BSL license.
	LicenseClassOSS *LicenseClass = ptr[LicenseClass]("oss")

	// LicenseClassEnterprise specifies that releases should be filtered to only include those under an Enterprise
	// license.
	LicenseClassEnterprise *LicenseClass = ptr[LicenseClass]("enterprise")

	// LicenseClassHCP specifies that releases should be filtered to include only those intended for use with HashiCorp
	// Cloud Platform.
	LicenseClassHCP *LicenseClass = ptr[LicenseClass]("hcp")
)

func licenseClassNames() string {
	elems := []string{
		fmt.Sprintf("%q", *LicenseClassAny),
		fmt.Sprintf("%q", *LicenseClassOSS),
		fmt.Sprintf("%q", *LicenseClassEnterprise),
		fmt.Sprintf("%q", *LicenseClassHCP),
	}

	return fmt.Sprintf("%s or %s", strings.Join(elems[:len(elems)-2], ", "), elems[len(elems)-1])
}

// ReleaseState represents whether a release of a product is within support, out of support, or has been withdrawn.
type ReleaseState string

var (
	//ReleaseStateSupported specifies that a release of a product is within support.
	ReleaseStateSupported ReleaseState = "supported"

	//ReleaseStateUnsupported specifies that a release of a product is out of support.
	ReleaseStateUnsupported ReleaseState = "unsupported"

	//ReleaseStateWithdrawn specifies that a release of a product has been withdrawn.
	ReleaseStateWithdrawn ReleaseState = "withdrawn"
)

// BuildInfo provides metadata about a specific build variant of a product release.
type BuildInfo struct {
	// Arch is the target CPU architecture for this build.
	Arch string `json:"arch"`

	// OS is the target operating system for this build.
	OS string `json:"os"`

	// Unsupported is set to true if this build is provided only for convenience, but is not supported by HashiCorp.
	Unsupported bool `json:"unsupported"`

	// URL is a URL from which this build may be downloaded.
	URL string `json:"url"`
}

// ReleaseStatus provides information about the status of this release.
type ReleaseStatus struct {
	// Message provides information about the most recent change to State, and is required if State is "withdrawn".
	Message string `json:"message"`

	// State indicates whether this release is supported, unsupported or withdrawn.
	State ReleaseState `json:"state"`
}

// ReleaseInfo provides metadata about a specific release of a product.
type ReleaseInfo struct {
	// Builds provides metadata about each build variant of this release.
	Builds []BuildInfo `json:"builds"`

	// DockerNameTag is a docker image name and tag for this release in the format `name`:`tag`.
	DockerNameTag string `json:"docker_name_tag"`

	// IsPrerelease is set to true if this product release is a prerelease.
	IsPrerelease bool `json:"is_prerelease"`

	// LicenseClass is the class of license which applies to this release.
	LicenseClass LicenseClass `json:"license_class"`

	// Name is the name of the product of which this is a release.
	Name string `json:"name"`

	// Status provides information about the status of this release.
	Status ReleaseStatus `json:"status"`

	// TimestampCreated is the time at which this release was first created.
	TimestampCreated time.Time `json:"timestamp_created"`

	// TimestampUpdated is the time at which this release was last updated. Note that this does not include changes
	// in support status, which are tracked in Status.
	TimestampUpdated time.Time `json:"timestamp_updated"`

	// URLBlogpost is a URL for a blog post announcing this release. This may refer to the post announcing the major
	// or minor release, since patch versions are not typically announced this way.
	URLBlogpost string `json:"url_blogpost"`

	// URLChangelog is a URL for the change log file covering this release.
	URLChangelog string `json:"url_changelog"`

	// URLDockerRegistryDockerHub is a URL for the Docker image for this release on Docker Hub.
	URLDockerRegistryDockerhub string `json:"url_docker_registry_dockerhub"`

	// URLDockerRegistryECR is a URL for the Docker image for this release on a public AWS Elastic Container Repository.
	URLDockerRegistryECR string `json:"url_docker_registry_ecr"`

	// URLLicense is a URL for the text of the license which applies to this release.
	URLLicense string `json:"url_license"`

	// URLProjectWebsite is a URL for the main website for the product of which this is a release.
	URLProjectWebsite string `json:"url_project_website"`

	// URLReleaseNotes is a URL for any release notes pertaining to this release.
	URLReleaseNotes string `json:"url_release_notes"`

	// URLSHASUMs is a URL containing the SHA 256 checksums of each build of this release.
	URLSHASUMs string `json:"url_shasums"`

	// URLSHASUMsSignatures is a set of URLs, each of which contains a detached GPG signature for the checksums file
	// at URLSHASUMs. Key IDs may be noted in the filename.
	URLSHASUMsSignatures []string `json:"url_shasums_signatures"`

	// URLSourceRepository is a URL for the product source code repository. This is typically empty for enterprise
	// products for which source is not publicly available.
	URLSourceRepository string `json:"url_source_repository"`

	// Version is the version number of this release.
	Version string `json:"version"`
}
