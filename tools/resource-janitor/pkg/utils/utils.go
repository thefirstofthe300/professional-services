package utils

import (
	"strings"
	"log"
	"fmt"
	"time"
	"google.golang.org/api/compute/v1"
)

// GetOldAndNonSingletonImages returns a list of images that are older
// than the specified time and are not the only images with the same naming
// scheme.
func GetOldAndNonSingletonImages(computeService *compute.Service, project string, t time.Time, nameDelimiter string) ([]*compute.Image, error) {
	imageListCall := computeService.Images.List(project)

	allImages := []*compute.Image{}
	for {
		imageList, err := imageListCall.Do()
		if err != nil {
			return nil, fmt.Errorf("utils.go: Error getting images: %s", err)
		}

		for _, image := range imageList.Items {
			allImages = append(allImages, image)
		}
		if imageList.NextPageToken == "" {
			break
		}
		imageListCall = imageListCall.PageToken(imageList.NextPageToken)
	}

	nonSingletonImages := getNonSingletonImages(allImages, nameDelimiter)
	oldAndNonSingletonImages, err := getOldImages(nonSingletonImages, t)
	if err != nil {
		return nil, fmt.Errorf("utils.go: Unable to get old images: %s", err)
	}

	return oldAndNonSingletonImages, nil
}

func getNonSingletonImages(imageList []*compute.Image, nameDelimiter string) []*compute.Image {
	singleton := &compute.Image{}
	nonSingletons := []*compute.Image{}
	for i := range imageList {
		if i == 0 {
			singleton = imageList[i]
			continue
		}

		// Whether or not an image is singleton is tracked using the singleton
		// variable. If the two currently compared images match, neither can be
		// a singleton. If the two currently compared images don't match and the
		// singleton variable is set, the image is a singleton. Otherwise, the
		// image that is the current `i` value in the loop is a candidate to be a
		// singleton.
		if resourceNamesMatch(imageList[i-1].Name, imageList[i].Name, nameDelimiter) {
			singleton = &compute.Image{}
			nonSingletons = append(nonSingletons, imageList[i-1])
			if len(imageList)-1 == i {
				log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is most recent of its type\"", imageList[i].Name, imageList[i].CreationTimestamp)
			}
		} else if singleton.Name != "" {
			singleton = imageList[i]
			log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is singleton\"", singleton.Name, singleton.CreationTimestamp)
		} else {
			// Ignore the resource (imageList[i-1]) that was matched to a resource at some point
			// This block is run when the most recent of a certain type is at imageList[i-1]
			singleton = imageList[i]
			log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is most recent of its type\"", imageList[i-1].Name, imageList[i-1].CreationTimestamp)
			if len(imageList)-1 == i {
				log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is singleton\"", singleton.Name, singleton.CreationTimestamp)
			}
		}
	}
	return nonSingletons
}

func getOldImages(i []*compute.Image, t time.Time) ([]*compute.Image, error) {
	oldImages := []*compute.Image{}

	for _, image := range i {
		stamp, err := parseCreationTimestamp(image.CreationTimestamp)
		if err != nil {
			return nil, fmt.Errorf("utils.go: Failed to parse timestamp: %v", err)
		}

		if stamp.Before(t) {
			log.Printf("utils.go: selected image for deletion: name=%s creationTimestamp=%s reason=\"older than %s\"", image.Name, image.CreationTimestamp, t)
			oldImages = append(oldImages, image)
		} else {
			log.Printf("utils.go: excluded image from deletion: name=%s creationTimestamp=%s reason=\"newer than %s\"", image.Name, image.CreationTimestamp, t)
		}
	}

	return oldImages, nil
}

// GetOldAndNonSingletonInstances returns a list of instances that are older
// than the specified time and are not the only instances with the same naming
// scheme.
func GetOldAndNonSingletonInstances(computeService *compute.Service, project string, t time.Time, nameDelimiter string) ([]*compute.Instance, error) {
	zones, err := computeService.Zones.List(project).Do()
	if err != nil {
		log.Fatalf("utils.go: Unable to get list of zones: %s", err)
	}

	allInstances := []*compute.Instance{}

	for _, zone := range zones.Items {
		instanceListCall := computeService.Instances.List(project, zone.Name)

		for {
			instanceList, err := instanceListCall.Do()
			if err != nil {
				return nil, fmt.Errorf("utils.go: Error getting instances: %s", err)
			}

			for _, instance := range instanceList.Items {
				allInstances = append(allInstances, instance)
			}
			if instanceList.NextPageToken == "" {
				break
			}
			instanceListCall = instanceListCall.PageToken(instanceList.NextPageToken)
		}
	}

	nonSingletonInstances := getNonSingletonInstances(allInstances, nameDelimiter)
	oldAndNonSingletonInstances, err := getOldInstances(nonSingletonInstances, t)
	if err != nil {
		return nil, fmt.Errorf("utils.go: Unable to get old instances: %s", err)
	}

	return oldAndNonSingletonInstances, nil
}

func getNonSingletonInstances(instanceList []*compute.Instance, nameDelimiter string) []*compute.Instance {
	singleton := &compute.Instance{}
	nonSingletons := []*compute.Instance{}
	for i := range instanceList {
		if i == 0 {
			singleton = instanceList[i]
			continue
		}

		// Whether or not an instance is singleton is tracked using the singleton
		// variable. If the two currently compared instances match, neither can be
		// a singleton. If the two currently compared instances don't match and the
		// singleton variable is set, the instance is a singleton. Otherwise, the
		// instance that is the current `i` value in the loop is a candidate to be a
		// singleton.
		if resourceNamesMatch(instanceList[i-1].Name, instanceList[i].Name, nameDelimiter) {
			singleton = &compute.Instance{}
			nonSingletons = append(nonSingletons, instanceList[i-1])
			if len(instanceList)-1 == i {
				log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is most recent of its type\"", instanceList[i].Name, instanceList[i].CreationTimestamp)
			}
		} else if singleton.Name != "" {
			singleton = instanceList[i]
			log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is singleton\"", singleton.Name, singleton.CreationTimestamp)
		} else {
			// Ignore the resource (instanceList[i-1]) that was matched to a resource at some point
			// This block is run when the most recent of a certain type is at instanceList[i-1]
			singleton = instanceList[i]
			log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is most recent of its type\"", instanceList[i-1].Name, instanceList[i-1].CreationTimestamp)
			if len(instanceList)-1 == i {
				log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"instance is singleton\"", singleton.Name, singleton.CreationTimestamp)
			}
		}
	}
	return nonSingletons
}

func getOldInstances(i []*compute.Instance, t time.Time) ([]*compute.Instance, error) {
	oldInstances := []*compute.Instance{}

	for _, instance := range i {
		stamp, err := parseCreationTimestamp(instance.CreationTimestamp)
		if err != nil {
			return nil, fmt.Errorf("utils.go: Failed to parse timestamp: %v", err)
		}

		if stamp.Before(t) {
			log.Printf("utils.go: selected instance for deletion: name=%s creationTimestamp=%s reason=\"older than %s\"", instance.Name, instance.CreationTimestamp, t)
			oldInstances = append(oldInstances, instance)
		} else {
			log.Printf("utils.go: excluded instance from deletion: name=%s creationTimestamp=%s reason=\"newer than %s\"", instance.Name, instance.CreationTimestamp, t)
		}
	}

	return oldInstances, nil
}

// GetTooOldTime takes in a number of seconds and returns a time.Time that is that number of
// seconds from the current time.
func GetTooOldTime(i int64) time.Time {
	return time.Unix(time.Now().Unix()-i, 0)
}

func parseCreationTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func resourceNamesMatch(a string, b string, delimiter string) bool {
	aSplit := strings.Split(a, "-")
	aName := strings.Join(aSplit[:len(aSplit)-1], "-")
	bSplit := strings.Split(b, "-")
	bName := strings.Join(bSplit[:len(bSplit)-1], "-")

	return (aName == bName)
}
