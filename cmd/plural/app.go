package main

import (
	"fmt"
	"context"
	"github.com/pluralsh/plural/pkg/config"
	"github.com/pluralsh/plural/pkg/application"
	"github.com/pluralsh/plural/pkg/utils"
	"sigs.k8s.io/application/api/v1beta1"
	"github.com/urfave/cli"

	tm "github.com/buger/goterm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func handleWatch(c *cli.Context) error {
	repo := c.Args().Get(0)
	conf := config.Read()
	kube, err := utils.Kubernetes()
	if err != nil {
		return err
	}
	ctx := context.Background()
	appClient := kube.Application.Applications(conf.Namespace(repo))
	app, err := appClient.Get(ctx, repo, metav1.GetOptions{})
	if err != nil {
		return err
	}

	tm.Clear()
	application.Print(app)

	watcher, err := application.WatchNamespace(ctx, appClient)
	if err != nil {
		return err
	}

	ch := watcher.ResultChan()
	for {
		event := <-ch
		app, ok := event.Object.(*v1beta1.Application)
		if !ok {
			return fmt.Errorf("Failed to parse watch event")
		}
		tm.MoveCursor(1,1)
		fmt.Println("from watch event!")
		application.Print(app)
		tm.Flush()
	}
}