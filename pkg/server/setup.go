package server

import (
	"github.com/gin-gonic/gin"
	"github.com/pluralsh/plural/pkg/crypto"
	"github.com/pluralsh/plural/pkg/config"
	"github.com/pluralsh/plural/pkg/utils"
	"github.com/pluralsh/plural/pkg/manifest"
)

func toConfig(setup *SetupRequest) *config.Config {
	return &config.Config{
		Email: setup.User.Email,
		Token: setup.User.AccessToken,
	}
}

func toManifest(setup *SetupRequest) *manifest.ProjectManifest {
	wk := setup.Workspace
	return &manifest.ProjectManifest{
		Cluster:      wk.Cluster,
		Bucket:       wk.Bucket,
		Project:      wk.Project,
		Provider:     toProvider(setup.Provider),
		Region:       wk.Region,
		BucketPrefix: wk.BucketPrefix,
		Owner:        &manifest.Owner{Email: setup.User.Email},
		Network:      &manifest.NetworkConfig{
			PluralDns: true,
			Subdomain: wk.Subdomain,
		},
	}
}

func toContext(setup *SetupRequest) *manifest.Context {
	ctx := manifest.NewContext()
	ctx.Configuration = map[string]map[string]interface{}{
		"console": map[string]interface{}{
			"private_key": setup.SshPrivateKey,
			"public_key": setup.SshPublicKey,
			"passphrase": "",
			"repo_url": setup.GitUrl,
		},
	}
	return ctx
}

func setupCli(c *gin.Context) error {
	var setup SetupRequest
	if err := c.ShouldBindJSON(&setup); err != nil {
		return err
	}

	if err := crypto.Setup(setup.AesKey); err != nil {
		return err
	}

	conf := toConfig(&setup)
	if err := conf.Flush(); err != nil {
		return err
	}

	if err := setupGit(&setup); err != nil {
		return err
	}

	man := toManifest(&setup)
	path := manifest.ProjectManifestPath()
	if err := man.Write(path); err != nil {
		return err
	}

	ctx := toContext(&setup)
	path = manifest.ContextPath()
	if !utils.Exists(path) {
		return ctx.Write(path)
	}

	return nil
}