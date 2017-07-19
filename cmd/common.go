// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/spf13/viper"
	"google.golang.org/api/appengine/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	storage "google.golang.org/api/storage/v1"
)

type Taker interface {
	GetApplication(*reportProject) (*appengine.Application, error)
	ListServices(*reportApplication) ([]*appengine.Service, error)
	ListVersions(*reportService) ([]*appengine.Version, error)
	ListVersionInstances(*reportVersion) ([]*appengine.Instance, error)
	// ListSQLInstances(*reportSQLInstance)
	// ListBuckets(*reportBucket)
}

type TakerSQLAdmin interface {
	ListSQLInstances(*reportProject) ([]*sqladmin.DatabaseInstance, error)
	ListBackupRuns(*reportProject, *reportSQLInstance) ([]*sqladmin.BackupRun, error)
}

type TakerStorage interface {
	ListBuckets(*reportProject) ([]*storage.Bucket, error)
	ListObjects(*reportBucket) ([]*storage.Object, error)
}

type reportNode interface {
	Parent() reportNode
	Ingest(Taker) error
}

type reportBucket struct {
	isBackup   bool
	gcpBucket  *storage.Bucket
	gcpObjects []*storage.Object
	objects    []*reportObject
	kindMap    map[string][]*reportObject

	project *reportProject
}

func (rb *reportBucket) Parent() reportNode {
	return rb.project
}

type reportSQLInstance struct {
	gcpSQLInstance *sqladmin.DatabaseInstance
	backupRuns     []*reportBackupRun

	project *reportProject // parent
}

type reportBackupRun struct {
	gcpBackupRun *sqladmin.BackupRun
}

func (rdb *reportSQLInstance) Parent() reportNode {
	return rdb.project
}

type reportVersionInstance struct {
	gcpVersionInstance *appengine.Instance

	version *reportVersion
}

func (rvi *reportVersionInstance) Parent() reportNode {
	return rvi.version
}

type reportVersion struct {
	gcpVersion *appengine.Version
	instances  []*reportVersionInstance
	deployTime time.Time

	service *reportService // parent
}

func (rv *reportVersion) Parent() reportNode {
	return rv.service
}

type reportService struct {
	gcpService *appengine.Service
	versions   []*reportVersion

	application *reportApplication //parent
}

func (rs *reportService) Parent() reportNode {
	return rs.application
}

type reportApplication struct {
	gcpApplication *appengine.Application

	services []*reportService

	project *reportProject // parent
}

func (ra *reportApplication) Parent() reportNode {
	return ra.project
}

type reportProject struct {
	gcpProject *cloudresourcemanager.Project

	component     string
	env           string
	backupBuckets []*reportBucket
	sqlInstances  []*reportSQLInstance
	application   *reportApplication
}

func (rp *reportProject) Parent() reportNode {
	return rp
}

type TakerGCP struct {
	crmService *cloudresourcemanager.Service
	appEngine  *appengine.APIService
}

type TakerStorageGCP struct {
	storageService *storage.Service
}

type TakerSQLAdminGCP struct {
	sqladminService *sqladmin.Service
}

// ListServices takes GCP-provided data about services provided by an application
func (taker *TakerGCP) ListServices(ra *reportApplication) (services []*appengine.Service, err error) {
	servicesService := appengine.NewAppsServicesService(taker.appEngine)
	serviceResponse, serr := servicesService.List(ra.gcpApplication.Id).Do()
	if err == nil {
		services = serviceResponse.Services
	}
	err = serr
	return
}

func (app *reportApplication) Ingest(taker Taker) error {
	if services, svcErr := taker.ListServices(app); svcErr != nil {
		return svcErr
	} else {
		doneChan := make(chan string)
		for _, service := range services {
			// fmt.Println("ingest service:", app.gcpApplication.Id+"."+service.Id)
			repService := &reportService{gcpService: service, application: app}
			app.services = append(app.services, repService)
			go func(service *reportService) {
				repService.Ingest(taker)
				doneChan <- repService.application.gcpApplication.Id + "." + service.gcpService.Id
			}(repService)
		}
		for range services {
			_ = <-doneChan
		}
	}
	return nil
}

// ListVersionInstances returns a list of instances running at a particular version
func (taker *TakerGCP) ListVersionInstances(rv *reportVersion) (instances []*appengine.Instance, err error) {
	versionsService := appengine.NewAppsServicesVersionsService(taker.appEngine)
	if instancesResponse, instanceErr := versionsService.Instances.List(rv.service.application.gcpApplication.Id, rv.service.gcpService.Id, rv.gcpVersion.Id).Do(); instanceErr == nil {
		instances = instancesResponse.Instances
	} else {
		err = instanceErr
	}
	return
}

func (rv *reportVersion) Ingest(taker Taker) error {
	if instances, instanceErr := taker.ListVersionInstances(rv); instanceErr == nil {
		for _, gcpInstance := range instances {
			instance := &reportVersionInstance{gcpVersionInstance: gcpInstance, version: rv}
			rv.instances = append(rv.instances, instance)
		}
	} else {
		return instanceErr
	}
	return nil
}

// ListVersions will take in all existing versions of the service in full detail.
func (taker *TakerGCP) ListVersions(rs *reportService) (versions []*appengine.Version, err error) {
	serviceService := appengine.NewAppsServicesVersionsService(taker.appEngine)
	if listResponse, listErr := serviceService.List(rs.application.gcpApplication.Id, rs.gcpService.Id).View("FULL").Do(); listErr == nil {
		versions = listResponse.Versions
	} else {
		err = listErr
	}
	return
}

// Ingest takes information 'taken' from the service providing the Google APIs
func (svc *reportService) Ingest(taker Taker) error {
	versions, versionErr := taker.ListVersions(svc)
	if versionErr != nil {
		return versionErr
	}
	versionLimit := viper.GetInt("versionLimit")
	if versionLimit > len(versions) {
		versionLimit = len(versions)
	}
	shortVersions := versions[len(versions)-versionLimit:]
	doneChan := make(chan string)
	for i := len(shortVersions) - 1; i >= 0; i-- {
		gcpVersion := versions[i]
		version := &reportVersion{gcpVersion: gcpVersion, service: svc}
		// fmt.Println("ingest version:", svc.application.gcpApplication.Id+"."+svc.gcpService.Id+"."+version.gcpVersion.Id)
		svc.versions = append(svc.versions, version)
		go func(version *reportVersion) {
			version.Ingest(taker)
			doneChan <- version.service.gcpService.Id + "." + version.gcpVersion.Id
		}(version)

		deployTime, utErr := time.Parse(time.RFC3339, gcpVersion.CreateTime)
		if utErr != nil {
			fmt.Println("cannot parse date:", utErr)
		}

		version.deployTime = deployTime

	}

	for i := len(shortVersions) - 1; i >= 0; i-- {
		_ = <-doneChan
	}

	return nil
}

type versionSlice []*reportVersion

func (o versionSlice) Len() int {
	return len(o)
}

func (o versionSlice) Less(i, j int) bool {
	return o[i].deployTime.After(o[j].deployTime)
}

func (o versionSlice) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

func (rs *reportService) Display() {
	fmt.Printf("  service[%18s], shard strat[%s]\n", rs.gcpService.Id, rs.gcpService.Split.ShardBy)

	limit := max(1, len(rs.versions)-2)

	if verbose {
		limit = len(rs.versions)
	} else if limit > 2 {
		limit = 3
		fmt.Println("    ...earlier versions elided...")
	}

	for _, version := range rs.versions[0:limit] {
		gcpVersion := version.gcpVersion
		env := supplyDefault(version.gcpVersion.Env, "standard")
		numInstances := len(version.instances)

		fmt.Printf("    version[%16s] runtime[%10s] env[%7s] serving[%12s] instances[%4d]\n",
			gcpVersion.Id, gcpVersion.Runtime, env, gcpVersion.ServingStatus, numInstances)
		if verbose {
			fmt.Printf("      deployed by[%s] at [%s]", gcpVersion.CreatedBy, gcpVersion.CreateTime)
			fmt.Printf("      url[%s]\n", gcpVersion.VersionUrl)
			fmt.Printf("      env-vars[%v]\n", gcpVersion.EnvVariables)
			if gcpVersion.BasicScaling != nil {
				fmt.Printf("      basic-scaling max[%4d] idle-timeout[%6s]",
					gcpVersion.BasicScaling.MaxInstances, gcpVersion.BasicScaling.IdleTimeout)
			}
			if gcpVersion.AutomaticScaling != nil {
				fmt.Printf("      auto-scaling max pending latency[%6s] max concurrent reqs[%6d] max total instances[%4d]\n",
					gcpVersion.AutomaticScaling.MaxPendingLatency,
					gcpVersion.AutomaticScaling.MaxConcurrentRequests,
					gcpVersion.AutomaticScaling.MaxTotalInstances,
				)
			}
			fmt.Printf("\n")
		}
		for _, handler := range gcpVersion.Handlers {
			fmt.Printf("      handler: URL regex[%26s], scriptpath[%s]\n",
				handler.UrlRegex, handler.Script.ScriptPath)
		}
	}
}

func (p *reportProject) Display() {
	if p.application != nil {
		p.application.Display()
	}
}

// Display sends appropriate output the console
func (app *reportApplication) Display() {
	fmt.Printf("application[%30s]: status[%s]\n", app.gcpApplication.Id, app.gcpApplication.ServingStatus)
	for _, dispatchRule := range app.gcpApplication.DispatchRules {
		fmt.Printf("  route: domain[%28s] dispatch[%18s] service[%16s]\n", dispatchRule.Domain, dispatchRule.Path, dispatchRule.Service)
	}
	for _, service := range app.services {
		service.Display()
	}
}

// GetApplication finds (maybe) an App Engine application associated with the project
func (taker *TakerGCP) GetApplication(rp *reportProject) (application *appengine.Application, err error) {
	getResponse, getErr := taker.appEngine.Apps.Get(rp.gcpProject.ProjectId).Do()
	if getErr != nil {
		//				log.Println("cannot get application for project:", appErr)
		err = getErr
	} else {
		application = getResponse
	}
	return
}

func (p *reportProject) Ingest(taker Taker) error {
	application, appErr := taker.GetApplication(p)
	if appErr != nil {
		//				log.Println("cannot get application for project:", appErr)
		return appErr
	}
	p.application = &reportApplication{gcpApplication: application, project: p}
	if siErr := p.application.Ingest(taker); siErr != nil {
		return siErr
	}

	return nil
}

func (taker TakerStorageGCP) ListObjects(bucket *reportBucket) (gcpObjects []*storage.Object, err error) {
	if objResponse, objErr := taker.storageService.Objects.List(bucket.gcpBucket.Id).Do(); objErr == nil {
		gcpObjects = objResponse.Items
	} else {
		err = objErr
	}
	return
}

type reportObject struct {
	gcpObject  *storage.Object
	updateTime time.Time
	kind       string
}

var objectDatastoreKindRegex = regexp.MustCompile("\\.([^.]+)\\.backup_info")

func (o *reportObject) DatastoreGleanMeta() {
	// organise an object map by 'kind', and then have a reverse-chronological listing of backups for that kind.
	matches := objectDatastoreKindRegex.FindStringSubmatch(o.gcpObject.Id)
	if len(matches) == 2 {
		o.kind = matches[1]
	}
}

type objectSlice []*reportObject

func (o objectSlice) Len() int {
	return len(o)
}

func (o objectSlice) Less(i, j int) bool {
	return o[i].updateTime.After(o[j].updateTime)
}

func (o objectSlice) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// UpdateKindMap updates (re-establishes) the map of kind(name): list of objects of same kind
func (rb *reportBucket) UpdateKindMap() {
	oSlice := objectSlice(rb.objects)
	sort.Sort(oSlice)
	kindMap := make(map[string][]*reportObject)
	for _, object := range rb.objects {
		if object.kind == "" {
			continue
		}
		kindMap[object.kind] = append(kindMap[object.kind], object)
	}
	rb.kindMap = kindMap
}

func ellipsize(s string, lhs int, rhs int) string {
	sz := len(s)
	if sz < (lhs + rhs + 3) {
		return s
	}
	return s[0:lhs] + "..." + s[sz-rhs:]
}

// IngestObjects takes in all objects in a GCS bucket
func (rb *reportBucket) IngestObjects(taker TakerStorage) (ingestErr error) {
	gcpObjects, listObjErr := taker.ListObjects(rb)
	if listObjErr != nil {
		ingestErr = listObjErr
		return
	}
	fmt.Printf("bucket %s has %d objects\n", rb.gcpBucket.Id, len(gcpObjects))
	for _, gcpObject := range gcpObjects {
		updateTime, utErr := time.Parse(time.RFC3339, gcpObject.Updated)
		if utErr != nil {
			fmt.Println("cannot parse date:", utErr)
		}
		object := &reportObject{gcpObject: gcpObject, updateTime: updateTime}
		object.DatastoreGleanMeta()
		rb.objects = append(rb.objects, object)
	}

	rb.UpdateKindMap()
	for kind, objectSlice := range rb.kindMap {
		fmt.Printf("    kind[%s] most recently updated object[%s] at [%s], size[%d]\n", kind,
			ellipsize(objectSlice[0].gcpObject.Id, 8, 12), objectSlice[0].updateTime, objectSlice[0].gcpObject.Size)
	}
	return
}

// ListBuckets queries actual GCP to get buckets for a project
func (taker TakerStorageGCP) ListBuckets(project *reportProject) (gcpBuckets []*storage.Bucket, err error) {
	if objResponse, objErr := taker.storageService.Buckets.List(project.gcpProject.ProjectId).Do(); objErr == nil {
		gcpBuckets = objResponse.Items
	} else {
		err = objErr
	}
	return
}

func (p *reportProject) IngestStorage(taker TakerStorage) (ingestErr error) {
	gcpBuckets, listErr := taker.ListBuckets(p)
	if listErr == nil {
		for _, gcpBucket := range gcpBuckets {
			isBackup := false
			if gcpBucket.Labels[*backupKey] == "true" {
				isBackup = true
			}
			bucket := &reportBucket{gcpBucket: gcpBucket, isBackup: isBackup}
			p.backupBuckets = append(p.backupBuckets, bucket)
			if isBackup {
				ingestErr = bucket.IngestObjects(taker)
			}

		}
	} else {
		ingestErr = listErr
	}
	return
}

// ListSQLInstances lists out the SQL instances associated with the given project
func (taker TakerSQLAdminGCP) ListSQLInstances(project *reportProject) (gcpInstances []*sqladmin.DatabaseInstance, err error) {
	sqlInstanceResponse, silErr := taker.sqladminService.Instances.List(project.gcpProject.ProjectId).Do()
	if silErr == nil {
		gcpInstances = sqlInstanceResponse.Items
	}
	err = silErr
	return
}

// ListBackupRuns gathers any listed backup-runs for the given SQL Instance
func (taker *TakerSQLAdminGCP) ListBackupRuns(project *reportProject, dbi *reportSQLInstance) (gcpRuns []*sqladmin.BackupRun, err error) {
	backupResponse, backupErr := taker.sqladminService.BackupRuns.List(project.gcpProject.ProjectId, dbi.gcpSQLInstance.Name).Do()
	if backupErr == nil {
		gcpRuns = backupResponse.Items
	}
	err = backupErr
	return
}

// IngestSQLInstances ingests all the SQL instances for this project
func (p *reportProject) IngestSQLInstances(taker TakerSQLAdmin) error {
	gcpInstances, listErr := taker.ListSQLInstances(p)
	if listErr != nil {
		return listErr
	}
	for _, gcpInstance := range gcpInstances {
		instance := &reportSQLInstance{gcpSQLInstance: gcpInstance}
		p.sqlInstances = append(p.sqlInstances, instance)
		fmt.Printf("  sql instance[%s] has backup enabled[%t]\n", gcpInstance.Name, gcpInstance.Settings.BackupConfiguration.Enabled)
		if gcpInstance.Settings.BackupConfiguration.Enabled {
			gcpBackups, backupErr := taker.ListBackupRuns(p, instance)
			if backupErr != nil {
				fmt.Printf("  cannot get list of backup runs for instance[%s]: %v", gcpInstance.Name, backupErr)
				continue
			}
			for _, gcpBackup := range gcpBackups {
				backup := &reportBackupRun{gcpBackupRun: gcpBackup}
				instance.backupRuns = append(instance.backupRuns, backup)
			}
			maxRuns := 3
			if maxRuns >= len(gcpBackups) {
				maxRuns = len(gcpBackups)
			}
			for index, backupRun := range instance.backupRuns[0:maxRuns] {
				fmt.Printf("    backup [%2d]: enqueued[%16s] start[%16s] end[%16s]\n",
					index, backupRun.gcpBackupRun.EnqueuedTime, backupRun.gcpBackupRun.StartTime, backupRun.gcpBackupRun.EndTime)
			}
		}
	}
	return nil
}

func filterProjects(gcpProjects []*cloudresourcemanager.Project, components []string, envList []string) (retProjects []*reportProject) {
	compKey := viper.GetString("componentKey")
	envKey := viper.GetString("envKey")
	// fmt.Println("envKey:", envKey, ", compKey", compKey, ", envList", envList, ", components", components)
	compMap := make(map[string]bool, 0)
	envMap := make(map[string]bool, 0)
	for _, component := range components {
		compMap[component] = true
	}
	for _, e := range envList {
		envMap[e] = true
	}
	mapArr := []map[string]bool{envMap, compMap}

	for _, project := range gcpProjects {
		// fmt.Println("project", project.ProjectId, "labels", project.Labels, ", envMap", envMap, ", compMap", compMap)
		ok := true
		for idx, key := range []string{envKey, compKey} {
			if len(mapArr[idx]) != 0 && !mapArr[idx][project.Labels[key]] {
				// fmt.Println(key, ": couldn't match:", idx, key, project.Labels, mapArr[idx][project.Labels[key]])
				ok = false
				break
			}
		}
		if ok {
			retProj := &reportProject{gcpProject: project, env: project.Labels[envKey], component: project.Labels[compKey]}
			retProjects = append(retProjects, retProj)
		}
	}
	return retProjects
}
