package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	shophubv1alpha1 "github.com/ShopHub-DevOps/shop-operator/api/v1alpha1"
)

// BuildBackendDeployment composes the backend Deployment for a Shop. The caller
// is responsible for setting controller ownership before applying.
func BuildBackendDeployment(s *shophubv1alpha1.Shop) *appsv1.Deployment {
	return buildDeployment(s, BackendName(s), ComponentBackend, BackendImage(s))
}

// BuildFrontendDeployment composes the frontend Deployment for a Shop.
func BuildFrontendDeployment(s *shophubv1alpha1.Shop) *appsv1.Deployment {
	return buildDeployment(s, FrontendName(s), ComponentFrontend, FrontendImage(s))
}

func buildDeployment(s *shophubv1alpha1.Shop, name, component, image string) *appsv1.Deployment {
	labels := ComponentLabels(s.Name, component)
	replicas := Replicas(s)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  component,
						Image: image,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: ContainerPort,
						}},
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(s)}}},
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: SecretName(s)}}},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromString("http"),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					}},
				},
			},
		},
	}
}
