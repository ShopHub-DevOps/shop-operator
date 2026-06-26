package resources

const (
	LabelShopName  = "shophub.io/shop"
	LabelName      = "app.kubernetes.io/name"
	LabelInstance  = "app.kubernetes.io/instance"
	LabelComponent = "app.kubernetes.io/component"
	LabelManagedBy = "app.kubernetes.io/managed-by"

	ManagedByValue = "shop-operator"
	AppNameValue   = "shop"

	ComponentBackend  = "backend"
	ComponentFrontend = "frontend"

	DefaultBackendImage  = "ghcr.io/shophub-devops/shop-backend:local"  //:latest
	DefaultFrontendImage = "ghcr.io/shophub-devops/shop-frontend:local" //:latest

	ContainerPort int32 = 3000
)

// CommonLabels returns labels that identify a per-Shop resource regardless of component.
func CommonLabels(shopName string) map[string]string {
	return map[string]string{
		LabelShopName:  shopName,
		LabelName:      AppNameValue,
		LabelInstance:  shopName,
		LabelManagedBy: ManagedByValue,
	}
}

// ComponentLabels extends CommonLabels with the component (backend / frontend) marker.
func ComponentLabels(shopName, component string) map[string]string {
	l := CommonLabels(shopName)
	l[LabelComponent] = component
	return l
}
