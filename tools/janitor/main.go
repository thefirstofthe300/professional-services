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
	"os"
	"sync"

	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/delete"
	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/images"
	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/instances"
	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/utils"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
	yaml "gopkg.in/yaml.v2"
)

// BlacklistConfig stores a list of resources that should be ignored during
// deletion.
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
	verbosity := flag.String("verbosity", "info", "YAML config file with a list of naming schemes to ignore")
	deleteSingletons := flag.Bool("delete-singletons", false, "If set, all resources that are older than the time specified will be deleted regardless of whether they are the only resource of a certain name.")
	logType := flag.String("log-type", "text", "If set, all resources that are older than the time specified will be deleted regardless of whether they are the only resource of a certain name.")
	notDryRun := flag.Bool("not-dry-run", false, "Logs the changes that will be made without taking any actions.")

	flag.Parse()

	if *logFile != "" {
		file, err := os.Create(*logFile)
		if err != nil {
			fmt.Printf("main.go: unable to open log file: %s", err)
		}
		log.SetOutput(file)
	}

	switch *verbosity {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "panic":
		log.SetLevel(log.PanicLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	if *logType == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	blacklistConfig := BlacklistConfig{
		Instances: []string{},
		Images:    []string{},
	}

	if *blacklistFile != "" {
		blacklist, err := ioutil.ReadFile(*blacklistFile)
		if err != nil {
			log.Infof("main.go: unable to open blacklist file: %s", err)
		}

		err = yaml.Unmarshal(blacklist, &blacklistConfig)
		if err != nil {
			log.Infof("main.go: unable to parse blacklist file: %s", err)
		}
	}

	compute, err := initClient()
	if err != nil {
		log.Fatalf("main.go: unable to initialize Compute Engine client: %s", err)
	}

	tooOld := utils.GetTooOldTime(*olderThan)

	imtd := images.NewJanitorMetadata(compute, *project, tooOld, *deleteSingletons, blacklistConfig.Images, *nameDelimiter)
	intd := instances.NewJanitorMetadata(compute, *project, tooOld, *deleteSingletons, blacklistConfig.Instances, *nameDelimiter)

	var wg sync.WaitGroup
	wg.Add(2)
	go deleteImages(imtd, *workers, *notDryRun, &wg)
	go deleteInstances(intd, *workers, *notDryRun, &wg)
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

func deleteImages(i *images.JanitorMetadata, workers int, notDryRun bool, wg *sync.WaitGroup) {
	err := i.Refresh()
	i.Blacklist()
	i.Singletons()
	i.Expired()

	if len(i.Items) == 0 {
		log.Info("No images to delete")
		wg.Done()
		return
	}

	if notDryRun {
		err = delete.Parallel(workers, i)
		if err != nil {
			log.Fatalf("Deletion exited with an error: %s", err)
		}

		log.Info("Successfully cleaned up images")
	}

	wg.Done()
}

func deleteInstances(i *instances.JanitorMetadata, workers int, notDryRun bool, wg *sync.WaitGroup) {
	err := i.Refresh()
	i.Blacklist()
	i.Singletons()
	i.Expired()

	if len(i.Items) == 0 {
		log.Info("No instances to delete")
		wg.Done()
		return
	}

	if notDryRun {
		err = delete.Parallel(workers, i)
		if err != nil {
			log.Fatalf("Deletion exited with an error: %s", err)
		}

		log.Info("Successfully cleaned up instances")
	}

	wg.Done()
}
