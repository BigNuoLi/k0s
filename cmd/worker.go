package cmd

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mirantis/mke/pkg/component"
	"github.com/Mirantis/mke/pkg/component/worker"
	"github.com/Mirantis/mke/pkg/constant"
	"github.com/Mirantis/mke/pkg/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/tools/clientcmd"
)

// WorkerCommand ...
func WorkerCommand() *cli.Command {
	return &cli.Command{
		Name:   "worker",
		Usage:  "Run worker",
		Action: startWorker,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "server",
			},
		},
	}
}

func startWorker(ctx *cli.Context) error {
	worker.KernelSetup()

	// logrus.Debugf("using server address %s", serverAddress)
	token := ctx.Args().First()
	if token == "" && !util.FileExists("/var/lib/mke/kubelet.conf") {
		return fmt.Errorf("normal kubelet kubeconfig does not exist and no join-token given. dunno how to make kubelet auth to api")
	}

	// Dump join token into kubelet-bootstrap kubeconfig
	if token != "" {
		kubeconfig, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return errors.Wrap(err, "join-token does not seem to be proper token created by 'mke token create'")
		}

		// Load the kubeconfig to validate it
		_, err = clientcmd.Load(kubeconfig)
		if err != nil {
			return errors.Wrap(err, "failed to parse kubelet bootstrap auth from token")
		}
		// if !util.FileExists("/var/lib/mke/pki/ca.crt") {
		// 	os.MkdirAll(constant.CertRoot, 0755) // ignore errors in case directory exists
		// 	err = ioutil.WriteFile("/var/lib/mke/pki/ca.crt", kc.Clusters["mke"].CertificateAuthorityData, 0600)
		// 	if err != nil {
		// 		return errors.Wrap(err, "failed to write ca client cert")
		// 	}
		// }

		//err = clientcmd.WriteToFile(*kc, constant.KubeletBootstrapConfigPath)
		err = ioutil.WriteFile(constant.KubeletBootstrapConfigPath, kubeconfig, 0600)
		if err != nil {
			return errors.Wrap(err, "failed writing kubelet bootstrap auth config")
		}
	}

	components := make(map[string]component.Component)

	components["containerd"] = &worker.ContainerD{}
	components["kubelet"] = &worker.Kubelet{}

	// extract needed components
	for _, comp := range components {
		if err := comp.Init(); err != nil {
			return err
		}
	}

	// Set up signal handling. Use bufferend channel so we dont miss
	// signals during startup
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	components["containerd"].Run()
	components["kubelet"].Run()

	// Wait for mke process termination
	<-c
	logrus.Info("Shutting down mke worker")

	components["kubelet"].Stop()
	components["containerd"].Stop()

	return nil

}
