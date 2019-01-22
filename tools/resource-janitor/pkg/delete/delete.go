package delete

import (
	"fmt"
	"strings"
	compute "google.golang.org/api/compute/v1"
	"log"
	"sync"
	"time"
	"google.golang.org/api/googleapi"
)

type resourceDelete interface {
	Do(...googleapi.CallOption) (*compute.Operation, error)
}

// ParallelImages issues a parallel delete for listed images
func ParallelImages(computeSvc *compute.Service, project string, workers int, imageList []*compute.Image) error {
	images := make(chan resourceDelete, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		log.Printf("delete.go: image-worker-%d: starting", i)
		go deleteWorker(fmt.Sprintf("image-worker-%d", i), computeSvc, project, images, &wg)
	}

	for _, image := range imageList {
		images <- computeSvc.Images.Delete(project, image.Name)
	}

	close(images)

	wg.Wait()
	return nil
}

// ParallelInstances issues a parallel delete for listed instances
func ParallelInstances(computeSvc *compute.Service, project string, workers int, instanceList []*compute.Instance) error {
	instances := make(chan resourceDelete, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		log.Printf("delete.go: instance-worker-%d: Starting", i)
		go deleteWorker(fmt.Sprintf("delete.go: instance-worker-%d", i), computeSvc, project, instances, &wg)
	}

	for _, instance := range instanceList {
		log.Printf("delete.go: Deleting instance: project=%s zone=%s name=%s", project, sanitizeResourceURL(instance.Zone), instance.Name)
		instances <- computeSvc.Instances.Delete(project, sanitizeResourceURL(instance.Zone), instance.Name)
	}

	close(instances)

	wg.Wait()
	return nil
}

func deleteWorker(id string, computeSvc *compute.Service, project string, resourceDeleteCalls <-chan resourceDelete, wg *sync.WaitGroup) {
	defer wg.Done()
	for call := range resourceDeleteCalls {
		deleteOperation, err := call.Do()
		if err != nil {
			log.Fatalf("%s: Unable to issue delete call %v: %s", id, call, err)
		}
		var queryDeleteOperation resourceDelete
		if deleteOperation.Zone != "" {
			queryDeleteOperation = computeSvc.ZoneOperations.Get(project, sanitizeResourceURL(deleteOperation.Zone), deleteOperation.Name)
		} else if deleteOperation.Region != "" {
			queryDeleteOperation = computeSvc.RegionOperations.Get(project, sanitizeResourceURL(deleteOperation.Region), deleteOperation.Name)
		} else {
			queryDeleteOperation = computeSvc.GlobalOperations.Get(project, deleteOperation.Name)
		}
		for {
			toSleep, _ := time.ParseDuration("3s")
			time.Sleep(toSleep)
			deleteOperation, err = queryDeleteOperation.Do()
			if err != nil {
				log.Fatalf("%s: Unable to fetch operation %s: %s", id, deleteOperation.Name, err)
			}

			if deleteOperation.Status == "DONE" {
				break
			}
		}
		log.Printf("delete.go: %s: Deleted resource %s", id, deleteOperation.TargetLink)
	}
	log.Printf("delete.go: %s: Stopping", id)
}

func sanitizeResourceURL(z string) string {
	splitZone := strings.Split(z, "/")
	return splitZone[len(splitZone)-1]
}
