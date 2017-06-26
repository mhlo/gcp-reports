// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"fmt"

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

type reportNode interface {
	Parent() reportNode
	Ingest(Taker) error
}

type reportBucket struct {
	gcpBucket  *storage.Bucket
	gcpObjects []*storage.Object

	project *reportProject
}

func (rb *reportBucket) Parent() reportNode {
	return rb.project
}

type reportSQLInstance struct {
	gcpSQLInstance *sqladmin.DatabaseInstance

	project *reportProject // parent
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
		for _, service := range services {
			// fmt.Println("ingest service:", app.gcpApplication.Id+"."+service.Id)
			repService := &reportService{gcpService: service, application: app}
			app.services = append(app.services, repService)
			repService.Ingest(taker)
		}
	}
	return nil
}

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

func (taker *TakerGCP) ListVersions(rs *reportService) (versions []*appengine.Version, err error) {
	serviceService := appengine.NewAppsServicesVersionsService(taker.appEngine)
	if listResponse, listErr := serviceService.List(rs.application.gcpApplication.Id, rs.gcpService.Id).Do(); listErr == nil {
		versions = listResponse.Versions
	} else {
		err = listErr
	}
	return
}

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
	}
	return nil
}

func (rs *reportService) Display() {
	fmt.Printf("  service[%18s], shard strat[%s]\n", rs.gcpService.Id, rs.gcpService.Split.ShardBy)

	limit := max(0, len(rs.versions)-2)
	if limit > 0 {
		if verbose {
			limit = 0
		} else {
			fmt.Println("    ...earlier versions elided...")
		}
	}

	for _, version := range rs.versions[limit:len(rs.versions)] {
		gcpVersion := version.gcpVersion
		env := supplyDefault(version.gcpVersion.Env, "standard")
		numInstances := len(version.instances)

		fmt.Printf("    version[%16s] runtime[%10s] env[%7s] serving[%12s] instances[%4d]\n",
			gcpVersion.Id, gcpVersion.Runtime, env, gcpVersion.ServingStatus, numInstances)
		if verbose {
			fmt.Printf("      url[%s]\n", gcpVersion.VersionUrl)
			fmt.Printf("      env-vars[%v]\n", gcpVersion.EnvVariables)
			if gcpVersion.BasicScaling != nil {
				fmt.Printf("      basic-scaling max[%4d] idle-timeout[%6d]",
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
				handler.UrlRegex, handler.ApiEndpoint.ScriptPath)
		}
	}
}

func (p *reportProject) Display() {
	if p.application != nil {
		p.application.Display()
	}
}

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
