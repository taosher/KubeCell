package profile

import "fmt"

type ChildVersion struct {
	Canonical string
	K3kSpec   string
	ImageTag  string
}

var supportedChildVersions = map[string]ChildVersion{
	"v1.34.2+k3s1": {
		Canonical: "v1.34.2+k3s1",
		K3kSpec:   "v1.34.2-k3s1",
		ImageTag:  "v1.34.2-k3s1",
	},
}

func ResolveChildVersion(canonical string) (ChildVersion, error) {
	resolved, ok := supportedChildVersions[canonical]
	if !ok {
		return ChildVersion{}, fmt.Errorf("Child K3s version %q is not certified for the selected K3k provider", canonical)
	}
	return resolved, nil
}
