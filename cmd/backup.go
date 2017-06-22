// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"context"
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

type reportBucket struct {
	gcpBucket  *storage.Bucket
	gcpObjects []*storage.Object
}

type reportSQLInstance struct {
	gcpSQLInstance *sqladmin.DatabaseInstance
}

type reportProject struct {
	gcpProject *cloudresourcemanager.Project

	component     string
	env           string
	backupBuckets []*reportBucket
	sqlInstances  []*reportSQLInstance
}

var (
	env            string
	backup         string
	component      string
	withinDuration time.Duration
)

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
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

		sqlAdminService, saErr := sqladmin.New(client)
		if saErr != nil {
			log.Fatalln("cannot create an sql admin service:", saErr)
		}

		filteredProjectList := filterProjects(projects.Projects, args, envFilter)
		ourProjects := []*reportProject{}
		for _, p := range filteredProjectList {
			ourProject := &reportProject{gcpProject: p, component: p.Labels[*componentKey], env: p.Labels[*envKey]}
			ourProjects = append(ourProjects, ourProject)
		}

		// we now have a list of projects that should have backups (unless squashed)
		for _, project := range ourProjects {
			fmt.Printf("project ID[%32s]: env[%8s], component[%28s]\n",
				project.gcpProject.ProjectId, project.env, project.component)

			// get GCS buckets that belong to project to see if any marked as backup
			storageService, storageErr := storage.New(client)
			if storageErr == nil {
				storageErr = project.IngestStorageInfo(ctx, storageService)
			}
			if storageErr != nil {
				log.Fatalln("cannot use storage API successfully:", storageErr)
			}

			sqlErr := project.IngestSQLInstances(ctx, sqlAdminService)
			if sqlErr != nil {
				log.Fatalln("cannot use SQL-admin API successfully:", sqlErr)
			}
		}
	},
}

func (project *reportProject) IngestStorageInfo(ctx context.Context, storageService *storage.Service) error {
	bucketService := storage.NewBucketsService(storageService)
	if listResponse, err := bucketService.List(project.gcpProject.ProjectId).Do(); err == nil {
		for _, gcpBucket := range listResponse.Items {
			if gcpBucket.Labels[backup] == "" {
				continue
			}
			bucket := &reportBucket{gcpBucket: gcpBucket}
			project.backupBuckets = append(project.backupBuckets, bucket)
			updateTime, utErr := time.Parse(time.RFC3339, gcpBucket.Updated)
			if utErr != nil || time.Since(updateTime) > withinDuration {
				fmt.Printf("  backup bucket[%s] has not been backed up since %s\n", gcpBucket.Id, gcpBucket.Updated)
			} else {
				fmt.Printf("  backup bucket[%s] backed up %v ago\n", gcpBucket.Id, time.Since(updateTime))
				continue
			}

			fmt.Println("bucket:", gcpBucket.Id, gcpBucket.Kind, gcpBucket.Labels)
			objResponse, objErr := storageService.Objects.List(gcpBucket.Id).Do()
			if objErr != nil {
				fmt.Printf("  bad object get on bucket[%s]: %s\n", gcpBucket.Id, objErr)
				continue
			}
			bucket.gcpObjects = objResponse.Items
			if verbose {
				for _, object := range objResponse.Items {
					updateTime, utErr := time.Parse(time.RFC3339, object.Updated)
					if utErr != nil {
						fmt.Println("cannot parse date:", utErr)
						continue
					}
					fmt.Printf("    object[]%s] at [%s], size[%d]\n", object.Name, updateTime, object.Size)
				}
			}
			if len(objResponse.Items) == 0 {
				fmt.Println("    no backup listings seen!")
			}
		}
	} else {
		return err
	}
	return nil
}

func (project *reportProject) IngestSQLInstances(ctx context.Context, sqlAdminService *sqladmin.Service) error {
	sqlInstanceResponse, silErr := sqlAdminService.Instances.List(project.gcpProject.ProjectId).Do()
	if silErr != nil {
		return silErr
	}
	for _, gcpInstance := range sqlInstanceResponse.Items {
		instance := &reportSQLInstance{gcpSQLInstance: gcpInstance}
		project.sqlInstances = append(project.sqlInstances, instance)
		fmt.Printf("  sql instance[%s] has backup enabled[%t]\n", gcpInstance.Name, gcpInstance.Settings.BackupConfiguration.Enabled)
		if gcpInstance.Settings.BackupConfiguration.Enabled {
			backupResponse, backupErr := sqlAdminService.BackupRuns.List(project.gcpProject.ProjectId, gcpInstance.Name).Do()
			if backupErr != nil {
				fmt.Printf("  cannot get list of backup runs for instance[%s]: %v", gcpInstance.Name, backupErr)
				continue
			}
			maxRuns := 3
			if maxRuns >= len(backupResponse.Items) {
				maxRuns = len(backupResponse.Items)
			}
			for index, backupRun := range backupResponse.Items[0:maxRuns] {
				fmt.Printf("    backup [%2d]: enqueued[%16s] start[%16s] end[%16s]\n", index, backupRun.EnqueuedTime, backupRun.StartTime, backupRun.EndTime)
			}
		}
	}
	return nil
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
