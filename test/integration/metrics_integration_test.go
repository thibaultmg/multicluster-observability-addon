package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	prometheusapi "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring"
	prometheusalpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	addonctrl "github.com/rhobs/multicluster-observability-addon/internal/controllers/addon"
	"github.com/rhobs/multicluster-observability-addon/internal/metrics/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterapiv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIntegration_Metrics(t *testing.T) {
	scheme := newScheme()

	// Set up the test environment
	spokeName := "managed-cluster-1"
	obsNamespace := "open-cluster-management-observability"

	// promAgentConfig := types.NamespacedName{Namespace: obsNamespace, Name: resource.DefaultPlatformMetricsCollectorApp}
	// envoyConfig := types.NamespacedName{Namespace: obsNamespace, Name: resource.DefaultPlatformEnvoyConfigMap}
	addonConfigName := "addon-config"
	remoteWriteSecrets := newRemoteWriteSecrets(obsNamespace)
	prometheusAgent := newPrometeheusAgent("titi", obsNamespace)
	clusterSet := newManagedClusterSet("titi")
	placement := newPlacement("titi", obsNamespace, clusterSet.Name)
	cma := ClusterManagementAddonBuilder(placement).WithPrometheusAgent(*prometheusAgent).Build()
	resources := []client.Object{
		newManagedCluster(spokeName),
		newNamespace(spokeName),
		newNamespace(obsNamespace),
		mewImagesListConfigMap(obsNamespace),
		remoteWriteSecrets[0],
		remoteWriteSecrets[1],
		cma,
		prometheusAgent,
		clusterSet,
	}
	k8sClient, err := client.New(restCfgHub, client.Options{Scheme: scheme})
	require.NoError(t, err)
	require.NoError(t, applyResources(context.Background(), k8sClient, resources))

	mca := NewManagedClusterAddon(spokeName).WithPrometheusAgent(prometheusAgent).Build()
	// mca.Status.ConfigReferences = []addonapiv1alpha1.ConfigReference{
	// 	{
	// 		ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
	// 			Group:    addonapiv1alpha1.GroupName,
	// 			Resource: addon.AddonDeploymentConfigResource,
	// 		},
	// 		ConfigReferent: addonapiv1alpha1.ConfigReferent{
	// 			Namespace: promAgentConfig.Namespace,
	// 			Name:      "addon-config",
	// 		},
	// 		DesiredConfig: &addonapiv1alpha1.ConfigSpecHash{
	// 			ConfigReferent: addonapiv1alpha1.ConfigReferent{
	// 				Namespace: promAgentConfig.Namespace,
	// 				Name:      "addon-config",
	// 			},
	// 			SpecHash: "1234",
	// 		},
	// 	},
	// 	{
	// 		ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
	// 			Group:    prometheusapi.GroupName,
	// 			Resource: prometheusalpha1.PrometheusAgentName,
	// 		},
	// 		ConfigReferent: addonapiv1alpha1.ConfigReferent{
	// 			Namespace: promAgentConfig.Namespace,
	// 			Name:      promAgentConfig.Name,
	// 		},
	// 		DesiredConfig: &addonapiv1alpha1.ConfigSpecHash{
	// 			ConfigReferent: addonapiv1alpha1.ConfigReferent{
	// 				Namespace: promAgentConfig.Namespace,
	// 				Name:      promAgentConfig.Name,
	// 			},
	// 			SpecHash: "1234",
	// 		},
	// 	},
	// 	{
	// 		ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
	// 			Group:    "",
	// 			Resource: "configmaps",
	// 		},
	// 		ConfigReferent: addonapiv1alpha1.ConfigReferent{
	// 			Namespace: envoyConfig.Namespace,
	// 			Name:      envoyConfig.Name,
	// 		},
	// 		DesiredConfig: &addonapiv1alpha1.ConfigSpecHash{
	// 			ConfigReferent: addonapiv1alpha1.ConfigReferent{
	// 				Namespace: envoyConfig.Namespace,
	// 				Name:      envoyConfig.Name,
	// 			},
	// 			SpecHash: "1234",
	// 		},
	// 	},
	// }
	require.NoError(t, setOwner(k8sClient, mca, cma))

	adc := newAddonDeploymentConfig(obsNamespace, addonConfigName).WithPlatformMetricsCollection().WithPlatformHubEndpoint("https://gogo.go").Build()
	adc.Spec.AgentInstallNamespace = spokeName
	require.NoError(t, setOwner(k8sClient, adc, cma))

	resources = []client.Object{mca, adc}
	require.NoError(t, applyResources(context.Background(), k8sClient, resources))

	// // client.ObjectKey{Name: addon.Name, Namespace: obj.GetNamespace()}
	// fmt.Println(mca.ObjectMeta.ResourceVersion)
	// mcaCur := &addonapiv1alpha1.ManagedClusterAddOn{}
	// k8sClient.Get(context.Background(), client.ObjectKeyFromObject(mca), mcaCur)
	// fmt.Println(mca.ObjectMeta.ResourceVersion)

	// update the mca status to have the addon config

	// require.NoError(t, k8sClient.Status().Update(context.Background(), mca))

	// Start the controller
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr, err := addonctrl.NewAddonManager(ctx, restCfgHub, scheme)
	require.NoError(t, err)

	require.NoError(t, mgr.Start(ctx))
	require.NoError(t, waitForController(ctx, k8sClient))

	// Ensure that the manifest work was created with the prometheus agent
	manifestWork := &workv1.ManifestWork{}
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 15*time.Second, false, func(ctx context.Context) (bool, error) {
		// Print MCA
		curMca := &addonapiv1alpha1.ManagedClusterAddOn{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(mca), curMca))
		data, err := json.Marshal(curMca)
		require.NoError(t, err)
		fmt.Println("MCA")
		fmt.Println(string(data))
		// Print CMA
		curCma := &addonapiv1alpha1.ClusterManagementAddOn{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cma), curCma))
		data, err = json.Marshal(curCma)
		require.NoError(t, err)
		fmt.Println("CMA")
		fmt.Println(string(data))

		manifestWorkList := &workv1.ManifestWorkList{}
		err = k8sClient.List(ctx, manifestWorkList, client.InNamespace(spokeName))
		if err != nil {
			fmt.Println("Ping Err A", err)
			return false, err
		}

		if len(manifestWorkList.Items) == 0 {
			fmt.Println("Ping Err B", err)
			return false, nil
		}

		if len(manifestWorkList.Items) > 1 {
			fmt.Println("Ping Err C", err)
			return false, fmt.Errorf("expected 1 manifestwork, got %d", len(manifestWorkList.Items))
		}

		manifestWork = &manifestWorkList.Items[0]

		return true, nil
	})
	time.Sleep(10 * time.Minute)
	require.NoError(t, err)

	dec := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	var found bool
	for _, resource := range manifestWork.Spec.Workload.Manifests {
		obj, _, err := dec.Decode(resource.Raw, nil, nil)
		assert.NoError(t, err)

		if obj.GetObjectKind().GroupVersionKind().Group == prometheusapi.GroupName && obj.GetObjectKind().GroupVersionKind().Kind == prometheusalpha1.PrometheusAgentsKind {
			found = true
			break
		}
	}
	assert.Truef(t, found, "expected prometheus agent in manifest work")
}

// func newManagedClusterAddon(ns string) *addonapiv1alpha1.ManagedClusterAddOn {
// 	return &addonapiv1alpha1.ManagedClusterAddOn{
// 		ObjectMeta: ctrl.ObjectMeta{
// 			Name:      addon.Name,
// 			Namespace: ns,
// 		},
// 		Spec: addonapiv1alpha1.ManagedClusterAddOnSpec{
// 			InstallNamespace: "foo",
// 		},
// 	}
// }

func mewImagesListConfigMap(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "images-list",
			Namespace: ns,
		},
		Data: map[string]string{
			"prometheus_operator":        "operator-image",
			"prometheus_config_reloader": "reloader-image",
			"kube_rbac_proxy":            "proxy-image",
		},
	}
}

func newRemoteWriteSecrets(ns string) []*corev1.Secret {
	return []*corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.ClientCertSecretName,
				Namespace: ns,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.HubCASecretName,
				Namespace: ns,
			},
		},
	}
}

func newPrometeheusAgent(name, namespace string) *prometheusalpha1.PrometheusAgent {
	return &prometheusalpha1.PrometheusAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newPlacement(name, namespace, placementName string) *clusterapiv1beta1.Placement {
	return &clusterapiv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: clusterapiv1beta1.PlacementSpec{
			ClusterSets: []string{placementName},
		},
	}
}

func newManagedClusterSet(name string) *clusterapiv1beta2.ManagedClusterSet {
	return &clusterapiv1beta2.ManagedClusterSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterapiv1beta2.ManagedClusterSetSpec{
			ClusterSelector: clusterapiv1beta2.ManagedClusterSelector{
				SelectorType: clusterapiv1beta2.LabelSelector,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{},
				},
			},
		},
	}
}

type managedClusterAddonBuilder addonapiv1alpha1.ManagedClusterAddOn

func NewManagedClusterAddon(ns string) *managedClusterAddonBuilder {
	return &managedClusterAddonBuilder{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      addon.Name,
			Namespace: ns,
		},
		Spec: addonapiv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: "foo",
		},
	}
}

func (m *managedClusterAddonBuilder) WithPrometheusAgent(promAgent *prometheusalpha1.PrometheusAgent) *managedClusterAddonBuilder {
	m.Spec.Configs = append(m.Spec.Configs, addonapiv1alpha1.AddOnConfig{
		ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
			Group:    "monitoring.coreos.com",
			Resource: prometheusalpha1.PrometheusAgentName,
		},
		ConfigReferent: addonapiv1alpha1.ConfigReferent{
			Namespace: promAgent.Namespace,
			Name:      promAgent.Name,
		},
	})

	return m
}

func (m *managedClusterAddonBuilder) Build() *addonapiv1alpha1.ManagedClusterAddOn {
	return (*addonapiv1alpha1.ManagedClusterAddOn)(m)
}

type clusterManagementAddonBuilder addonapiv1alpha1.ClusterManagementAddOn

func ClusterManagementAddonBuilder(placement *clusterapiv1beta1.Placement) *clusterManagementAddonBuilder {
	return &clusterManagementAddonBuilder{
		ObjectMeta: metav1.ObjectMeta{
			Name: addon.Name,
		},
		Spec: addonapiv1alpha1.ClusterManagementAddOnSpec{
			InstallStrategy: addonapiv1alpha1.InstallStrategy{
				Type: addonapiv1alpha1.AddonInstallStrategyPlacements,
				Placements: []addonapiv1alpha1.PlacementStrategy{
					{
						PlacementRef: addonapiv1alpha1.PlacementRef{
							Namespace: placement.Namespace,
							Name:      placement.Name,
						},
					},
				},
			},
			SupportedConfigs: []addonapiv1alpha1.ConfigMeta{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "monitoring.coreos.com",
						Resource: prometheusalpha1.PrometheusAgentName,
					},
				},
			},
		},
	}
}

func (c *clusterManagementAddonBuilder) WithPrometheusAgent(promAgent prometheusalpha1.PrometheusAgent) *clusterManagementAddonBuilder {
	c.Spec.InstallStrategy.Placements[0].Configs = append(c.Spec.InstallStrategy.Placements[0].Configs, addonapiv1alpha1.AddOnConfig{
		ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
			Group:    "monitoring.coreos.com",
			Resource: prometheusalpha1.PrometheusAgentName,
		},
		ConfigReferent: addonapiv1alpha1.ConfigReferent{
			Namespace: promAgent.Namespace,
			Name:      promAgent.Name,
		},
	})

	return c
}

func (c *clusterManagementAddonBuilder) Build() *addonapiv1alpha1.ClusterManagementAddOn {
	return (*addonapiv1alpha1.ClusterManagementAddOn)(c)
}
