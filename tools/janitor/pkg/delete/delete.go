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

package delete

import (
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/utils"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

// ResourceDeleter provides an interface that is used by the individual
// deletion workers.
type ResourceDeleter interface {
	Project() string
	Service() *compute.Service
	Do(...googleapi.CallOption) (*compute.Operation, error)
	Zone() string
	ZoneOperationsService() *compute.ZoneOperationsService
	RegionOperationsService() *compute.RegionOperationsService
	GlobalOperationsService() *compute.GlobalOperationsService
}

// ResourceGetter provides an interface used to allow for querying for operation's
// status to be mostly generic.
type ResourceGetter interface {
	Do(s ...googleapi.CallOption) (*compute.Operation, error)
}

// JanitoryMetadata provides an interface to be used by each resource that
// will be deleted by the deletion engine.
type JanitorMetadata interface {
	Delete(chan<- ResourceDeleter)
}

// Parallel issues a parallel delete by starting an engine comprised of workers
// capable of deleting resources.
func Parallel(workers int, i JanitorMetadata) error {
	r := make(chan ResourceDeleter, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go deleteWorker(fmt.Sprintf("instance-worker-%d", i), r, &wg)
	}

	i.Delete(r)

	wg.Wait()
	return nil
}

func deleteWorker(id string, resourceDeleteCalls <-chan ResourceDeleter, wg *sync.WaitGroup) {
	idLog := log.WithFields(log.Fields{
		"worker": id,
	})
	idLog.Info("Starting delete worker")
	defer wg.Done()
	for call := range resourceDeleteCalls {
		deleteOperation, err := call.Do()
		if err != nil {
			idLog.WithField("error", err).Fatal("Unable to issue delete call")
		}
		var queryDeleteOperation ResourceGetter
		if deleteOperation.Zone != "" {
			queryDeleteOperation = call.ZoneOperationsService().Get(call.Project(), utils.GetResourceNameFromURL(deleteOperation.Zone), deleteOperation.Name)
		} else if deleteOperation.Region != "" {
			queryDeleteOperation = call.RegionOperationsService().Get(call.Project(), utils.GetResourceNameFromURL(deleteOperation.Region), deleteOperation.Name)
		} else {
			queryDeleteOperation = call.GlobalOperationsService().Get(call.Project(), deleteOperation.Name)
		}
		for {
			toSleep, _ := time.ParseDuration("3s")
			time.Sleep(toSleep)
			deleteOperation, err = queryDeleteOperation.Do()
			if err != nil {
				idLog.WithFields(log.Fields{
					"operation_id": deleteOperation.Name,
					"error":        err,
				}).Fatal("Unable to get operation")
			}

			if deleteOperation.Status == "DONE" {
				break
			} else {
				idLog.WithFields(log.Fields{
					"operation_id": deleteOperation.Name,
					"start_time":   deleteOperation.StartTime,
					"status":       deleteOperation.Status,
					"resource":     deleteOperation.TargetLink,
				}).Info("Deleting resource")
			}
		}
		idLog.WithFields(log.Fields{
			"resource": deleteOperation.TargetLink,
		}).Info("Deleted resource")
	}
	idLog.Info("Stopping worker")
}
