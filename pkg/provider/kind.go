package provider

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pluralsh/plural/pkg/config"
	"github.com/pluralsh/plural/pkg/manifest"
	"github.com/pluralsh/plural/pkg/template"
	"github.com/pluralsh/plural/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

type KINDProvider struct {
	Clust  string `survey:"cluster"`
	Proj   string
	bucket string
	Reg    string
	ctx    map[string]interface{}
}

var kindSurvey = []*survey.Question{
	{
		Name:     "cluster",
		Prompt:   &survey.Input{Message: "Enter the name of your cluster:"},
		Validate: validCluster,
	},
}

func mkKind(conf config.Config) (provider *KINDProvider, err error) {
	var resp struct {
		Cluster string
	}
	if err := survey.Ask(kindSurvey, &resp); err != nil {
		return nil, err
	}

	provider = &KINDProvider{
		resp.Cluster,
		"",
		"",
		"us-east-1",
		map[string]interface{}{},
	}

	projectManifest := manifest.ProjectManifest{
		Cluster:  provider.Cluster(),
		Project:  provider.Project(),
		Provider: KIND,
		Region:   provider.Region(),
		Context:  provider.Context(),
		Owner:    &manifest.Owner{Email: conf.Email, Endpoint: conf.Endpoint},
	}

	if err := projectManifest.Configure(); err != nil {
		return nil, err
	}

	provider.bucket = projectManifest.Bucket
	return provider, nil
}

func kindFromManifest(man *manifest.ProjectManifest) (*KINDProvider, error) {
	return &KINDProvider{man.Cluster, man.Project, man.Bucket, man.Region, man.Context}, nil
}

func (kind *KINDProvider) CreateBackend(prefix string, ctx map[string]interface{}) (string, error) {

	ctx["Region"] = kind.Region()
	ctx["Bucket"] = kind.Bucket()
	ctx["Prefix"] = prefix
	ctx["ClusterCreated"] = false
	ctx["__CLUSTER__"] = kind.Cluster()
	if cluster, ok := ctx["cluster"]; ok {
		ctx["Cluster"] = cluster
		ctx["ClusterCreated"] = true
	} else {
		ctx["Cluster"] = fmt.Sprintf(`"%s"`, kind.Cluster())
	}

	utils.WriteFile(filepath.Join(kind.Bucket(), ".gitignore"), []byte("!/**"))
	utils.WriteFile(filepath.Join(kind.Bucket(), ".gitattributes"), []byte("/** filter=plural-crypt diff=plural-crypt\n.gitattributes !filter !diff"))
	scaffold, err := GetProviderScaffold("KIND")
	if err != nil {
		return "", err
	}
	return template.RenderString(scaffold, ctx)
}

func (kind *KINDProvider) KubeConfig() error {
	if utils.InKubernetes() {
		return nil
	}
	cmd := exec.Command(
		"kind", "export", "kubeconfig", "--name", kind.Cluster())
	return utils.Execute(cmd)
}

func (kind *KINDProvider) Name() string {
	return KIND
}

func (kind *KINDProvider) Cluster() string {
	return kind.Clust
}

func (kind *KINDProvider) Project() string {
	return kind.Proj
}

func (kind *KINDProvider) Bucket() string {
	return kind.bucket
}

func (kind *KINDProvider) Region() string {
	return kind.Reg
}

func (kind *KINDProvider) Context() map[string]interface{} {
	return kind.ctx
}

func (prov *KINDProvider) Decommision(node *v1.Node) error {
	return nil
}
