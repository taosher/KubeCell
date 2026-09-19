package controller

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
	hostprovider "github.com/kubecell/kubecell/internal/provider/host"
	"github.com/kubecell/kubecell/internal/provider/k3k"
	"github.com/kubecell/kubecell/internal/provider/ocm"
	"github.com/kubecell/kubecell/internal/render"
)

const virtualClusterFinalizer = "kubecell.io/virtualcluster-finalizer"

type ChildObservation struct {
	APIReady          bool
	LogicalNodeName   string
	LogicalNodeReady  bool
	NodePort          int32
	Endpoint          string
	SourceSecretName  string
	Kubeconfig        []byte
	StorageClassReady bool
	Client            k3k.ChildClient
}

type ChildObserver interface {
	Observe(context.Context, *api.VirtualCluster, []string) (ChildObservation, error)
}

type ChildObserverFactory interface {
	ForCell(context.Context, *api.Cell) (ChildObserver, error)
}

type VirtualClusterReconciler struct {
	client.Client
	ChildObserver        ChildObserver
	ChildObserverFactory ChildObserverFactory
	HostReaderFactory    VirtualNodeHostReaderFactory
	// AppsSuffix is the Ingress hostname suffix (§11.2); endpoint sync is skipped when empty.
	AppsSuffix string
}

// +kubebuilder:rbac:groups=kubecell.io,resources=virtualclusters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kubecell.io,resources=virtualclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubecell.io,resources=cells,verbs=get;list;watch
// +kubebuilder:rbac:groups=kubecell.io,resources=virtualnodeclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *VirtualClusterReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&api.VirtualCluster{}).
		Watches(&workv1.ManifestWork{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, object client.Object) []reconcile.Request {
			labels := object.GetLabels()
			namespace, name := labels[helpers.LabelVirtualClusterNamespace], labels[helpers.LabelVirtualClusterName]
			if namespace == "" || name == "" {
				return nil
			}
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}}
		})).
		Complete(r)
}

func (r *VirtualClusterReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cluster := &api.VirtualCluster{}
	if err := r.Get(ctx, request.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if cluster.DeletionTimestamp != nil {
		return r.reconcileDeletion(ctx, cluster)
	}
	if !containsString(cluster.Finalizers, virtualClusterFinalizer) {
		cluster.Finalizers = append(cluster.Finalizers, virtualClusterFinalizer)
		if err := r.Update(ctx, cluster); err != nil {
			return reconcile.Result{}, err
		}
	}
	cluster.Status.ObservedGeneration = cluster.Generation
	cell := &api.Cell{}
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.CellRef.Name, Namespace: cluster.Spec.CellRef.Namespace}, cell); err != nil {
		return reconcile.Result{}, err
	}
	class := &api.VirtualNodeClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ClassRef.Name}, class); err != nil {
		return reconcile.Result{}, err
	}
	if cell.Status.Phase != api.CellPhaseReady {
		cluster.Status.Phase = api.VirtualClusterPhaseDegraded
		setVirtualClusterCondition(cluster, "CellSelected", metav1.ConditionFalse, "CellNotReady", "referenced Cell is not ready")
		return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	derived := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	names := render.Names{HostNamespace: derived.HostNamespace, K3kCluster: derived.K3kCluster, FoundationWork: derived.FoundationWork, InstanceWork: derived.InstanceWork, AdminKubeconfig: derived.AdminKubeconfig}
	resolved, resolveErr := resolveVirtualCluster(cluster, cell, class)
	if resolveErr != nil {
		setVirtualClusterCondition(cluster, "Validated", metav1.ConditionFalse, "InvalidSpec", resolveErr.Error())
		cluster.Status.Phase = api.VirtualClusterPhaseFailed
		return reconcile.Result{}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	setVirtualClusterCondition(cluster, "Validated", metav1.ConditionTrue, "SpecAccepted", "VirtualCluster specification and immutable references are valid")
	setVirtualClusterCondition(cluster, "CellSelected", metav1.ConditionTrue, "CellReady", "VirtualCluster is bound to a ready Cell")
	setVirtualClusterCondition(cluster, "ProviderReady", metav1.ConditionTrue, "VersionResolved", "K3k and Child version profile resolved")
	resolved.HostNamespace = names.HostNamespace
	resolved.K3kClusterName = names.K3kCluster
	if cluster.Status.Resolved.HostNamespace == "" {
		cluster.Status.Resolved = resolved
	} else if !sameResolvedProvider(cluster.Status.Resolved, resolved) {
		setVirtualClusterCondition(cluster, "Validated", metav1.ConditionFalse, "ResolvedDrift", "immutable resolved provider snapshot differs from current references")
		cluster.Status.Phase = api.VirtualClusterPhaseFailed
		return reconcile.Result{}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	feasibility := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	cluster.Status.FeasibilityChecks = feasibility.Checks
	if feasibility.Decision != api.VirtualClusterPlanDecisionAccepted {
		setVirtualClusterCondition(cluster, "Feasible", conditionStatus(feasibility.Decision == api.VirtualClusterPlanDecisionAccepted), string(feasibility.Decision), "current Cell inventory cannot be accepted for provisioning")
		if feasibility.Decision == api.VirtualClusterPlanDecisionRejected {
			cluster.Status.Phase = api.VirtualClusterPhaseFailed
			return reconcile.Result{}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
		cluster.Status.Phase = api.VirtualClusterPhaseDegraded
		return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	setVirtualClusterCondition(cluster, "Feasible", metav1.ConditionTrue, "Accepted", "current Cell inventory accepts the provisioning envelope")
	// Host address auto-discovery: read Ready host nodes via the proxy, sorted by node name;
	// the InternalIP of the first node serves as the API address for every VC (§7). Both render (tls-san,
	// NetworkPolicy) and child-cluster probing share this address table.
	var hostReader VirtualNodeHostReader
	if r.HostReaderFactory != nil {
		hostReaderFromFactory, hostReaderErr := r.HostReaderFactory.ForCell(ctx, cell)
		if hostReaderErr != nil {
			setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "HostReaderUnavailable", hostReaderErr.Error())
			return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
		hostReader = hostReaderFromFactory
	}
	addresses := []string{}
	if hostReader != nil {
		hostNodes, hostNodesErr := hostReader.ListNodes(ctx)
		if hostNodesErr != nil {
			setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "HostNodeReadFailed", hostNodesErr.Error())
			return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
		addresses = discoverHostAPIAddresses(hostNodes)
	}
	if len(addresses) == 0 {
		setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "HostAddressNotFound", "no Ready Host node with an InternalIP was observed")
		return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	names.HostAPIAddresses = addresses
	foundation, err := ocm.BuildFoundationWork(*cluster, resolved, names, cell.Spec.ManagedClusterRef.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err := r.createOrUpdateWork(ctx, foundation); err != nil {
		return reconcile.Result{}, err
	}
	foundationCurrent := &workv1.ManifestWork{}
	if err := r.Get(ctx, types.NamespacedName{Name: foundation.Name, Namespace: foundation.Namespace}, foundationCurrent); err != nil {
		return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	cluster.Status.WorkRefs.Foundation = workReference(foundationCurrent)
	applyWorkStatus(&cluster.Status.WorkRefs.Foundation, foundationCurrent)
	foundationSummary := ocm.SummarizeWork(foundationCurrent)
	if foundationSummary.Failed {
		cluster.Status.Phase = api.VirtualClusterPhaseDegraded
		setVirtualClusterCondition(cluster, "FoundationReady", metav1.ConditionFalse, "WorkFailed", boundedWorkMessage(foundationSummary.Message, "Foundation ManifestWork is degraded"))
		setVirtualClusterCondition(cluster, "HostQuotaReady", metav1.ConditionFalse, "WorkFailed", "Host quota foundation was not applied")
		return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	if !foundationSummary.Applied {
		cluster.Status.Phase = api.VirtualClusterPhaseProvisioning
		setVirtualClusterCondition(cluster, "FoundationReady", metav1.ConditionFalse, "WorkProgressing", "Foundation ManifestWork is not Applied")
		setVirtualClusterCondition(cluster, "HostQuotaReady", metav1.ConditionFalse, "WorkProgressing", "Host quota foundation is not Applied")
		return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	instanceCurrent := &workv1.ManifestWork{}
	if err := r.Get(ctx, types.NamespacedName{Name: names.InstanceWork, Namespace: cell.Spec.ManagedClusterRef.Name}, instanceCurrent); err != nil {
		if !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		instanceCurrent = &workv1.ManifestWork{}
	}
	cluster.Status.WorkRefs.Instance = workReference(instanceCurrent)
	applyWorkStatus(&cluster.Status.WorkRefs.Instance, instanceCurrent)
	instanceSummary := ocm.SummarizeWork(instanceCurrent)
	if instanceSummary.Failed {
		cluster.Status.Phase = api.VirtualClusterPhaseDegraded
		setVirtualClusterCondition(cluster, "InstanceReady", metav1.ConditionFalse, "WorkFailed", boundedWorkMessage(instanceSummary.Message, "Instance ManifestWork is degraded"))
		return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	if hostReader != nil {
		quota, quotaErr := hostReader.GetResourceQuota(ctx, resolved.HostNamespace, "kubecell-workload")
		if quotaErr == nil && quota != nil {
			cluster.Status.Host.QuotaHard = quota.Status.Hard.DeepCopy()
			cluster.Status.Host.QuotaUsed = quota.Status.Used.DeepCopy()
		}
	}
	cluster.Status.Host.Namespace = resolved.HostNamespace
	projectHostQuota(&cluster.Status.Host, cluster.Status.Host.QuotaHard, cluster.Status.Host.QuotaUsed, resolved.Reservation)
	setVirtualClusterCondition(cluster, "HostQuotaReady", conditionStatus(cluster.Status.Host.QuotaReady), "HostQuotaObserved", "Host ResourceQuota feedback was observed")
	if cluster.Status.Host.ReservationExceeded {
		setVirtualClusterCondition(cluster, "ReservationExceeded", metav1.ConditionTrue, "HostUsageExceedsReservation", "Host system usage exceeds the resolved reservation")
	} else {
		setVirtualClusterCondition(cluster, "ReservationExceeded", metav1.ConditionFalse, "HostUsageWithinReservation", "Host system usage is within the resolved reservation")
	}
	observer := r.ChildObserver
	if observer == nil && r.ChildObserverFactory != nil {
		observer, err = r.ChildObserverFactory.ForCell(ctx, cell)
		if err != nil {
			setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "HostReaderUnavailable", err.Error())
			return reconcile.Result{RequeueAfter: 10 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
	}
	// Endpoint sync (§11.3): once the child cluster is ready, mirror Ingresses with class=kubecell into the instance Work.
	// The instance Work is built and applied at most once per round (mirror included), avoiding empty/full version flapping.
	// Read failures only log and continue this round with an empty mirror (a requeue in 5s retries); VC status is not flipped.
	var mirrored []*unstructured.Unstructured
	observedOK := false
	mirrorOK := false
	if ocm.IsApplied(instanceCurrent) && observer != nil {
		child, observeErr := observer.Observe(ctx, cluster, addresses)
		if observeErr != nil {
			setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "HostReadFailed", observeErr.Error())
			setVirtualClusterCondition(cluster, "APIAccessible", metav1.ConditionFalse, "ChildProbeFailed", observeErr.Error())
			setVirtualClusterCondition(cluster, "LogicalNodeReady", metav1.ConditionFalse, "ChildProbeFailed", observeErr.Error())
			setVirtualClusterCondition(cluster, "StorageReady", metav1.ConditionFalse, "ChildProbeFailed", observeErr.Error())
			// Never keep a stale child-cluster projection on observation failure (KI-16): clear it, and only degrade if previously Ready;
			// startup Provisioning semantics stay unchanged.
			cluster.Status.Child = api.ChildClusterObservation{}
			if cluster.Status.Phase == api.VirtualClusterPhaseReady {
				cluster.Status.Phase = api.VirtualClusterPhaseDegraded
			}
			return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
		setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionTrue, "HostObjectsRead", "Child Secret and Service were read through the Host observation path")
		cluster.Status.Child = api.ChildClusterObservation{APIReady: child.APIReady, LogicalNodeName: child.LogicalNodeName, LogicalNodeReady: child.LogicalNodeReady, StorageClassReady: child.StorageClassReady}
		setVirtualClusterCondition(cluster, "APIAccessible", conditionStatus(child.APIReady), "ChildAPIProbe", "Child API endpoint probe completed")
		setVirtualClusterCondition(cluster, "LogicalNodeReady", conditionStatus(child.LogicalNodeReady), "ChildNodeProbe", "Child logical Node probe completed")
		setVirtualClusterCondition(cluster, "StorageReady", conditionStatus(child.StorageClassReady), "ChildStorageClass", "approved Child StorageClass probe completed")
		// The admin kubeconfig is always published (§7 platform constant, no spec switch).
		if child.NodePort > 0 && child.Endpoint != "" {
			parsedEndpoint, endpointErr := url.Parse(child.Endpoint)
			if endpointErr != nil || parsedEndpoint.Hostname() == "" {
				setVirtualClusterCondition(cluster, "Credential", metav1.ConditionFalse, "CredentialInvalid", "Child endpoint is not a valid URL")
				return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
			}
			if err := r.publishAdminKubeconfig(ctx, cluster, child.Kubeconfig, child.SourceSecretName, child.Endpoint); err != nil {
				return reconcile.Result{}, err
			}
			setVirtualClusterCondition(cluster, "ChildIdentityReady", metav1.ConditionTrue, "CredentialPublished", "Child administrator kubeconfig was published")
			cluster.Status.Endpoint = api.EndpointStatus{Address: parsedEndpoint.Hostname(), NodePort: child.NodePort, URLHash: helpers.ProfileHash(child.Endpoint)}
		}
		if child.APIReady && child.LogicalNodeReady && hostReader != nil {
			hostPods, hostPodsErr := hostReader.ListPods(ctx, resolved.HostNamespace)
			if hostPodsErr != nil {
				setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "PlacementReadFailed", hostPodsErr.Error())
				return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
			}
			cluster.Status.Placements = projectPlacements(ctx, hostReader, resolved.HostNamespace, cluster.Status.Resolved.K3kClusterName, hostPods.Items)
		}
		// Endpoint sync (§11.3): collect the mirror once the child cluster is ready, then apply it together with the instance Work below.
		if child.APIReady && child.Client != nil {
			fetched, mirrorErr := mirrorChildIngresses(ctx, cluster, resolved, r.AppsSuffix, child.Client)
			if mirrorErr != nil {
				ctrl.LoggerFrom(ctx).Error(mirrorErr, "failed to mirror Child ingresses", "virtualcluster", cluster.Name)
			} else {
				mirrored = fetched
				mirrorOK = true
			}
		}
		observedOK = true
	}
	// Write the instance Work only when safe: on first creation, or when both observation and mirror succeed.
	// Skip the apply on transient read failures (gate closed, mirror error, child not ready),
	// otherwise stored mirror objects would be deleted and self-oscillate with work-agent.
	if instanceCurrent.Name == "" || (observedOK && mirrorOK) {
		instance, err := ocm.BuildInstanceWork(*cluster, resolved, names, cell.Spec.ManagedClusterRef.Name, mirrored)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := r.createOrUpdateWork(ctx, instance); err != nil {
			return reconcile.Result{}, err
		}
		refreshed := &workv1.ManifestWork{}
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, refreshed); err != nil {
			return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
		instanceCurrent = refreshed
		cluster.Status.WorkRefs.Instance = workReference(instanceCurrent)
		applyWorkStatus(&cluster.Status.WorkRefs.Instance, instanceCurrent)
		instanceSummary = ocm.SummarizeWork(instanceCurrent)
	}
	if cluster.Status.Child.APIReady && cluster.Status.Child.LogicalNodeReady && cluster.Status.Child.StorageClassReady {
		cluster.Status.Phase = api.VirtualClusterPhaseReady
		setVirtualClusterCondition(cluster, "APIAccessible", metav1.ConditionTrue, "ChildReady", "Child API endpoint is accessible")
		setVirtualClusterCondition(cluster, "LogicalNodeReady", metav1.ConditionTrue, "ChildReady", "Child logical Node is ready")
		setVirtualClusterCondition(cluster, "StorageReady", metav1.ConditionTrue, "StorageClassReady", "approved Child StorageClass is ready")
	} else {
		cluster.Status.Phase = api.VirtualClusterPhaseProvisioning
	}
	if foundationSummary.Applied {
		setVirtualClusterCondition(cluster, "FoundationReady", metav1.ConditionTrue, "WorkApplied", "Foundation ManifestWork is Applied")
	}
	if instanceSummary.Applied {
		setVirtualClusterCondition(cluster, "InstanceReady", metav1.ConditionTrue, "WorkApplied", "Instance ManifestWork is Applied")
	}
	if observer == nil {
		setVirtualClusterCondition(cluster, "HostReadReady", metav1.ConditionFalse, "HostReaderUnavailable", "Child observation path is not configured")
	}
	return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
}

// mirrorChildIngresses collects child-cluster Ingresses plus backend Service/Endpoints for render.
// Unreadable backends are skipped per rule (child-side eventual consistency); no sync happens when appsSuffix is empty.
func mirrorChildIngresses(ctx context.Context, cluster *api.VirtualCluster, resolved api.ResolvedSnapshot, appsSuffix string, client k3k.ChildClient) ([]*unstructured.Unstructured, error) {
	if client == nil || strings.TrimSpace(appsSuffix) == "" {
		return nil, nil
	}
	list, err := client.ListIngresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Child ingresses: %w", err)
	}
	services := map[string]*corev1.Service{}
	endpoints := map[string]*corev1.Endpoints{}
	backends := make([]render.ChildIngressBackend, 0)
	for index := range list.Items {
		ingress := list.Items[index]
		if ingress.Spec.IngressClassName == nil || *ingress.Spec.IngressClassName != platform.IngressClassName {
			continue
		}
		for _, rule := range ingress.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					continue
				}
				key := ingress.Namespace + "/" + path.Backend.Service.Name
				service, ok := services[key]
				if !ok {
					fetched, fetchErr := client.GetService(ctx, ingress.Namespace, path.Backend.Service.Name)
					if fetchErr != nil {
						continue
					}
					service = fetched
					services[key] = service
				}
				var endpointsForBackend *corev1.Endpoints
				if cached, ok := endpoints[key]; ok {
					endpointsForBackend = cached
				} else if fetched, fetchErr := client.GetEndpoints(ctx, ingress.Namespace, path.Backend.Service.Name); fetchErr == nil {
					endpointsForBackend = fetched
					endpoints[key] = fetched
				}
				backends = append(backends, render.ChildIngressBackend{Ingress: ingress, Service: service, Endpoints: endpointsForBackend})
			}
		}
	}
	return render.RenderIngressMirror(*cluster, resolved, appsSuffix, backends)
}

// discoverHostAPIAddresses returns InternalIPs of Ready host nodes sorted by node name (§7).
func discoverHostAPIAddresses(nodes *corev1.NodeList) []string {
	if nodes == nil {
		return nil
	}
	type namedAddress struct {
		name    string
		address string
	}
	observed := make([]namedAddress, 0, len(nodes.Items))
	for index := range nodes.Items {
		node := &nodes.Items[index]
		ready := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			continue
		}
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP && address.Address != "" {
				observed = append(observed, namedAddress{name: node.Name, address: address.Address})
				break
			}
		}
	}
	sort.Slice(observed, func(i, j int) bool { return observed[i].name < observed[j].name })
	result := make([]string, 0, len(observed))
	for _, entry := range observed {
		result = append(result, entry.address)
	}
	return result
}

func projectHostQuota(observation *api.HostClusterObservation, hard, used corev1.ResourceList, reservation api.ReservationSpec) {
	if observation == nil {
		return
	}
	observation.QuotaReady = len(hard) > 0 && len(used) > 0
	if !observation.QuotaReady {
		observation.TenantHeadroom = nil
		observation.Reservation = nil
		observation.ReservationExceeded = false
		return
	}
	combined := corev1.ResourceList{}
	for name, quantity := range reservation.ControlPlane {
		combined[name] = quantity.DeepCopy()
	}
	for name, quantity := range reservation.ReflectedSystem {
		current := combined[name]
		current.Add(quantity)
		combined[name] = current
	}
	observation.Reservation = combined
	observation.TenantHeadroom = hostprovider.TenantHeadroom(hard, used, combined)
	observation.ReservationExceeded = hostprovider.ReservationExceeded(hard, used, combined)
}

func boundedWorkMessage(message, fallback string) string {
	if message == "" {
		return fallback
	}
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

func applyWorkStatus(reference *api.WorkReference, work *workv1.ManifestWork) {
	if reference == nil || work == nil {
		return
	}
	summary := ocm.SummarizeWork(work)
	reference.Applied = summary.Applied
	reference.Progressing = summary.Progressing
	reference.Failed = summary.Failed
}

func (r *VirtualClusterReconciler) publishAdminKubeconfig(ctx context.Context, cluster *api.VirtualCluster, data []byte, sourceSecretName, endpoint string) error {
	name := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID).AdminKubeconfig
	desired := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace, Labels: map[string]string{helpers.LabelManagedBy: helpers.ManagedByKubecell, helpers.LabelVirtualClusterUID: string(cluster.UID)}, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(cluster, api.GroupVersion.WithKind("VirtualCluster"))}}, Data: map[string][]byte{"kubeconfig.yaml": append([]byte(nil), data...)}, Type: corev1.SecretTypeOpaque}
	current := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cluster.Namespace}, current)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		desired.ResourceVersion = current.ResourceVersion
		if err := r.Update(ctx, desired); err != nil {
			return err
		}
	}
	cluster.Status.Credential = api.CredentialStatus{SecretName: name, SourceSecretName: sourceSecretName, EndpointHash: helpers.ProfileHash(endpoint), ObservedResourceVersion: current.ResourceVersion}
	return nil
}

func (r *VirtualClusterReconciler) createOrUpdateWork(ctx context.Context, desired *workv1.ManifestWork) error {
	current := &workv1.ManifestWork{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	if err := r.Get(ctx, key, current); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err := ValidateWorkReference(current, desired.Labels[helpers.LabelVirtualClusterUID], ""); err != nil {
		return err
	}
	if !helpers.MatchesOwnedHostLabels(current.Labels,
		desired.Labels[helpers.LabelCellName],
		desired.Labels[helpers.LabelVirtualClusterNamespace],
		desired.Labels[helpers.LabelVirtualClusterName],
		desired.Labels[helpers.LabelVirtualClusterUID],
	) {
		return fmt.Errorf("ManifestWork %s/%s has foreign ownership labels", current.Namespace, current.Name)
	}
	return r.Patch(ctx, desired, client.Apply, client.ForceOwnership, client.FieldOwner("kubecell-controller"))
}

func (r *VirtualClusterReconciler) reconcileDeletion(ctx context.Context, cluster *api.VirtualCluster) (reconcile.Result, error) {
	if !containsString(cluster.Finalizers, virtualClusterFinalizer) {
		return reconcile.Result{}, nil
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	if err := r.deleteOwnedSecret(ctx, cluster.Namespace, names.AdminKubeconfig, string(cluster.UID)); err != nil {
		return reconcile.Result{}, err
	}

	// Deletion lookup does not rely on the status snapshot: a VC deleted before its status persists can still find its own Work (KI-7).
	managedClusterName := cluster.Status.Resolved.ManagedClusterName
	if managedClusterName == "" {
		referenced := &api.Cell{}
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.CellRef.Name, Namespace: cluster.Spec.CellRef.Namespace}, referenced); err != nil {
			cluster.Status.Phase = api.VirtualClusterPhaseDeleting
			setVirtualClusterCondition(cluster, "CleanupBlocked", metav1.ConditionTrue, "CellUnavailable", "cannot resolve ManagedCluster for cleanup without a resolved snapshot")
			return reconcile.Result{RequeueAfter: 15 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
		}
		managedClusterName = referenced.Spec.ManagedClusterRef.Name
	}
	instance, instanceErr := r.getWork(ctx, managedClusterName, names.InstanceWork)
	foundation, foundationErr := r.getWork(ctx, managedClusterName, names.FoundationWork)
	if instanceErr != nil && !apierrors.IsNotFound(instanceErr) {
		return reconcile.Result{}, instanceErr
	}
	if foundationErr != nil && !apierrors.IsNotFound(foundationErr) {
		return reconcile.Result{}, foundationErr
	}
	plan := BuildDeletionPlan(instance, foundation, string(cluster.UID))
	if plan.Blocked {
		cluster.Status.Phase = api.VirtualClusterPhaseDeleting
		setVirtualClusterCondition(cluster, "CleanupBlocked", metav1.ConditionTrue, "ForeignObject", plan.Reason)
		return reconcile.Result{RequeueAfter: 15 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	if instance != nil {
		if err := r.Delete(ctx, instance); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		cluster.Status.Phase = api.VirtualClusterPhaseDeleting
		setVirtualClusterCondition(cluster, "CleanupBlocked", metav1.ConditionFalse, "InstanceDeletionRequested", "Instance Work deletion requested; waiting for Host cleanup")
		return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	if foundation != nil {
		if err := r.Delete(ctx, foundation); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		cluster.Status.Phase = api.VirtualClusterPhaseDeleting
		setVirtualClusterCondition(cluster, "CleanupBlocked", metav1.ConditionFalse, "FoundationDeletionRequested", "Foundation Work deletion requested; waiting for Host cleanup")
		return reconcile.Result{RequeueAfter: 5 * time.Second}, updateVirtualClusterStatus(ctx, r.Client, cluster)
	}
	cluster.Status.Phase = api.VirtualClusterPhaseDeleting
	cluster.Finalizers = removeString(cluster.Finalizers, virtualClusterFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *VirtualClusterReconciler) getWork(ctx context.Context, namespace, name string) (*workv1.ManifestWork, error) {
	if namespace == "" || name == "" {
		return nil, nil
	}
	work := &workv1.ManifestWork{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, work); err != nil {
		return nil, err
	}
	return work, nil
}

func (r *VirtualClusterReconciler) deleteOwnedSecret(ctx context.Context, namespace, name, uid string) error {
	if name == "" {
		return nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if secret.Labels[helpers.LabelVirtualClusterUID] != uid {
		return fmt.Errorf("published Secret %s/%s has foreign VirtualCluster UID", namespace, name)
	}
	if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func sameResolvedProvider(current, desired api.ResolvedSnapshot) bool {
	return current.CellUID == desired.CellUID &&
		current.ManagedClusterName == desired.ManagedClusterName &&
		current.ClassUID == desired.ClassUID &&
		current.ProfileHash == desired.ProfileHash &&
		current.MachineProfileName == desired.MachineProfileName &&
		current.K3kVersion == desired.K3kVersion &&
		current.ChildK3sVersion == desired.ChildK3sVersion &&
		current.K3kChildVersion == desired.K3kChildVersion &&
		current.ChildImageTag == desired.ChildImageTag &&
		current.HostNamespace == desired.HostNamespace &&
		current.K3kClusterName == desired.K3kClusterName &&
		apiequality.Semantic.DeepEqual(current.Quota, desired.Quota) &&
		apiequality.Semantic.DeepEqual(current.Reservation, desired.Reservation) &&
		apiequality.Semantic.DeepEqual(current.Storage, desired.Storage) &&
		apiequality.Semantic.DeepEqual(current.AcceleratorKeys, desired.AcceleratorKeys) &&
		current.HostPathTenantName == desired.HostPathTenantName &&
		current.HostPathTenantRoot == desired.HostPathTenantRoot
}

func workReference(work *workv1.ManifestWork) api.WorkReference {
	return api.WorkReference{Namespace: work.Namespace, Name: work.Name, UID: string(work.UID)}
}
func setVirtualClusterCondition(cluster *api.VirtualCluster, conditionType string, status metav1.ConditionStatus, reason, message string) {
	cluster.Status.Conditions = helpers.SetCondition(cluster.Status.Conditions, metav1.Condition{Type: conditionType, Status: status, Reason: reason, Message: message, ObservedGeneration: cluster.Generation}, metav1.Now())
}
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

var _ = clusterv1.GroupVersion
var _ = fmt.Sprintf
