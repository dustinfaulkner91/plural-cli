package utils

import (
	"os"
	"path/filepath"
	"context"
	"bytes"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
	"github.com/pluralsh/plural/pkg/application"
	pluralv1alpha1 "github.com/pluralsh/plural-operator/generated/platform/clientset/versioned"
)

const tokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"

func InKubernetes() bool {
	if os.Getenv("IGNORE_IN_CLUSTER") == "true" {
		return false
	}

	return Exists(tokenFile)
}

type Kube struct {
	Kube  *kubernetes.Clientset
	Plural *pluralv1alpha1.Clientset
	Application *application.ApplicationV1Beta1Client
	Dynamic dynamic.Interface
}

func InClusterKubernetes() (*Kube, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return buildKubeFromConfig(config)
}

func Kubernetes() (*Kube, error) {
	if InKubernetes() {
		return InClusterKubernetes()
	}

	homedir, _ := os.UserHomeDir()
	conf := filepath.Join(homedir, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", conf)
	if err != nil {
		return nil, err
	}

	return buildKubeFromConfig(config)
}

func ParseYaml(content []byte) ([]*unstructured.Unstructured, error) {
	d := kubeyaml.NewYAMLOrJSONDecoder(bytes.NewReader(content), 4096)
	var objs []*unstructured.Unstructured
	for {
		ext := runtime.RawExtension{}
		if err := d.Decode(&ext); err != nil {
			if err == io.EOF {
				break
			}
			return objs, fmt.Errorf("failed to unmarshal manifest: %v", err)
		}
	
		ext.Raw = bytes.TrimSpace(ext.Raw)
		if len(ext.Raw) == 0 || bytes.Equal(ext.Raw, []byte("null")) {
			continue
		}
	
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(ext.Raw, u); err != nil {
			return objs, fmt.Errorf("failed to unmarshal manifest: %v", err)
		}
		objs = append(objs, u)
	}

	return objs, nil
}

func buildKubeFromConfig(config *rest.Config) (*Kube, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	plural, err := pluralv1alpha1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	app, err := application.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Kube{Kube: clientset, Plural: plural, Application: app, Dynamic: dyn}, nil
}

func (k *Kube) Secret(namespace string, name string) (*v1.Secret, error) {
	return k.Kube.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func (k *Kube) Node(name string) (*v1.Node, error) {
	return k.Kube.CoreV1().Nodes().Get(context.Background(), name, metav1.GetOptions{})
}

func (k *Kube) Nodes() (*v1.NodeList, error) {
	return k.Kube.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
}

func (k *Kube) FinalizeNamespace(namespace string) error {
	ctx := context.Background()
	client := k.Kube.CoreV1().Namespaces()
	ns, err := client.Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}

	ns.Spec.Finalizers = []v1.FinalizerName{}
	_, err = client.Finalize(ctx, ns, metav1.UpdateOptions{})
	return err
}
