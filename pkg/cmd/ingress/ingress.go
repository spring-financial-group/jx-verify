package ingress

import (
	"net"

	"github.com/jenkins-x/jx-api/pkg/config"
	"github.com/jenkins-x/jx-helpers/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx/v2/pkg/cmd/helper"
	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
	"github.com/jenkins-x/jx/v2/pkg/cmd/step/verify"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCmdVerifyIngress creates a command object for the command
func NewCmdVerifyIngress(commonOpts *opts.CommonOptions) *cobra.Command {
	verifyIngress := verify.NewCmdStepVerifyIngress(commonOpts)
	flag := verifyIngress.Flag("ingress-namespace")
	if flag != nil {
		flag.Value.Set("nginx")
		flag.DefValue = "nginx"
	}
	flag = verifyIngress.Flag("ingress-service")
	if flag != nil {
		flag.Value.Set("nginx-ingress-controller")
		flag.DefValue = "nginx-ingress-controller"
	}

	oldRun := verifyIngress.Run

	verifyIngress.Run = func(cmd *cobra.Command, args []string) {
		oldRun(cmd, args)

		err := verifyDockerRegistry(verifyIngress, commonOpts)
		if err != nil {
			log.Logger().Errorf("failed %s", err.Error())
		}
		helper.CheckErr(err)
	}
	return verifyIngress
}

func verifyDockerRegistry(verifyIngress *cobra.Command, o *opts.CommonOptions) error {
	dir := "."
	flag := verifyIngress.Flag("dir")
	if flag != nil {
		dir = flag.Value.String()
	}
	if dir == "" {
		dir = "."
	}

	log.Logger().Infof("now verifying docker registry ingress setup in dir %s", dir)

	requirements, requirementsFileName, err := config.LoadRequirementsConfig(dir, false)
	if err != nil {
		return errors.Wrapf(err, "failed to load Jenkins X requirements")
	}

	if requirements.Cluster.Registry != "" {
		// if the registry is an IP address then lets still default as the service could have been recreated
		addr := net.ParseIP(requirements.Cluster.Registry)
		if addr == nil {
			return nil
		}
	}
	switch requirements.Cluster.Provider {
	case "kubernetes", "kind", "docker", "minikube", "minishift":
		if requirements.Cluster.Namespace == "" {
			requirements.Cluster.Namespace = "jx"
		}

		client, err := o.KubeClient()
		if err != nil {
			return errors.Wrapf(err, "failed to create kubernetes client")
		}

		svc, err := client.CoreV1().Services(requirements.Cluster.Namespace).Get("docker-registry", metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to list services in namespace %s so we can default the registry host", requirements.Cluster.Namespace)
		}

		if svc != nil && svc.Spec.ClusterIP != "" {
			requirements.Cluster.Registry = svc.Spec.ClusterIP
		} else {
			log.Logger().Warnf("could not find the clusterIP for the service docker-registry in the namespace %s so that we could default the container registry host", requirements.Cluster.Namespace)
			return nil
		}

		err = requirements.SaveConfig(requirementsFileName)
		if err != nil {
			return errors.Wrapf(err, "failed to save changes to file: %s", requirementsFileName)
		}
		log.Logger().Infof("defaulting the docker registry and modified %s\n", termcolor.ColorInfo(requirementsFileName))
		return nil

	default:
		return nil
	}
}
