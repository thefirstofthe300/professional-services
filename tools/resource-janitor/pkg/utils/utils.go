package utils

import (
	"regexp"
	"fmt"
	"google.golang.org/api/compute/v1"
	"log"
	"strings"
	"time"
)

// GetOldAndNonSingletonImages returns a list of images that are older
// than the specified time and are not the only images with the same naming
// scheme while leaving the newest image
func GetOldAndNonSingletonImages(computeService *compute.Service, project string, t time.Time, deleteSingletons bool, blacklist []string, nameDelimiter string) ([]*compute.Image, error) {
	imageListCall := computeService.Images.List(project).OrderBy("creationTimestamp desc")

	allImages := []*compute.Image{}
	for {
		imageList, err := imageListCall.Do()
		if err != nil {
			return nil, fmt.Errorf("utils.go: Error getting images: %s", err)
		}

		for _, image := range imageList.Items {
			for _, blacklistPattern := range blacklist {
				re := regexp.MustCompile(blacklistPattern)
				// If not blacklisted, add to images to process
				if !re.MatchString(image.Name) {
					allImages = append(allImages, image)
				} else {
					log.Printf("utils.go: ignoring blacklisted image: %s", image.Name)
				}
			}
		}
		if imageList.NextPageToken == "" {
			break
		}
		imageListCall = imageListCall.PageToken(imageList.NextPageToken)
	}

	var nonSingletonImages []*compute.Image
	if !deleteSingletons {
		nonSingletonImages = getNonSingletonImages(allImages, nameDelimiter)
	} else {
		nonSingletonImages = allImages
	}
	oldAndNonSingletonImages, err := getOldImages(nonSingletonImages, t)
	if err != nil {
		return nil, fmt.Errorf("utils.go: Unable to get old images: %s", err)
	}

	return oldAndNonSingletonImages, nil
}

func getNonSingletonImages(imageList []*compute.Image, nameDelimiter string) []*compute.Image {
	nonSingletonsMap := make(map[string]string)
	nonSingletons := []*compute.Image{}
	for i := range imageList {
		if _, ok := nonSingletonsMap[getResourceName(imageList[i].Name, nameDelimiter)]; !ok {
			nonSingletonsMap[getResourceName(imageList[i].Name, nameDelimiter)] = "yes"
			log.Printf("utils.go: image excluded from deletion: name=%s creationTimestamp=%s reason=\"image is newest of its kind\"", imageList[i].Name, imageList[i].CreationTimestamp)
		} else {
			nonSingletons = append(nonSingletons, imageList[i])
			log.Printf("utils.go: image eligible for deletion: name=%s creationTimestamp=%s reason=\"image is not a singleton\"", imageList[i].Name, imageList[i].CreationTimestamp)
		}
	}
	for k, v := range nonSingletonsMap {
		if v == "yes" {
			log.Printf("Image of naming scheme %s is a singleton", k)
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
func GetOldAndNonSingletonInstances(computeService *compute.Service, project string, t time.Time, deleteSingletons bool, blacklist []string, nameDelimiter string) ([]*compute.Instance, error) {
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
				for _, blacklistPattern := range blacklist {
					re := regexp.MustCompile(blacklistPattern)
					// If not blacklisted, add to instances to process
					if !re.MatchString(instance.Name) {
						allInstances = append(allInstances, instance)
					} else {
						log.Printf("utils.go: ignoring blacklisted instance: %s", instance.Name)
					}
				}
			}
			if instanceList.NextPageToken == "" {
				break
			}
			instanceListCall = instanceListCall.PageToken(instanceList.NextPageToken)
		}
	}

	var nonSingletonInstances []*compute.Instance
	if !deleteSingletons {
		nonSingletonInstances = getNonSingletonInstances(allInstances, nameDelimiter)
	} else {
		nonSingletonInstances = allInstances
	}
	oldAndNonSingletonInstances, err := getOldInstances(nonSingletonInstances, t)
	if err != nil {
		return nil, fmt.Errorf("utils.go: Unable to get old instances: %s", err)
	}

	return oldAndNonSingletonInstances, nil
}

func getNonSingletonInstances(instanceList []*compute.Instance, nameDelimiter string) []*compute.Instance {
	nonSingletonsMap := make(map[string]string)
	nonSingletons := []*compute.Instance{}
	for i := range instanceList {
		if _, ok := nonSingletonsMap[getResourceName(instanceList[i].Name, nameDelimiter)]; !ok {
			nonSingletonsMap[getResourceName(instanceList[i].Name, nameDelimiter)] = "yes"
			log.Printf("utils.go: instance excluded from deletion: name=%s creationTimestamp=%s reason=\"instance is newest of its kind\"", instanceList[i].Name, instanceList[i].CreationTimestamp)
		} else {
			nonSingletons = append(nonSingletons, instanceList[i])
			log.Printf("utils.go: instance eligible for deletion: name=%s creationTimestamp=%s reason=\"instance is not a singleton\"", instanceList[i].Name, instanceList[i].CreationTimestamp)
		}
	}
	for k, v := range nonSingletonsMap {
		if v == "yes" {
			log.Printf("Instance of naming scheme %s is a singleton", k)
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

func getResourceName(a string, delimiter string) string {
	aSplit := strings.Split(a, delimiter)
	return strings.Join(aSplit[:len(aSplit)-1], delimiter)
}

