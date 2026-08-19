package clusteragent

import (
	"fmt"
	"strings"
)

// GenerateManifest builds the Kubernetes manifest (Secret and Deployment)
// used to deploy the transport-only Cluster Agent inside a
// target cluster. The image and server URL are injected so the manifest is
// always consistent with the platform configuration.
func GenerateManifest(serverURL, token, publicKey, image string) string {
	serverURL = strings.TrimSpace(serverURL)
	token = strings.TrimSpace(token)
	publicKey = strings.TrimSpace(publicKey)
	image = strings.TrimSpace(image)
	// JSON-encode the string values so the YAML is always valid regardless
	// of special characters.
	tokenJSON := fmt.Sprintf("%q", token)
	publicKeyJSON := fmt.Sprintf("%q", publicKey)
	serverJSON := fmt.Sprintf("%q", serverURL)
	imageJSON := fmt.Sprintf("%q", image)
	tokenHashJSON := fmt.Sprintf("%q", tokenHash(token))
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: kite-cluster-agent-token
  namespace: kube-system
type: Opaque
stringData:
  token: %s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kite-cluster-agent
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: kite-cluster-agent
  template:
    metadata:
      annotations:
        kite.kubernetes.io/cluster-agent-token-hash: %s
      labels:
        app.kubernetes.io/name: kite-cluster-agent
    spec:
      containers:
        - name: cluster-agent
          image: %s
          command:
            - /app/kite
          args:
            - cluster-agent
            - --server=$(KITE_SERVER)
            - --token=$(CLUSTER_AGENT_TOKEN)
            - --public-key=$(CLUSTER_AGENT_PUBLIC_KEY)
            - --api-server=https://kubernetes.default.svc
            - --ca-file=/var/run/kite/ca/ca.crt
          env:
            - name: KITE_SERVER
              value: %s
            - name: CLUSTER_AGENT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: kite-cluster-agent-token
                  key: token
            - name: CLUSTER_AGENT_PUBLIC_KEY
              value: %s
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
            readOnlyRootFilesystem: true
            runAsNonRoot: true
          volumeMounts:
            - name: kube-root-ca
              mountPath: /var/run/kite/ca
              readOnly: true
      automountServiceAccountToken: false
      securityContext:
        seccompProfile:
          type: RuntimeDefault
      volumes:
        - name: kube-root-ca
          configMap:
            name: kube-root-ca.crt
            items:
              - key: ca.crt
                path: ca.crt
`, tokenJSON, tokenHashJSON, imageJSON, serverJSON, publicKeyJSON)
}
