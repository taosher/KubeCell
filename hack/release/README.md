# Release Bundle Workflow

`build-bundle.sh` produces a self-contained KubeCell release directory. OCM artifacts are supplied by the OCM release process; KubeCell only copies them and records deterministic SHA-256 digests without taking over OCM objects.

Example:

```sh
hack/release/build-bundle.sh \
  --output dist/kubecell-0.1.0 \
  --ocm-bundle /path/to/ocm-hub-bundle.yaml \
  --ocm-join-bundle /path/to/ocm-join-bundle.yaml \
  --controller-image registry.example/kubecell/controller@sha256:<64-hex-digest>
```

Validate the generated bundle:

```sh
go run ./cmd/kubecell-installer doctor management --bundle dist/kubecell-0.1.0 --dry-run
go run ./cmd/kubecell-installer doctor host --bundle dist/kubecell-0.1.0 --dry-run
go run ./cmd/kubecell-installer management --bundle dist/kubecell-0.1.0
```

`doctor` is read-only by default and `--dry-run` only prints commands. Lifecycle changes require both `--apply` and `--confirm` on the corresponding command. Helm rollback and uninstall only affect the selected KubeCell release and never delete the OCM Hub, ManagedCluster, ManifestWork, or klusterlet resources.
