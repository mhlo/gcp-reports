// Copyright © 2017 Michael Boe <mboe@acm.org>
// This file is part of gcp-reports.

package cmd

import (
	"fmt"

	"github.com/spf13/viper"
	"google.golang.org/api/cloudresourcemanager/v1"
)

func filterProjects(gcpProjects []*cloudresourcemanager.Project, components []string, envList []string) (retProjects []*cloudresourcemanager.Project) {
	compKey := viper.GetString("componentKey")
	envKey := viper.GetString("envKey")
	fmt.Println("envKey:", envKey, ", compKey", compKey, ", envList", envList, ", components", components)
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
		fmt.Println("project", project.ProjectId, "labels", project.Labels, ", envMap", envMap, ", compMap", compMap)
		ok := true
		for idx, key := range []string{envKey, compKey} {
			if len(mapArr[idx]) != 0 && !mapArr[idx][project.Labels[key]] {
				fmt.Println(key, ": couldn't match:", project.Labels, mapArr[idx][project.Labels[key]])
				ok = false
				break
			}
		}
		if ok {
			retProjects = append(retProjects, project)
		}
	}
	return retProjects
}
