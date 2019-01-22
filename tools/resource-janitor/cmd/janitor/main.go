// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/professional-services/tools/resource-janitor/pkg/delete"
	"github.com/GoogleCloudPlatform/professional-services/tools/resource-janitor/pkg/utils"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
	yaml "gopkg.in/yaml.v2"
)

type BlacklistConfig struct {
	Instances []string `yaml:"instances"`
	Images    []string `yaml:"images"`
}

func main() {
	project := flag.String("project", "foo", "ID of the project to clean up")
	nameDelimiter := flag.String("image-delimiter", "-", "Delimiter used to separate parts of resource name")
	workers := flag.Int("workers", 10, "Delimiter used to separate parts of the resource name")
	olderThan := flag.Int64("older-than", 2592000, "Time in seconds that resources should not be older than")
	logFile := flag.String("log-file", "", "File to which output is sent. Default is STDOUT.")
	blacklistFile := flag.String("blacklist-file", "", "YAML config file with a list of naming schemes to ignore")
	deleteSingletons := flag.Bool("delete-singletons", false, "If set, all resources that are older than the time specified will be deleted regardless of whether they are the only resource of a certain name.")
	notDryRun := flag.Bool("not-dry-run", false, "Logs the changes that will be made without taking any actions.")

	flag.Parse()

	if *logFile != "" {
		file, err := os.Create(*logFile)
		if err != nil {
			fmt.Printf("main.go: unable to open log file: %s", err)
		}

		log.SetOutput(file)
	}

	blacklistConfig := BlacklistConfig{
		Instances: []string{},
		Images:    []string{},
	}
	if *blacklistFile != "" {
		blacklist, err := ioutil.ReadFile(*blacklistFile)
		if err != nil {
			fmt.Printf("main.go: unable to open blacklist file: %s", err)
		}

		err = yaml.Unmarshal(blacklist, &blacklistConfig)
		if err != nil {
			fmt.Printf("main.go: unable to parse blacklist file: %s", err)
		}
	}

	compute, err := initClient()
	if err != nil {
		log.Fatalf("main.go: unable to initialize Compute Engine client: %s", err)
	}

	tooOld := utils.GetTooOldTime(*olderThan)

	var wg sync.WaitGroup
	wg.Add(2)
	go deleteImages(compute, *project, tooOld, *nameDelimiter, *workers, *notDryRun, *deleteSingletons, blacklistConfig.Images, &wg)
	go deleteInstances(compute, *project, tooOld, *nameDelimiter, *workers, *notDryRun, *deleteSingletons, blacklistConfig.Instances, &wg)
	wg.Wait()
}

func initClient() (*compute.Service, error) {
	client, err := google.DefaultClient(oauth2.NoContext,
		"https://www.googleapis.com/auth/compute")
	if err != nil {
		return nil, err
	}

	computeService, err := compute.New(client)
	if err != nil {
		return nil, err
	}

	return computeService, nil
}

func deleteImages(computeSvc *compute.Service, project string, tooOld time.Time, nameDelimiter string, workers int, notDryRun bool, deleteSingletons bool, blacklist []string, wg *sync.WaitGroup) {
	images, err := utils.GetOldAndNonSingletonImages(computeSvc, project, tooOld, deleteSingletons, blacklist, nameDelimiter)
	if err != nil {
		log.Fatalf("main.go: unable to get list of images older than %s: %s", tooOld, err)
	}

	if len(images) == 0 {
		log.Printf("main.go: no images to delete")
	}

	if notDryRun {
		log.Printf("main.go: issuing parallel image delete.")

		err = delete.ParallelImages(computeSvc, project, workers, images)
		if err != nil {
			log.Fatalf("main.go: deletion exited with an error: %s", err)
		}

		log.Printf("main.go: successfully deleted old images")
	}

	wg.Done()
}

func deleteInstances(computeSvc *compute.Service, project string, tooOld time.Time, nameDelimiter string, workers int, notDryRun bool, deleteSingletons bool, blacklist []string, wg *sync.WaitGroup) {
	instances, err := utils.GetOldAndNonSingletonInstances(computeSvc, project, tooOld, deleteSingletons, blacklist, nameDelimiter)
	if err != nil {
		log.Fatalf("main.go: unable to get list of instances older than %s: %s", tooOld, err)
	}

	if len(instances) == 0 {
		log.Printf("main.go: no instances to delete")
	}

	if notDryRun {
		log.Printf("main.go: issuing parallel instances delete.")
		err = delete.ParallelInstances(computeSvc, project, workers, instances)
		if err != nil {
			log.Fatalf("main.go: deletion exited with an error: %s", err)
		}
		log.Printf("main.go: successfully deleted old instances")
	}

	wg.Done()
}

