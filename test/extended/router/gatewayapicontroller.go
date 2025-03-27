package router

import (
	"context"
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	//exutil "github.com/openshift/origin/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	//admissionapi "k8s.io/pod-security-admission/api"

	sailv1 "github.com/istio-ecosystem/sail-operator/api/v1"
	operatorclient "github.com/openshift/cluster-ingress-operator/pkg/operator/client"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = g.Describe("[sig-network][OCPFeatureGate:GatewayAPIController][Feature:Router][apigroup:gateway.networking.k8s.io]", func() {
	defer g.GinkgoRecover()
	// var (
	// 	oc = exutil.NewCLIWithPodSecurityLevel("gatewayapi-controller", admissionapi.LevelBaseline)
	// )
	const (
		// The expected OSSM subscription name.
		expectedSubscriptionName = "servicemeshoperator3"
		// The expected OSSM catalog source name.
		expectedCatalogSourceName = "redhat-operators"
		// The expected catalog source namespace.
		expectedCatalogSourceNamespace = "openshift-marketplace"
		// The gatewayclass name used to create ossm and other gateway api resources.
		gatewayClassName = "openshift-default"
		// gatewayClassControllerName is the name that must be used to create a supported gatewayClass.
		gatewayClassControllerName = "openshift.io/gateway-controller"
	)
	g.BeforeEach(func() {
		gwc, err := createGWC(gatewayClassName, gatewayClassControllerName)
		if err != nil {
			e2e.Logf("Gateway Class %s already exists, or has failed to be created, checking its status", gwc.Name)
		}

	})

	g.Describe("Verify Gateway API controller resources are created", func() {
		g.It("and ensure OSSM operator is installed after creating gatewayclass", func() {
			_, err := checkGatewayClass(gatewayClassName)
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("GatewayClass %s and OSSM Operator successfully installed!", gatewayClassName)
		})
		g.It("and ensure OSSM related resources are created", func() {
			kubeConfig, errC := config.GetConfig()
			o.Expect(errC).NotTo(o.HaveOccurred())
			kubeClient, errC := operatorclient.NewClient(kubeConfig)
			o.Expect(errC).NotTo(o.HaveOccurred())
			kclient := kubeClient
			// check the subscription
			subscription := &operatorsv1alpha1.Subscription{}
			ns := types.NamespacedName{Namespace: "openshift-operators", Name: expectedSubscriptionName}
			errSub := wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 30*time.Second, false, func(context context.Context) (bool, error) {
				if errSub := kclient.Get(context, ns, subscription); errSub != nil {
					e2e.Logf("failed to get subscription %s, retrying...", expectedSubscriptionName)
					return false, nil
				}
				e2e.Logf("Found subscription %s", subscription.Name)
				return true, nil
			})
			if errSub != nil {
				e2e.Failf("Expected ISTIO subscription %s not found", expectedSubscriptionName)
			}

			csv := &operatorsv1alpha1.ClusterServiceVersion{}
			ns = types.NamespacedName{Namespace: "openshift-operators", Name: "servicemeshoperator3.v3.0.0"}
			errCSV := wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 30*time.Second, false, func(context context.Context) (bool, error) {
				if errCSV := kclient.Get(context, ns, csv); errCSV != nil {
					e2e.Logf("failed to get Cluster Service Version %s, retrying...", "servicemeshoperator3.v3.0.0")
					return false, nil
				}
				e2e.Logf("Found the CSV %s", csv.Name)
				return true, nil
			})
			if errCSV != nil {
				e2e.Failf("Expected ISTIO CSV %s not found", "servicemeshoperator3.v3.0.0")
			}

			coreClient, err := clientset.NewForConfig(kubeConfig)
			podList, err := coreClient.CoreV1().Pods("openshift-operators").List(context.Background(), metav1.ListOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podList).To(o.ContainSubstring(`servicemesh-operator3`))

			podList, err = coreClient.CoreV1().Pods("openshift-ingress").List(context.Background(), metav1.ListOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podList).To(o.ContainSubstring(`istiod-openshift-gateway`))

			istio := &sailv1.Istio{}
			ns = types.NamespacedName{Namespace: "openshift-ingress", Name: "openshift-gateway"}
			errIstio := wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 30*time.Second, false, func(context context.Context) (bool, error) {
				if errIstio := kclient.Get(context, ns, istio); errIstio != nil {
					e2e.Logf("failed to get Istio CR %s, retrying...", "openshift-gateway")
					return false, nil
				}
				if istio.Status.GetCondition(sailv1.IstioConditionReady).Status == metav1.ConditionTrue {
					e2e.Logf("Found Istio %s/%s, and it reports ready", istio.Namespace, istio.Name)
					return true, nil
				}
				e2e.Logf("Found Istio %s/%s, but it isn't ready.  Retrying...", istio.Namespace, istio.Name)
				return false, nil
			})
			if errIstio != nil {
				e2e.Failf("Expected ISTIO CR %s not found", "openshift-gateway")
			}

		})
	})
})

func checkGatewayClass(name string) (*gatewayapiv1.GatewayClass, error) {
	kubeConfig, errC := config.GetConfig()
	o.Expect(errC).NotTo(o.HaveOccurred())
	kubeClient, errC := operatorclient.NewClient(kubeConfig)
	o.Expect(errC).NotTo(o.HaveOccurred())
	kclient := kubeClient
	gatewayClass := &gatewayapiv1.GatewayClass{}
	nsName := types.NamespacedName{Namespace: "", Name: name}

	err := wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 2*time.Minute, false, func(context context.Context) (bool, error) {
		if err := kclient.Get(context, nsName, gatewayClass); err != nil {
			e2e.Logf("failed to get gatewayclass %s, retrying...", name)
			return false, nil
		}
		for _, condition := range gatewayClass.Status.Conditions {
			if condition.Type == string(gatewayapiv1.GatewayClassConditionStatusAccepted) {
				if condition.Status == metav1.ConditionTrue {
					return true, nil
				}
			}
		}
		e2e.Logf("Found gatewayclass %s but it is not accepted, retrying...", name)
		return false, nil
	})

	if err != nil {
		return nil, fmt.Errorf("Gatewayclass %s is not accepted", name)
	}
	e2e.Logf("Gateway Class %s is created and accpeted", name)
	return gatewayClass, nil
}

func createGWC(name, controllerName string) (*gatewayapiv1.GatewayClass, error) {
	kubeConfig, err := config.GetConfig()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := operatorclient.NewClient(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())
	kclient := kubeClient
	gatewayClass := buildGWC(name, controllerName)
	// checks if the gatewayclass can be created
	if err := kclient.Create(context.TODO(), gatewayClass); err != nil {
		if kerrors.IsAlreadyExists(err) {
			name := types.NamespacedName{Namespace: "", Name: name}
			if err := kclient.Get(context.TODO(), name, gatewayClass); err != nil {
				return nil, fmt.Errorf("gatewayClass %s already exists, but got an error: %w", name.Name, err)
			}
			return gatewayClass, nil
		} else {
			return nil, fmt.Errorf("gatewayClass %s creation failed with error: %w", gatewayClass.Name, err)
		}
	}
	return gatewayClass, nil
}

// buildGWC initializes the GatewayClass and returns its address.
func buildGWC(name, controllerName string) *gatewayapiv1.GatewayClass {
	return &gatewayapiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: gatewayapiv1.GatewayClassSpec{
			ControllerName: gatewayapiv1.GatewayController(controllerName),
		},
	}
}
