package cmd

import (
	"context"
	"fmt"

	"github.com/aquasecurity/starboard/pkg/plugin"
	"github.com/aquasecurity/starboard/pkg/starboard"
	"github.com/aquasecurity/starboard/pkg/vulnerabilityreport"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	vulnerabilitiesCmdShort = "Run static vulnerability scanner for each container image of a given workload"
	vulnerabilitiesCmdLong  = `Scan a given workload for vulnerabilities using Trivy scanner

TYPE is a Kubernetes workload. Shortcuts and API groups will be resolved, e.g. 'po' or 'deployments.apps'.
NAME is the name of a particular Kubernetes workload.
`
)

func NewScanVulnerabilityReportsCmd(buildInfo starboard.BuildInfo, cf *genericclioptions.ConfigFlags) *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"vulns", "vuln"},
		Use:     "vulnerabilityreports (NAME | TYPE/NAME)",
		Short:   vulnerabilitiesCmdShort,
		Long:    vulnerabilitiesCmdShort,
		Example: fmt.Sprintf(`  # Scan a pod with the specified name
  %[1]s scan vulnerabilities nginx

  # Scan a pod with the specified name in the specified namespace
  %[1]s scan vulnerabilityreports po/nginx -n staging

  # Scan a replicaset with the specified name
  %[1]s scan vulnerabilityreports replicaset/nginx

  # Scan a replicationcontroller with the given name
  %[1]s scan vulnerabilityreports rc/nginx

  # Scan a deployment with the specified name
  %[1]s scan vulnerabilityreports deployments.apps/nginx

  # Scan a daemonset with the specified name
  %[1]s scan vulnerabilityreports daemonsets/nginx

  # Scan a statefulset with the specified name
  %[1]s vulnerabilityreports sts/redis

  # Scan a job with the specified name
  %[1]s scan vulnerabilityreports job/my-job

  # Scan a cronjob with the specified name and the specified scan job timeout
  %[1]s scan vulnerabilityreports cj/my-cronjob --scan-job-timeout 2m`, buildInfo.Executable),
		RunE: ScanVulnerabilityReports(buildInfo, cf),
	}

	registerScannerOpts(cmd)

	return cmd
}

func ScanVulnerabilityReports(buildInfo starboard.BuildInfo, cf *genericclioptions.ConfigFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		ns, _, err := cf.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}
		mapper, err := cf.ToRESTMapper()
		if err != nil {
			return err
		}
		workload, _, err := WorkloadFromArgs(mapper, ns, args)
		if err != nil {
			return err
		}
		kubeConfig, err := cf.ToRESTConfig()
		if err != nil {
			return err
		}
		kubeClientset, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}
		scheme := starboard.NewScheme()
		kubeClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
		if err != nil {
			return err
		}
		starboardConfig, err := starboard.NewConfigManager(kubeClientset, starboard.NamespaceName).Read(ctx)
		if err != nil {
			return err
		}
		opts, err := getScannerOpts(cmd)
		if err != nil {
			return err
		}
		plugin, err := plugin.NewResolver().
			WithBuildInfo(buildInfo).
			WithNamespace(starboard.NamespaceName).
			WithServiceAccountName(starboard.ServiceAccountName).
			WithConfig(starboardConfig).
			WithClient(kubeClient).
			GetVulnerabilityPlugin()
		if err != nil {
			return err
		}
		scanner := vulnerabilityreport.NewScanner(kubeClientset, kubeClient, opts, plugin)
		reports, err := scanner.Scan(ctx, workload)
		if err != nil {
			return err
		}
		writer := vulnerabilityreport.NewReadWriter(kubeClient)
		return writer.Write(ctx, reports)
	}
}
