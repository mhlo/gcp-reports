// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"fmt"
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

		taker := &TakerGCP{crmService: cloudResourceManagerService, appEngine: appEngine}

		ourProjects := filterProjects(projectsResponse.Projects, args, envFilter)
		doneChan := make(chan string)
		for _, project := range ourProjects {
			fmt.Println("project pre:", project.gcpProject.ProjectId)
			go func(project *reportProject) {
				ingestErr := project.Ingest(taker)
				doneChan <- project.gcpProject.ProjectId
				fmt.Println("project inside done:", project.gcpProject.ProjectId, ingestErr)
			}(project)
		}
		for _ = range ourProjects {
			dp := <-doneChan
			fmt.Println("project done", dp)
		}
		fmt.Println("GCP information ingested...now to display")
		for _, project := range ourProjects {
			project.Display()
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
	appsCmd.Flags().Int("version-limit", 3, "How many versions (most recent) to be gathered")
	viper.BindPFlag("versionLimit", appsCmd.Flags().Lookup("version-limit"))

}
