// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"fmt"
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/spf13/cobra"
	"google.golang.org/api/appengine/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
)

func max(i1, i2 int) int {
	if i1 > i2 {
		return i1
	}
	return i2
}

func supplyDefault(s, defaults string) string {
	if s == "" {
		return defaults
	}
	return s
}

// appsCmd represents the apps command
var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "Show Google App Engine details available from given credentials",
	Long: `Show Google App Engine instances associated with project resources
that are visible from the account used. At its most basic, this will output
information about all services and versions running as part of an app. To
see apps matching a particular component name of 'our-foo', try this:
    gcp-reports apps our-foo
Applications with the 'component' label matching 'our-foo' will be listed.
`,
	Run: func(cmd *cobra.Command, args []string) {

		ctx := oauth2.NoContext
		client, err := google.DefaultClient(ctx, appengine.CloudPlatformReadOnlyScope)
		if err != nil {
			log.Fatalln("cannot create a gcloud client:", err)
		}

		cloudResourceManagerService, err := cloudresourcemanager.New(client)
		if err != nil {
			log.Fatalln("cannot establish cloud resource-manager service:", err)
		}

		pService := cloudresourcemanager.NewProjectsService(cloudResourceManagerService)
		projectsResponse, projErr := pService.List().Context(ctx).Do()
		if projErr != nil {
			log.Fatalln("cannot list projects at Google Cloud:", projErr)
		}

		appEngine, err := appengine.New(client)
		if err != nil {
			log.Fatalln("cannot establish app engine service:", err)
		}
		servicesService := appengine.NewAppsServicesService(appEngine)
		versionsService := appengine.NewAppsServicesVersionsService(appEngine)

		ourProjects := filterProjects(projectsResponse.Projects, args, envFilter)
		// by default, search all projects we are allowed to see.
		// TODO: scope that differently (command-line? a google-deployment-mgr file?)
		for _, project := range ourProjects {
			application, appErr := appEngine.Apps.Get(project.ProjectId).Do()
			if appErr != nil {
				//				log.Println("cannot get application for project:", appErr)
				continue
			}
			fmt.Printf("application[%30s]: status[%s]\n", application.Id, application.ServingStatus)
			services, svcErr := servicesService.List(application.Id).Do()
			if svcErr != nil {
				log.Println("failure to get services:", svcErr)
			}

			for _, dispatchRule := range application.DispatchRules {
				fmt.Printf("  route: domain[%28s] dispatch[%18s] service[%16s]\n", dispatchRule.Domain, dispatchRule.Path, dispatchRule.Service)
			}

			for _, service := range services.Services {
				fmt.Printf("  service[%18s], shard strategy[%s]\n", service.Id, service.Split.ShardBy)
				if versions, versionErr := versionsService.List(application.Id, service.Id).Do(); versionErr == nil {
					limit := max(0, len(versions.Versions)-2)
					if limit > 0 {
						if verbose {
							limit = 0
						} else {
							fmt.Println("    ...earlier versions elided...")
						}
					}
					for _, version := range versions.Versions[limit:len(versions.Versions)] {
						env := supplyDefault(version.Env, "standard")
						instances, instanceErr := versionsService.Instances.List(application.Id, service.Id, version.Id).Do()
						numInstances := -1
						if instanceErr != nil {
							log.Printf("    cannot get instance data for version[%s]\n", version.Id)
						} else {
							numInstances = len(instances.Instances)
						}
						fmt.Printf("    version[%16s] runtime[%10s] env[%7s] serving[%12s] instances[%4d]\n",
							version.Id, version.Runtime, env, version.ServingStatus, numInstances)
						if verbose {
							fmt.Printf("      url[%s]\n", version.VersionUrl)
							fmt.Printf("      env-vars[%v]\n", version.EnvVariables)
							if version.BasicScaling != nil {
								fmt.Printf("      basic-scaling max[%4d] idle-timeout[%6d]",
									version.BasicScaling.MaxInstances, version.BasicScaling.IdleTimeout)
							}
							if version.AutomaticScaling != nil {
								fmt.Printf("      auto-scaling max pending latency[%6s] max concurrent reqs[%6d] max total instances[%4d]\n",
									version.AutomaticScaling.MaxPendingLatency,
									version.AutomaticScaling.MaxConcurrentRequests,
									version.AutomaticScaling.MaxTotalInstances,
								)
							}
							fmt.Printf("\n")
						}
						for _, handler := range version.Handlers {
							fmt.Printf("      handler: URL regex[%26s], scriptpath[%s]\n",
								handler.UrlRegex, handler.ApiEndpoint.ScriptPath)
						}
					}

				} else {
					log.Printf("    trouble retrieving versions for service[%s]\n", service.Id)
				}
			}

		}
	},
}

func init() {
	RootCmd.AddCommand(appsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// appsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// appsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}
