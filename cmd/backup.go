// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/sqladmin/v1beta4"
	"google.golang.org/api/storage/v1"
)

var (
	env            string
	backup         string
	component      string
	withinDuration time.Duration
)

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backups",
	Short: "report on backups in various environments",
	Long: `Report on the state of various storage technologies in Google Cloud
for all projects, from the authenticated user/account point-of-view.
Each project of interest will have two labels associated with it.
One key is 'env' and the value is the environment for this command.
Another key is 'component' and is the value of the runtime component.
The third key is 'run-backups' and indicates, if true, that the storage should be backed up.
A GCS bucket which has label match of  'backup' and env:<env>
will be considered the backup bucket for this environment.
Backup objects within this bucket are named: /backup/<component>/<env>/<resource-kind>
Eg, /backup/foo/dev/datastore

CloudSQL backups do not work through GCS object storage; however, their existence
is discoverable via API. In this case, a project with a configured cloudSQL resource
should have backups enabled and should have a history of recent backups.
For each project discovered that should have backed-up storage, the program will
discover Cloud SQL instances and Datastore and determine if backups should be taken.
If so, it will check to see if a backup has been done within the interval specified by
the 'within' option (default is 24h).
`,
	Run: func(cmd *cobra.Command, args []string) {
		env = viper.GetString("envKey")
		component = viper.GetString("componentKey")
		backup = viper.GetString("backupKey")
		withinDuration = viper.GetDuration("within")

		fmt.Printf("using env key[%s], backup key[%s], component key[%s] across environments%v\n",
			env, backup, component, envFilter)
		ctx := oauth2.NoContext
		client, err := google.DefaultClient(ctx, cloudresourcemanager.CloudPlatformReadOnlyScope)
		if err != nil {
			log.Fatalln("cannot create a gcloud client:", err)
		}

		cloudResourceManagerService, err := cloudresourcemanager.New(client)
		if err != nil {
			log.Fatalln("cannot establish cloud resource-manager service:", err)
		}

		pService := cloudresourcemanager.NewProjectsService(cloudResourceManagerService)
		projects, projErr := pService.List().Context(ctx).Do()
		if projErr != nil {
			log.Fatalln("cannot list projects at Google Cloud:", projErr)
		}

		sqladminService, saErr := sqladmin.New(client)
		if saErr != nil {
			log.Fatalln("cannot create an sql admin service:", saErr)
		}

		storageService, storageErr := storage.New(client)
		// get GCS buckets that belong to project to see if any marked as backup
		if storageErr != nil {
			log.Fatalln("cannot use storage API successfully:", storageErr)
		}

		storageTaker := &TakerStorageGCP{
			storageService: storageService,
		}
		sqladminTaker := &TakerSQLAdminGCP{
			sqladminService: sqladminService,
		}
		ourProjects := filterProjects(projects.Projects, args, envFilter)

		// we now have a list of (filtered) projects that should have backups
		for _, project := range ourProjects {
			fmt.Printf("project ID[%32s]: env[%8s], component[%28s]\n",
				project.gcpProject.ProjectId, project.env, project.component)

			storageErr = project.IngestStorage(storageTaker)
			sqlErr := project.IngestSQLInstances(sqladminTaker)
			if storageErr != nil || sqlErr != nil {
				log.Println("at least some GCP info cannot be ingested:", sqlErr, storageErr)
			}
		}
	},
}

var (
	within       *time.Duration
	envKey       *string
	componentKey *string
	backupKey    *string
)

func init() {
	RootCmd.AddCommand(backupCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// backupCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	duration, _ := time.ParseDuration("24h")
	within = backupCmd.Flags().DurationP("within", "w", duration, "interval from now last backup should have occurred")
	envKey = backupCmd.Flags().String("env-key", "env", "platform label key describing environment")
	componentKey = backupCmd.Flags().String("component-key", "component", "platform label key describing component")
	backupKey = backupCmd.Flags().String("backup-key", "backup", "GCS label key whose value (true/false) indicates whether a bucket is a backup bucket for Datastore")

	// bind things together....
	viper.BindPFlag("within", backupCmd.Flags().Lookup("within"))
	viper.BindPFlag("envKey", backupCmd.Flags().Lookup("env-key"))
	viper.BindPFlag("componentKey", backupCmd.Flags().Lookup("component-key"))
	viper.BindPFlag("backupKey", backupCmd.Flags().Lookup("backup-key"))

}
