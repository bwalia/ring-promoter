package controller

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	saTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	saCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// ---------------------------------------------------------------------------
// Minimal typed views of the API objects we touch.
// ---------------------------------------------------------------------------

type objectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type greeting struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Message string `json:"message"`
	} `json:"spec"`
}

type greetingList struct {
	Items []greeting `json:"items"`
}

type configMap struct {
	APIVersion string            `json:"apiVersion,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Metadata   objectMeta        `json:"metadata"`
	Data       map[string]string `json:"data,omitempty"`
}

// ---------------------------------------------------------------------------
// Standard-library Kubernetes REST client (in-cluster).
// ---------------------------------------------------------------------------

type k8sClient struct {
	host  string // https://host:port
	token string
	http  *http.Client
}

func newInClusterClient() (*k8sClient, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := envOr("KUBERNETES_SERVICE_PORT", "443")
	if host == "" {
		return nil, errors.New("KUBERNETES_SERVICE_HOST not set")
	}
	token, err := os.ReadFile(saTokenPath)
	if err != nil {
		return nil, fmt.Errorf("read service-account token: %w", err)
	}
	caPEM, err := os.ReadFile(saCAPath)
	if err != nil {
		return nil, fmt.Errorf("read cluster CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("cluster CA bundle is not valid PEM")
	}
	return &k8sClient{
		host:  fmt.Sprintf("https://%s:%s", host, port),
		token: string(bytes.TrimSpace(token)),
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
}

// listGreetings lists Greetings cluster-wide (ns == "") or in one namespace.
func (c *k8sClient) listGreetings(ctx context.Context, ns string) ([]greeting, error) {
	var path string
	if ns == "" {
		path = fmt.Sprintf("/apis/%s/%s/%s", crdGroup, crdVersion, crdPlural)
	} else {
		path = fmt.Sprintf("/apis/%s/%s/namespaces/%s/%s", crdGroup, crdVersion, ns, crdPlural)
	}
	body, status, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list greetings: unexpected status %d: %s", status, string(body))
	}
	var list greetingList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("decode greeting list: %w", err)
	}
	return list.Items, nil
}

func (c *k8sClient) getConfigMap(ctx context.Context, ns, name string) (configMap, bool, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", ns, name)
	body, status, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return configMap{}, false, err
	}
	switch status {
	case http.StatusOK:
		var cm configMap
		if err := json.Unmarshal(body, &cm); err != nil {
			return configMap{}, false, fmt.Errorf("decode configmap: %w", err)
		}
		return cm, true, nil
	case http.StatusNotFound:
		return configMap{}, false, nil
	default:
		return configMap{}, false, fmt.Errorf("get configmap: unexpected status %d: %s", status, string(body))
	}
}

func (c *k8sClient) createConfigMap(ctx context.Context, ns string, cm configMap) error {
	cm.APIVersion = "v1"
	cm.Kind = "ConfigMap"
	payload, err := json.Marshal(cm)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/configmaps", ns)
	body, status, err := c.do(ctx, http.MethodPost, path, payload)
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("create configmap: unexpected status %d: %s", status, string(body))
	}
	return nil
}

func (c *k8sClient) updateConfigMap(ctx context.Context, ns string, cm configMap) error {
	cm.APIVersion = "v1"
	cm.Kind = "ConfigMap"
	payload, err := json.Marshal(cm)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", ns, cm.Metadata.Name)
	body, status, err := c.do(ctx, http.MethodPut, path, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("update configmap: unexpected status %d: %s", status, string(body))
	}
	return nil
}

// do performs one authenticated request and returns body + status.
func (c *k8sClient) do(ctx context.Context, method, path string, payload []byte) ([]byte, int, error) {
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Small helpers.
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("invalid %s=%q, using default %s", key, v, def)
	}
	return def
}
