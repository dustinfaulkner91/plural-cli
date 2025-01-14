package provider

import (
	"context"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"github.com/imdario/mergo"
	metal "github.com/packethost/packngo"
	"github.com/pluralsh/plural/pkg/config"
	"github.com/pluralsh/plural/pkg/manifest"
	"github.com/pluralsh/plural/pkg/template"
	"github.com/pluralsh/plural/pkg/utils"
	"github.com/pluralsh/plural/pkg/utils/git"
	"github.com/pluralsh/plural/pkg/utils/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"sigs.k8s.io/yaml"
)

type EQUINIXProvider struct {
	Clust  string `survey:"cluster"`
	Proj   string
	bucket string
	Metro  string `survey:"metro"`
	ctx    map[string]interface{}
}

var equinixSurvey = []*survey.Question{
	{
		Name:     "cluster",
		Prompt:   &survey.Input{Message: "Enter the name of your cluster:"},
		Validate: validCluster,
	},
	{
		Name:     "metro",
		Prompt:   &survey.Input{Message: "What metro will you deploy to?", Default: "sv"},
		Validate: survey.Required,
	},
	{
		Name:     "project",
		Prompt:   &survey.Input{Message: "Enter the name of the project you want to use:"},
		Validate: survey.Required,
	},
	{
		Name:     "apiToken",
		Prompt:   &survey.Input{Message: "Enter your personal API Token for Equinix Metal:"},
		Validate: survey.Required,
	},
}

func mkEquinix(conf config.Config) (provider *EQUINIXProvider, err error) {
	var resp struct {
		Cluster  string
		Metro    string
		Project  string
		ApiToken string
	}
	if err := survey.Ask(equinixSurvey, &resp); err != nil {
		return nil, err
	}

	projectID, err := getProjectIDFromName(resp.Project, resp.ApiToken)
	if err != nil {
		return nil, errors.ErrorWrap(err, "Failed to get metal project ID (is your metal cli configured?)")
	}

	provider = &EQUINIXProvider{
		resp.Cluster,
		projectID,
		"",
		resp.Metro,
		map[string]interface{}{
			"ApiToken": resp.ApiToken,
		},
	}

	projectManifest := manifest.ProjectManifest{
		Cluster:  provider.Cluster(),
		Project:  provider.Project(),
		Provider: EQUINIX,
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

func equinixFromManifest(man *manifest.ProjectManifest) (*EQUINIXProvider, error) {
	return &EQUINIXProvider{man.Cluster, man.Project, man.Bucket, man.Region, man.Context}, nil
}

func (equinix *EQUINIXProvider) CreateBackend(prefix string, ctx map[string]interface{}) (string, error) {

	ctx["Region"] = equinix.Region()
	ctx["Bucket"] = equinix.Bucket()
	ctx["Prefix"] = prefix
	ctx["ClusterCreated"] = false
	ctx["__CLUSTER__"] = equinix.Cluster()
	if cluster, ok := ctx["cluster"]; ok {
		ctx["Cluster"] = cluster
		ctx["ClusterCreated"] = true
	} else {
		ctx["Cluster"] = fmt.Sprintf(`"%s"`, equinix.Cluster())
	}

	utils.WriteFile(filepath.Join(equinix.Bucket(), ".gitignore"), []byte("!/**"))
	utils.WriteFile(filepath.Join(equinix.Bucket(), ".gitattributes"), []byte("/** filter=plural-crypt diff=plural-crypt\n.gitattributes !filter !diff"))
	scaffold, err := GetProviderScaffold("EQUINIX")
	if err != nil {
		return "", err
	}
	return template.RenderString(scaffold, ctx)
}

func (equinix *EQUINIXProvider) KubeConfig() error {
	// TODO: deal with current configured KUBECONFIG
	// TODO: deal with KUBECONFIG env var if it is set, as then the output KUBECONFIG file will be used
	if utils.InKubernetes() {
		return nil
	}

	usr, _ := user.Current()

	input, err := ioutil.ReadFile(filepath.Join(usr.HomeDir, ".kube/config"))
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(usr.HomeDir, ".kube/config-plural-backup"), input, 0644)
	if err != nil {
		return err
	}

	repoRoot, err := git.Root()
	if err != nil {
		return err
	}

	kubeConfigFiles := []string{
		filepath.Join(usr.HomeDir, ".kube/config-plural-backup"),
		filepath.Join(repoRoot, "bootstrap/terraform/kube_config_cluster.yaml"),
	}
	kubeconfigs := []*clientcmdapi.Config{}

	for _, filename := range kubeConfigFiles {
		if len(filename) == 0 {
			// no work to do
			continue
		}

		config, err := clientcmd.LoadFromFile(filename)

		if err != nil {
			return err
		}

		kubeconfigs = append(kubeconfigs, config)
	}

	// first merge all of our maps
	mapConfig := clientcmdapi.NewConfig()

	for _, kubeconfig := range kubeconfigs {
		mergo.Merge(mapConfig, kubeconfig, mergo.WithOverride)
	}

	// merge all of the struct values
	nonMapConfig := clientcmdapi.NewConfig()
	for i := range kubeconfigs {
		kubeconfig := kubeconfigs[i]
		mergo.Merge(nonMapConfig, kubeconfig, mergo.WithOverride)
	}

	// since values are overwritten, but maps values are not, we can merge the non-map config on top of the map config and
	// get the values we expect.
	config := clientcmdapi.NewConfig()
	mergo.Merge(config, mapConfig, mergo.WithOverride)
	mergo.Merge(config, nonMapConfig, mergo.WithOverride)

	json, err := runtime.Encode(clientcmdlatest.Codec, config)
	if err != nil {
		return err
	}
	output, err := yaml.JSONToYAML(json)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(usr.HomeDir, ".kube/config"), output, 0644)
}

func (equinix *EQUINIXProvider) Name() string {
	return EQUINIX
}

func (equinix *EQUINIXProvider) Cluster() string {
	return equinix.Clust
}

func (equinix *EQUINIXProvider) Project() string {
	return equinix.Proj
}

func (equinix *EQUINIXProvider) Bucket() string {
	return equinix.bucket
}

func (equinix *EQUINIXProvider) Region() string {
	return equinix.Metro
}

func (equinix *EQUINIXProvider) Context() map[string]interface{} {
	return equinix.ctx
}

func (prov *EQUINIXProvider) Decommision(node *v1.Node) error {

	client := getMetalClient(prov.Context()["ApiToken"].(string))

	deviceID := strings.Replace(node.Spec.ProviderID, "equinixmetal://", "", -1)

	_, err := client.Devices.Delete(deviceID, false)

	if err != nil {
		return errors.ErrorWrap(err, "failed to terminate instance")
	}

	return nil
}

func getMetalClient(apiToken string) *metal.Client {
	transport := logging.NewTransport("Equinix Metal", http.DefaultTransport)
	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = transport
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = time.Second
	retryClient.RetryWaitMax = time.Second
	retryClient.CheckRetry = MetalRetryPolicy
	standardClient := retryClient.StandardClient()

	client := metal.NewClientWithAuth("plural", apiToken, standardClient)

	return client
}

func MetalRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	var redirectsErrorRe = regexp.MustCompile(`stopped after \d+ redirects\z`)

	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if err != nil {
		if v, ok := err.(*url.Error); ok {
			// Don't retry if the error was due to too many redirects.
			if redirectsErrorRe.MatchString(v.Error()) {
				return false, nil
			}

			// Don't retry if the error was due to TLS cert verification failure.
			if _, ok := v.Err.(x509.UnknownAuthorityError); ok {
				return false, nil
			}
		}

		// The error is likely recoverable so retry.
		return true, nil
	}
	return false, nil
}

func getProjectIDFromName(projectName, apiToken string) (string, error) {
	client := getMetalClient(apiToken)

	projects, _, err := client.Projects.List(nil)
	if err != nil {
		return "", errors.ErrorWrap(err, "Error getting project using Metal Client")
	}

	var projectID string

	for _, project := range projects {
		if project.Name == projectName {
			projectID = project.ID
			break
		}
	}
	if projectID == "" {
		return "", errors.ErrorWrap(err, fmt.Sprintf("Project with name %s not found", projectName))
	}

	return projectID, nil
}
