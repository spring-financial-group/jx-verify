package cmd

import (
	"os"

	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-verify/pkg/cmd/tls"
	"github.com/jenkins-x/jx-verify/pkg/cmd/version"
	"github.com/jenkins-x/jx-verify/pkg/rootcmd"
	"github.com/jenkins-x/jx/v2/pkg/cmd/clients"
	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
	"github.com/jenkins-x/jx/v2/pkg/cmd/update"
	"github.com/jenkins-x/jx/v2/pkg/cmd/step/verify"
	"github.com/spf13/cobra"
)

// Main creates the new command
func Main() *cobra.Command {
	cmd := &cobra.Command{
		Use:   rootcmd.TopLevelCommand,
		Short: "commands for verifying Jenkins X environments",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				log.Logger().Errorf(err.Error())
			}
		},
	}
	f := clients.NewFactory()
	commonOpts := opts.NewCommonOptionsWithTerm(f, os.Stdin, os.Stdout, os.Stderr)
	commonOpts.AddBaseFlags(cmd)

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

	cmd.AddCommand(verify.NewCmdStepVerifyBehavior(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyDependencies(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyDNS(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyEnvironments(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyGit(commonOpts))
	cmd.AddCommand(verifyIngress)
	cmd.AddCommand(verify.NewCmdStepVerifyInstall(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyPackages(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyPod(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyPreInstall(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyRequirements(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyURL(commonOpts))
	cmd.AddCommand(verify.NewCmdStepVerifyValues(commonOpts))
	cmd.AddCommand(update.NewCmdUpdateWebhooks(commonOpts))
	cmd.AddCommand(cobras.SplitCommand(tls.NewCmdVerifyTLS()))
	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))

	return cmd
}
