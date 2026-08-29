package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/model"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type kubernetesAuditTarget struct {
	group       string
	resource    string
	namespace   string
	name        string
	subresource string
}

type kubernetesMutationAudit struct {
	clientSet    *cluster.ClientSet
	user         model.User
	operation    string
	target       kubernetesAuditTarget
	requestJSON  []byte
	previousJSON []byte
}

func newKubernetesMutationAudit(c *gin.Context, clientSet *cluster.ClientSet, target *url.URL, upstreamPath string) *kubernetesMutationAudit {
	operation := map[string]string{
		http.MethodPost:   "create",
		http.MethodPut:    "update",
		http.MethodPatch:  "update",
		http.MethodDelete: "delete",
	}[c.Request.Method]
	if operation == "" {
		return nil
	}
	auditTarget, ok := parseKubernetesAuditTarget(upstreamPath)
	if !ok {
		return nil
	}
	value, ok := c.Get("user")
	if !ok {
		return nil
	}
	user, ok := value.(model.User)
	if !ok {
		return nil
	}
	audit := &kubernetesMutationAudit{
		clientSet: clientSet,
		user:      user,
		operation: operation,
		target:    auditTarget,
	}
	if c.Request.Body != nil {
		audit.requestJSON, _ = io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(audit.requestJSON))
	}
	if auditTarget.name != "" && operation != "create" {
		request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, joinURLPath(target.String(), upstreamPath), nil)
		if err == nil {
			response, requestErr := clientSet.K8sClient.HTTPClient.Do(request)
			if requestErr == nil {
				if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
					audit.previousJSON, _ = io.ReadAll(response.Body)
				}
				_ = response.Body.Close()
			}
		}
	}
	return audit
}

func (a *kubernetesMutationAudit) record(response *http.Response) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	success := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	name, namespace := a.target.name, a.target.namespace
	if name == "" {
		name, namespace = objectIdentity(a.requestJSON, namespace)
	}
	if success {
		responseName, responseNamespace := objectIdentity(body, namespace)
		if responseName != "" {
			name = responseName
		}
		if responseNamespace != "" {
			namespace = responseNamespace
		}
	}
	errorMessage := ""
	if !success {
		var status struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &status) == nil {
			errorMessage = status.Message
		}
		if errorMessage == "" {
			errorMessage = response.Status
		}
	}
	resourceYAML := ""
	if success && a.operation != "delete" {
		resourceYAML = jsonAsYAML(body)
	}
	previousYAML := jsonAsYAML(a.previousJSON)
	resourceType := common.HistoryResourceType(a.target.resource, a.target.group)
	if resourceType == string(common.Secrets) {
		resourceYAML, previousYAML = "", ""
		if errorMessage != "" {
			errorMessage = "Kubernetes Secret operation failed; details omitted"
		}
	}
	history := model.ResourceHistory{
		ClusterID:       a.clientSet.ClusterID,
		ClusterName:     a.clientSet.Name,
		ResourceType:    resourceType,
		ResourceName:    name,
		Namespace:       namespace,
		OperationType:   a.operation,
		OperationSource: "manual",
		ResourceYAML:    resourceYAML,
		PreviousYAML:    previousYAML,
		Success:         success,
		ErrorMessage:    errorMessage,
		OperatorID:      a.user.ID,
	}
	if err := model.DB.Create(&history).Error; err != nil {
		klog.Errorf("Failed to record Kubernetes API mutation: %v", err)
	}
	return nil
}

func objectIdentity(value []byte, defaultNamespace string) (string, string) {
	var object struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if json.Unmarshal(value, &object) != nil {
		return "", defaultNamespace
	}
	if object.Metadata.Namespace == "" {
		object.Metadata.Namespace = defaultNamespace
	}
	return object.Metadata.Name, object.Metadata.Namespace
}

func jsonAsYAML(value []byte) string {
	if len(bytes.TrimSpace(value)) == 0 {
		return ""
	}
	converted, err := yaml.JSONToYAML(value)
	if err != nil {
		return ""
	}
	return string(converted)
}

func parseKubernetesAuditTarget(path string) (kubernetesAuditTarget, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	var target kubernetesAuditTarget
	var index int
	switch {
	case len(segments) >= 3 && segments[0] == "api":
		index = 2
	case len(segments) >= 4 && segments[0] == "apis":
		target.group = segments[1]
		index = 3
	default:
		return kubernetesAuditTarget{}, false
	}
	if len(segments) > index+2 && segments[index] == "namespaces" {
		target.namespace = segments[index+1]
		index += 2
	}
	if len(segments) <= index {
		return kubernetesAuditTarget{}, false
	}
	target.resource = segments[index]
	if len(segments) > index+1 {
		target.name = segments[index+1]
	}
	if len(segments) > index+2 {
		target.subresource = segments[index+2]
	}
	return target, true
}
