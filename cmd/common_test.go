// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"testing"

	"github.com/spf13/viper"
	appengine "google.golang.org/api/appengine/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
)

type fpTestTable struct {
	envKey  string
	compKey string

	envList     []string
	compList    []string
	gcpProjList []*cloudresourcemanager.Project

	expectedRetProjects []*cloudresourcemanager.Project
}

var fpTT = []fpTestTable{
	{"env", "component", []string{"e1", "e2"}, []string{"c1", "c3"},
		gcpP,
		[]*cloudresourcemanager.Project{gcpP[0], gcpP[6]},
	},
	{"env", "component", []string{}, []string{"c1", "c3"},
		gcpP,
		[]*cloudresourcemanager.Project{gcpP[0], gcpP[1], gcpP[4], gcpP[6], gcpP[7], gcpP[8]},
	},
	{"env", "altcomponent", []string{}, []string{"c1", "c3"},
		gcpP,
		[]*cloudresourcemanager.Project{gcpP[10]},
	},
}

var p2a = map[string]*appengine.Application{
	"test1-project-000": &appengine.Application{
		Id:            "test1-000",
		ServingStatus: "SERVING",
	},
	"test1-project-006": &appengine.Application{
		Id:            "test1-006",
		ServingStatus: "SERVING",
	},
}

var a2s = map[string][]*appengine.Service{
	"test1-000": []*appengine.Service{
		&appengine.Service{Id: "default"},
		&appengine.Service{Id: "test1S1"},
		&appengine.Service{Id: "test1S2"},
	},
	"test1-006": []*appengine.Service{
		&appengine.Service{Id: "default"},
		&appengine.Service{Id: "test1.6S1"},
	},
}

var envTest1 = map[string]string{
	"envOne": "one",
	"envTwo": "two",
}

var s2v = map[string][]*appengine.Version{
	"test1-000/default": []*appengine.Version{
		&appengine.Version{Id: "mahjong", Env: "flexible", EnvVariables: envTest1, VersionUrl: "https://a.b.com/test1-000/default/mahjong"},
		&appengine.Version{Id: "holdem", Env: "standard", VersionUrl: "https://a.b.com/test1-000/default/holdem"},
	},
	"test1-000/test1S1": []*appengine.Version{
		&appengine.Version{Id: "v1", Env: "flexible", EnvVariables: envTest1, VersionUrl: "https://a.b.com/test1-000/test1S1/v1"},
		&appengine.Version{Id: "v2", Env: "standard", VersionUrl: "https://a.b.com/test1-000/test1S1/v2"},
	},
	"test1-000/test1S2": []*appengine.Version{
		&appengine.Version{Id: "v1", Env: "flexible", VersionUrl: "https://a.b.com/test1-000/test1S2/v1"},
		&appengine.Version{Id: "v2", Env: "standard", VersionUrl: "https://a.b.com/test1-000/test1S2/v2"},
	},
	"test1-006/default": []*appengine.Version{
		&appengine.Version{Id: "foo", Env: "flexible", VersionUrl: "https://a.b.com/test1-006/default/foo"},
		&appengine.Version{Id: "bar", Env: "standard", VersionUrl: "https://a.b.com/test1-006/default/bar"},
	},
	"test1-006/test1.6S1": []*appengine.Version{
		&appengine.Version{Id: "one", Env: "flexible", EnvVariables: envTest1, VersionUrl: "https://a.b.com/test1-000/test1.6S1/one"},
		&appengine.Version{Id: "two", Env: "standard", VersionUrl: "https://a.b.com/test1-000/test1.6S1/two"},
	},
}

var v2i = map[string][]*appengine.Instance{
	"test1-000/default/mahjong": []*appengine.Instance{
		&appengine.Instance{Id: "1"},
		&appengine.Instance{Id: "2"},
		&appengine.Instance{Id: "2a"},
	},
	"test1-000/test1S1/v2": []*appengine.Instance{
		&appengine.Instance{Id: "3"},
	},
	"test1-000/test1S2/v2": []*appengine.Instance{
		&appengine.Instance{Id: "4"},
	},
	"test1-006/default/bar": []*appengine.Instance{
		&appengine.Instance{Id: "5"},
		&appengine.Instance{Id: "6"},
	},
}

var gcpP = []*cloudresourcemanager.Project{
	&cloudresourcemanager.Project{ // 0
		Labels:    map[string]string{"env": "e1", "component": "c1"},
		ProjectId: "test1-project-000",
	},
	&cloudresourcemanager.Project{ // 1
		Labels:    map[string]string{"component": "c1"},
		ProjectId: "test1-project-001",
	},
	&cloudresourcemanager.Project{ // 2
		Labels:    map[string]string{"env": "e1"},
		ProjectId: "test1-project-002",
	},
	&cloudresourcemanager.Project{ // 3
		Labels:    map[string]string{"component": "c2"},
		ProjectId: "test1-project-003",
	},
	&cloudresourcemanager.Project{ // 4
		Labels:    map[string]string{"component": "c1"},
		ProjectId: "test1-project-004",
	},
	&cloudresourcemanager.Project{ // 5
		Labels:    map[string]string{"extraneous": "polevault"},
		ProjectId: "test1-project-005",
	},
	&cloudresourcemanager.Project{ // 6
		Labels:    map[string]string{"extraneous": "polevault", "env": "e1", "component": "c1"},
		ProjectId: "test1-project-006",
	},
	&cloudresourcemanager.Project{ // 7
		Labels:    map[string]string{"extraneous": "polevault", "altenv": "e1", "component": "c1"},
		ProjectId: "test1-project-007",
	},
	&cloudresourcemanager.Project{ // 8
		Labels:    map[string]string{"envbad": "ebad", "component": "c1"},
		ProjectId: "test1-project-008",
	},
	&cloudresourcemanager.Project{ // 9
		Labels:    map[string]string{},
		ProjectId: "test1-project-009",
	},
	&cloudresourcemanager.Project{ // 10
		Labels:    map[string]string{"extraneous": "polevault", "env": "e1", "altcomponent": "c1"},
		ProjectId: "test1-project-007",
	},
}

func idProj(pList []*reportProject) (displays []string) {
	for _, p := range pList {
		displays = append(displays, p.gcpProject.ProjectId)
	}
	return
}

func TestFilterProjects(t *testing.T) {
	for index, fpt := range fpTT {
		viper.Set("envKey", fpt.envKey)
		viper.Set("componentKey", fpt.compKey)
		outP := filterProjects(fpt.gcpProjList, fpt.compList, fpt.envList)
		if len(outP) != len(fpt.expectedRetProjects) {
			t.Errorf("TestFilterProjects: step %d: expected %d projects, but got %d projects: %v\n", index, len(fpt.expectedRetProjects), len(outP), idProj(outP))
		}
	}
}

type Stat struct {
	listCall int
	getCall  int
}

type ApplicationStats struct {
	Name          string
	ResourceStats map[string]Stat
}
type TestTaker struct {
	callStats map[string]*ApplicationStats
}

func (tt *TestTaker) GetApplication(rp *reportProject) (app *appengine.Application, err error) {
	app = p2a[rp.gcpProject.ProjectId]
	return
}

func (tt *TestTaker) ListServices(ra *reportApplication) (services []*appengine.Service, err error) {
	services = a2s[ra.gcpApplication.Id]
	return
}

func (tt *TestTaker) ListVersions(rs *reportService) (versions []*appengine.Version, err error) {
	versions = s2v[rs.application.gcpApplication.Id+"/"+rs.gcpService.Id]
	return
}
func (tt *TestTaker) ListVersionInstances(rv *reportVersion) (instances []*appengine.Instance, err error) {
	instances = v2i[rv.service.application.gcpApplication.Id+"/"+rv.service.gcpService.Id+"/"+rv.gcpVersion.Id]
	return
}

var ttaker = &TestTaker{callStats: make(map[string]*ApplicationStats, 0)}

// ListSQLInstances(*reportSQLInstance)
// ListBuckets(*reportBucket)

func TestIngestProjects(t *testing.T) {
	viper.Set("envKey", fpTT[0].envKey)
	viper.Set("componentKey", fpTT[0].compKey)
	verbose = true
	testProjList := filterProjects(fpTT[0].gcpProjList, fpTT[0].compList, fpTT[0].envList)
	for _, testProj := range testProjList {
		err := testProj.Ingest(ttaker)
		if err != nil {
			t.Errorf("app[%s] error calling TestTaker not expected: %s\n", testProj.gcpProject.ProjectId, err)
		}
	}
	if len(testProjList) != 2 {
		t.Errorf("bad number of filtered projects. Have %d expected 2\n", len(testProjList))
	}
	for _, testProj := range testProjList {
		if testProj.gcpProject.ProjectId != "test1-project-000" {
			continue
		}
		if testProj.application.gcpApplication.Id != "test1-000" {
			t.Errorf("unexpected name of app: have %s but expected %s\n", testProj.application.gcpApplication.Id, "test1-000")
		}
		if len(testProj.application.services) != 3 {
			t.Errorf("app[%s] bad number of services: have %d but expected 3\n", testProj.application.gcpApplication.Id, len(testProj.application.services))
		}
		if testProj.application.services[1].gcpService.Id != "test1S1" {
			t.Errorf("app[%s] second service name: have %s but expected %s\n", testProj.application.gcpApplication.Id, testProj.application.services[1].gcpService.Id, "test1S1")
		}
		service := testProj.application.services[1]
		if len(service.versions) != 2 {
			t.Errorf("app[%s] service[%s] bad number of versions: have %d but expected 2\n",
				testProj.application.gcpApplication.Id, service.gcpService.Id, len(service.versions))
		}
		version := service.versions[1]
		if version.gcpVersion.Id != "v1" {
			t.Errorf("app[%s] service[%s] second version ID: have %s but expected v1\n",
				testProj.application.gcpApplication.Id, service.gcpService.Id, version.gcpVersion.Id)
		}
		version = service.versions[0]
		if len(version.instances) != 1 {
			t.Errorf("app[%s] service[%s] bad number of instances: have %d but expected 1\n",
				testProj.application.gcpApplication.Id, service.gcpService.Id, len(version.instances))
		}
	}
}
