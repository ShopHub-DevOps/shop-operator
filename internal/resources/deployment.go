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
	var extraEnv []corev1.EnvVar

	// If standard tier (CNPG), map 'uri' to 'DATABASE_URL' that NestJS expects.
	switch s.Spec.DatabaseTier {
	case shophubv1alpha1.DatabaseStandard:
		extraEnv = append(extraEnv, corev1.EnvVar{
			Name: "DATABASE_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: CNPGSecretName(s),
					},
					Key: "uri", // CNPG generates string in field 'uri'
				},
			},
		})
	case shophubv1alpha1.DatabaseLight:
		// REDB secret provides "password" and "port"
		extraEnv = append(extraEnv, corev1.EnvVar{
			Name:  "REDIS_HOST",
			Value: REDBDatabaseName(s), // Kubernetes DNS routes this service name
		}, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: REDBSecretName(s),
					},
					Key: "password",
				},
			},
		}, corev1.EnvVar{
			Name: "REDIS_PORT",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: REDBSecretName(s),
					},
					Key: "port",
				},
			},
		})
	}

	// To backend send "/health" - health check.
	return buildDeployment(s, BackendName(s), ComponentBackend, BackendImage(s), "/health", extraEnv)
}

// BuildFrontendDeployment composes the frontend Deployment for a Shop.
func BuildFrontendDeployment(s *shophubv1alpha1.Shop) *appsv1.Deployment {
	// To frontend send "/" as health check.
	return buildDeployment(s, FrontendName(s), ComponentFrontend, FrontendImage(s), "/", nil)
}

func buildDeployment(s *shophubv1alpha1.Shop, name, component, image, healthPath string, extraEnv []corev1.EnvVar) *appsv1.Deployment {
	labels := ComponentLabels(s.Name, component)
	replicas := Replicas(s)

	// Build EnvFrom sources
	envFrom := []corev1.EnvFromSource{
		{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(s)}}},
		{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: SecretName(s)}}},
	}

	// Add CNPG Secret if databaseTier is standard
	if s.Spec.DatabaseTier == shophubv1alpha1.DatabaseStandard {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: CNPGSecretName(s),
				},
			},
		})
	}

	var initContainers []corev1.Container
	if component == ComponentBackend && s.Spec.DatabaseTier == shophubv1alpha1.DatabaseStandard {
		initContainers = []corev1.Container{{
			Name:  "run-migrations",
			Image: image,
			//
			Command: []string{"npx", "typeorm", "migration:run", "-d", "dist/data-source.js"},
			EnvFrom: envFrom,
			Env:     extraEnv,
		}}
	}

	// Add the host to the pod template labels so Prometheus can scrape it
	podLabels := make(map[string]string)
	for k, v := range labels {
		podLabels[k] = v
	}
	if s.Spec.Host != "" {
		podLabels["shophub.io/host"] = s.Spec.Host
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
					Containers: []corev1.Container{{
						Name:  component,
						Image: image,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: ContainerPort,
						}},
						EnvFrom: envFrom,
						Env:     extraEnv, // Eksplicitly mapped variables (DATABASE_URL for backend)
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: healthPath, // Dinamic /health for BE, / for FE
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
