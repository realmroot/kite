package resources

import (
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type GenericResourceHandler[T client.Object, V client.ObjectList] struct {
	name            string
	isClusterScoped bool
	objectType      reflect.Type
	listType        reflect.Type
	enableSearch    bool
}

func NewGenericResourceHandler[T client.Object, V client.ObjectList](
	resourceType common.ResourceType,
) *GenericResourceHandler[T, V] {
	var obj T
	var list V
	meta := common.MustLookupResource(string(resourceType))

	return &GenericResourceHandler[T, V]{
		name:            string(resourceType),
		isClusterScoped: meta.ClusterScoped,
		enableSearch:    meta.Searchable,
		objectType:      reflect.TypeOf(obj).Elem(),
		listType:        reflect.TypeOf(list).Elem(),
	}
}

func (h *GenericResourceHandler[T, V]) toYAML(obj T) string {
	if reflect.ValueOf(obj).IsNil() {
		return ""
	}
	copy := obj.DeepCopyObject().(T)
	copy.SetManagedFields(nil)
	yamlBytes, err := yaml.Marshal(copy)
	if err != nil {
		return ""
	}
	return string(yamlBytes)
}

func (h *GenericResourceHandler[T, V]) recordHistory(c *gin.Context, operation string, previous, current T, success bool, errorMessage string) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	resourceName, namespace := "", ""
	if !reflect.ValueOf(current).IsNil() {
		resourceName, namespace = current.GetName(), current.GetNamespace()
	}
	if resourceName == "" && !reflect.ValueOf(previous).IsNil() {
		resourceName, namespace = previous.GetName(), previous.GetNamespace()
	}
	resourceYAML, previousYAML := h.toYAML(current), h.toYAML(previous)
	if h.name == string(common.Secrets) {
		resourceYAML, previousYAML = "", ""
		if errorMessage != "" {
			errorMessage = "Kubernetes Secret operation failed; details omitted"
		}
	}
	if operation == "delete" {
		resourceYAML = ""
	}
	history := model.ResourceHistory{
		ClusterID: cs.ClusterID, ClusterName: cs.Name,
		ResourceType: h.name, ResourceName: resourceName, Namespace: namespace,
		OperationType: operation, OperationSource: "manual",
		ResourceYAML: resourceYAML, PreviousYAML: previousYAML,
		Success: success, ErrorMessage: errorMessage, OperatorID: user.ID,
	}
	if err := model.DB.Create(&history).Error; err != nil {
		klog.Errorf("Failed to create resource history: %v", err)
	}
}

func (h *GenericResourceHandler[T, V]) getGroupKind() schema.GroupKind {
	objValue := reflect.New(h.objectType).Interface().(T)
	gvks, _, err := kube.GetScheme().ObjectKinds(objValue)
	if err != nil || len(gvks) == 0 {
		return schema.GroupKind{}
	}
	return gvks[0].GroupKind()
}

func (h *GenericResourceHandler[T, V]) IsClusterScoped() bool {
	return h.isClusterScoped
}

func (h *GenericResourceHandler[T, V]) Name() string {
	return h.name
}

func (h *GenericResourceHandler[T, V]) Searchable() bool {
	return h.enableSearch
}

func (h *GenericResourceHandler[T, V]) GetResource(c *gin.Context, namespace, name string) (interface{}, error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	object := reflect.New(h.objectType).Interface().(T)
	namespacedName := types.NamespacedName{Name: name}
	if !h.isClusterScoped {
		if namespace != "" && namespace != common.AllNamespaces {
			namespacedName.Namespace = namespace
		}
	}
	if err := cs.K8sClient.Get(c.Request.Context(), namespacedName, object); err != nil {
		return nil, err
	}
	return object, nil
}
