package resources_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
	"github.com/ShopHub-DevOps/shop-operator/internal/resources"
)

func sampleShop() *shophubv1alpha1.Shop {
	return &shophubv1alpha1.Shop{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "tenants",
		},
		Spec: shophubv1alpha1.ShopSpec{
			DisplayName:   "Demo Store",
			Host:          "demo.shophub.local",
			Availability:  shophubv1alpha1.AvailabilityStandard,
			DatabaseTier:  shophubv1alpha1.DatabaseStandard,
			WalletAddress: "0xabcdef1234567890abcdef1234567890abcdef12",
			ChainID:       11155111,
		},
	}
}

func TestReplicas(t *testing.T) {
	cases := []struct {
		name string
		tier shophubv1alpha1.AvailabilityTier
		want int32
	}{
		{"standard tier returns 2 replicas", shophubv1alpha1.AvailabilityStandard, 2},
		{"high tier returns 3 replicas", shophubv1alpha1.AvailabilityHigh, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sampleShop()
			s.Spec.Availability = tc.tier
			if got := resources.Replicas(s); got != tc.want {
				t.Fatalf("Replicas(%s) = %d, want %d", tc.tier, got, tc.want)
			}
		})
	}
}

func TestBuildBackendDeployment(t *testing.T) {
	s := sampleShop()
	s.Spec.Availability = shophubv1alpha1.AvailabilityHigh
	d := resources.BuildBackendDeployment(s)

	if d.Name != "demo-be" {
		t.Errorf("Name = %q, want demo-be", d.Name)
	}
	if d.Namespace != "tenants" {
		t.Errorf("Namespace = %q, want tenants", d.Namespace)
	}
	if d.Spec.Replicas == nil || *d.Spec.Replicas != 3 {
		t.Errorf("Replicas = %v, want 3", d.Spec.Replicas)
	}
	if d.Labels[resources.LabelComponent] != resources.ComponentBackend {
		t.Errorf("component label = %q, want %q", d.Labels[resources.LabelComponent], resources.ComponentBackend)
	}
	if d.Labels[resources.LabelShopName] != "demo" {
		t.Errorf("shop label = %q, want demo", d.Labels[resources.LabelShopName])
	}
	if len(d.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(d.Spec.Template.Spec.Containers))
	}
	c := d.Spec.Template.Spec.Containers[0]
	if c.Image != resources.DefaultBackendImage {
		t.Errorf("image = %q, want %q", c.Image, resources.DefaultBackendImage)
	}
	if len(c.EnvFrom) != 3 {
		t.Errorf("expected 3 EnvFrom (configmap + secret + cnpg_secret), got %d", len(c.EnvFrom))
	}
}

func TestBuildBackendDeploymentRespectsImageOverride(t *testing.T) {
	s := sampleShop()
	s.Spec.Images.Backend = "ghcr.io/test/backend:v1.2.3"
	d := resources.BuildBackendDeployment(s)
	c := d.Spec.Template.Spec.Containers[0]
	if c.Image != "ghcr.io/test/backend:v1.2.3" {
		t.Errorf("image override ignored, got %q", c.Image)
	}
}

func TestBuildFrontendDeployment(t *testing.T) {
	s := sampleShop()
	d := resources.BuildFrontendDeployment(s)
	if d.Name != "demo-fe" {
		t.Errorf("Name = %q, want demo-fe", d.Name)
	}
	if d.Labels[resources.LabelComponent] != resources.ComponentFrontend {
		t.Errorf("component = %q, want %q", d.Labels[resources.LabelComponent], resources.ComponentFrontend)
	}
}

func TestServicesSelectMatchingDeployment(t *testing.T) {
	s := sampleShop()

	cases := []struct {
		name    string
		svc     func(*shophubv1alpha1.Shop) interface{ GetName() string }
		want    string
		compKey string
	}{
		{"backend service", func(s *shophubv1alpha1.Shop) interface{ GetName() string } {
			return resources.BuildBackendService(s)
		}, "demo-be", resources.ComponentBackend},
		{"frontend service", func(s *shophubv1alpha1.Shop) interface{ GetName() string } {
			return resources.BuildFrontendService(s)
		}, "demo-fe", resources.ComponentFrontend},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := tc.svc(s)
			if obj.GetName() != tc.want {
				t.Errorf("Name = %q, want %q", obj.GetName(), tc.want)
			}
		})
	}

	beSvc := resources.BuildBackendService(s)
	beDep := resources.BuildBackendDeployment(s)
	if beSvc.Spec.Selector[resources.LabelComponent] != beDep.Labels[resources.LabelComponent] {
		t.Errorf("backend service selector does not match deployment labels")
	}
	if beSvc.Spec.Ports[0].Port != 80 {
		t.Errorf("backend port = %d, want 80", beSvc.Spec.Ports[0].Port)
	}
}

func TestBuildIngressRoutesPathsToCorrectServices(t *testing.T) {
	s := sampleShop()
	ing := resources.BuildIngress(s)

	if ing.Name != "demo" {
		t.Errorf("Name = %q, want demo", ing.Name)
	}
	if len(ing.Spec.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(ing.Spec.Rules))
	}
	rule := ing.Spec.Rules[0]
	if rule.Host != "demo.shophub.local" {
		t.Errorf("host = %q, want demo.shophub.local", rule.Host)
	}
	if len(rule.HTTP.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(rule.HTTP.Paths))
	}
	apiPath := rule.HTTP.Paths[0]
	if apiPath.Path != "/api(/|$)(.*)" || apiPath.Backend.Service.Name != "demo-be" {
		t.Errorf("/api should route to demo-be, got path=%q name=%q", apiPath.Path, apiPath.Backend.Service.Name)
	}
	rootPath := rule.HTTP.Paths[1]
	if rootPath.Path != "/()(.*)" || rootPath.Backend.Service.Name != "demo-fe" {
		t.Errorf("/ should route to demo-fe, got path=%q name=%q", rootPath.Path, rootPath.Backend.Service.Name)
	}
}

func TestBuildConfigMapExposesShopFields(t *testing.T) {
	s := sampleShop()
	cm := resources.BuildConfigMap(s)
	if cm.Name != "demo-config" {
		t.Errorf("Name = %q, want demo-config", cm.Name)
	}
	want := map[string]string{
		"SHOP_DISPLAY_NAME": "Demo Store",
		"SHOP_HOST":         "demo.shophub.local",
		"SHOP_DB_TIER":      "standard",
		"WALLET_ADDRESS":    "0xabcdef1234567890abcdef1234567890abcdef12",
		"CHAIN_ID":          "11155111",
	}
	for k, v := range want {
		if got := cm.Data[k]; got != v {
			t.Errorf("Data[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestBuildSecretHasPlaceholderKeys(t *testing.T) {
	t.Setenv("SHARED_JWT_SECRET", "test-secret")
	s := sampleShop()
	sec := resources.BuildSecret(s, "test-secret")
	if sec.Name != "demo-secret" {
		t.Errorf("Name = %q, want demo-secret", sec.Name)
	}
	for _, k := range []string{"JWT_SECRET", "DB_PASSWORD"} {
		if _, ok := sec.StringData[k]; !ok {
			t.Errorf("Secret missing placeholder key %q", k)
		}
	}
}
