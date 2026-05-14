// Package integration contains envtest-backed sanity tests that verify the
// controller-runtime test environment boots and the client can round-trip
// objects against the in-process API server. Real controller reconciliation
// tests will be added alongside their controllers under internal/controller.
package integration

import (
	"context"
	"log"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	testEnv *envtest.Environment
	cfg     *rest.Config
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		log.Println("KUBEBUILDER_ASSETS is not set; run 'make test' so envtest binaries are downloaded")
		os.Exit(1)
	}

	testEnv = &envtest.Environment{}
	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		log.Fatalf("failed to start envtest: %v", err)
	}

	code := m.Run()

	if err := testEnv.Stop(); err != nil {
		log.Printf("failed to stop envtest cleanly: %v", err)
	}
	os.Exit(code)
}

// TestEnvtestProvidesAPIServer verifies that the envtest control plane started
// and exposed a reachable host. Without this the rest of the integration tests
// could not run, so it doubles as a fast-fail health check.
func TestEnvtestProvidesAPIServer(t *testing.T) {
	if cfg == nil || cfg.Host == "" {
		t.Fatal("envtest cfg has no host - control plane did not start")
	}
}

// TestClientCanRoundTripConfigMap creates a built-in ConfigMap against the
// envtest API server and reads it back to prove that the controller-runtime
// client wiring is functional end-to-end. Once real CRDs land, controller
// tests will follow the same shape but operate on Shop / DiscordChannel /
// Wallet resources instead.
func TestClientCanRoundTripConfigMap(t *testing.T) {
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	ctx := context.Background()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sanity",
			Namespace: "default",
		},
		Data: map[string]string{"answer": "42"},
	}

	if err := c.Create(ctx, cm); err != nil {
		t.Fatalf("create: %v", err)
	}

	got := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: "sanity", Namespace: "default"}
	if err := c.Get(ctx, key, got); err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Data["answer"] != "42" {
		t.Errorf("data mismatch: want answer=42, got %q", got.Data["answer"])
	}
}
