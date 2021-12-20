package scaffold

import (
	"path/filepath"
	"io/ioutil"
	"github.com/pluralsh/plural/pkg/utils"

	"gopkg.in/yaml.v2"
)

type Applications struct {
	Root string
}

func BuildApplications(root string) *Applications {
	return &Applications{Root: root}
}

func NewApplications() (*Applications, error) {
	root, err := utils.RepoRoot()
	if err != nil {
		return nil, err
	}

	return BuildApplications(root), nil
}

func (apps *Applications) HelmValues(app string) (map[string]interface{}, error) {
	var res map[string]interface{}
	path := filepath.Join(apps.Root, app, "helm", app, "values.yaml")
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return res, err
	}

	err = yaml.Unmarshal(content, &res)
	return res, err
} 