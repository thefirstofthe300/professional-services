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

package images

import (
	"regexp"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/delete"
	"github.com/GoogleCloudPlatform/professional-services/tools/janitor/pkg/utils"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

// DeleteMetadata implements the delete.ResourceDeleter interface and provides
// the functions necessary to issue an image deletion call and watch the
// resulting delete operation's status.
type DeleteMetadata struct {
	ImageJanitorMetadata
	Call *compute.ImagesDeleteCall
}

// Do executes the ImagesDeletionCall for the given image.
func (id *DeleteMetadata) Do(s ...googleapi.CallOption) (*compute.Operation, error) {
	log.WithFields(log.Fields{
		"image": id.ImageJanitorMetadata.Image.Name,
	}).Info("Issuing delete")
	return id.Call.Do()
}

// Project returns the project of the image.
func (id *DeleteMetadata) Project() string {
	return id.ImageJanitorMetadata.Project
}

// Service returns the *compute.Service client used to issue deletion calls.
func (id *DeleteMetadata) Service() *compute.Service {
	return id.ImageJanitorMetadata.Service
}

// Zone returns the zone of the image.
func (id *DeleteMetadata) Zone() string {
	return ""
}

// ZoneOperationsService returns the ZoneOperationsService attached to the
// *compute.Service
func (id *DeleteMetadata) ZoneOperationsService() *compute.ZoneOperationsService {
	return id.ImageJanitorMetadata.Service.ZoneOperations
}

// RegionOperationsService returns the RegionOperationsService attached to the
// *compute.Service
func (id *DeleteMetadata) RegionOperationsService() *compute.RegionOperationsService {
	return id.ImageJanitorMetadata.Service.RegionOperations
}

// GlobalOperationsService returns the GlobalOperationsService attached to the
// *compute.Service
func (id *DeleteMetadata) GlobalOperationsService() *compute.GlobalOperationsService {
	return id.ImageJanitorMetadata.Service.GlobalOperations
}

// ImageJanitorMetadata wraps a single GCE compute image to be used by the
// DeleteMetadata struct.
type ImageJanitorMetadata struct {
	Service *compute.Service
	Project string
	Image   *compute.Image
}

// JanitorMetadata wraps the Google Compute API image service and implements the
// delete.JanitorMetadata interface so resources can be deleted via using the deletion
// engine.
type JanitorMetadata struct {
	Service           *compute.Service
	Project           string
	Items             []*compute.Image
	ExpiredBefore     time.Time
	DeleteSingletons  bool
	BlacklistPatterns []string
	NameDelimiter     string
}

// NewJanitorMetadata creates a new instance of JanitorMetadata.
func NewJanitorMetadata(s *compute.Service, p string, e time.Time, d bool, b []string, n string) *JanitorMetadata {
	return &JanitorMetadata{
		Project:          p,
		NameDelimiter:    n,
		DeleteSingletons: d,
		Service:          s,
		ExpiredBefore:    e,
	}
}

// Refresh queries the GCE API and populates i.Items with all current images.
func (i *JanitorMetadata) Refresh() error {
	ilc := i.Service.Images.List(i.Project).OrderBy("creationTimestamp desc")
	var itl []*compute.Image
	for {
		il, err := ilc.Do()
		if err != nil {
			log.WithField("error", err).Fatal("Error getting images")
		}

		for _, im := range il.Items {
			log.WithFields(log.Fields{
				"name":               im.Name,
				"creation_timestamp": im.CreationTimestamp,
				"image_family":       im.Family,
				"project":            i.Project,
			}).Info("Found image")
			itl = append(itl, im)
		}

		if il.NextPageToken == "" {
			break
		}
		ilc = ilc.PageToken(il.NextPageToken)
	}
	i.Items = itl
	return nil
}

// Blacklist removes blacklisted images from Images.Items.
func (i *JanitorMetadata) Blacklist() {
	var bi []*compute.Image
	if len(i.BlacklistPatterns) == 0 {
		log.Debug("No blacklist patterns")
		return
	}
	for _, im := range i.Items {
		for _, p := range i.BlacklistPatterns {
			r := regexp.MustCompile(p)
			if !r.MatchString(im.Name) {
				bi = append(bi, im)
			} else {
				log.WithFields(log.Fields{
					"image":             im.Name,
					"blacklist_pattern": p,
				}).Info("Image is blacklisted")
			}
		}
	}
	i.Items = bi
}

// Singletons removes images that by name are singleton.
func (i *JanitorMetadata) Singletons() {
	il := []*compute.Image{}
	nsm := make(map[string]struct {
		Name      string
		Singleton bool
	})
	for _, im := range i.Items {
		if _, ok := nsm[utils.GetResourceBasename(im.Name, i.NameDelimiter)]; !ok {
			nsm[utils.GetResourceBasename(im.Name, i.NameDelimiter)] = struct {
				Name      string
				Singleton bool
			}{im.Name, true}
			log.WithFields(log.Fields{
				"name":               im.Name,
				"creation_timestamp": im.CreationTimestamp,
				"reason":             "latest_version",
				"delete":             "ineligible",
			}).Info("Image excluded from deletion")
		} else {
			nsm[utils.GetResourceBasename(im.Name, i.NameDelimiter)] = struct {
				Name      string
				Singleton bool
			}{im.Name, false}
			il = append(il, im)
			log.WithFields(log.Fields{
				"name":               im.Name,
				"creation_timestamp": im.CreationTimestamp,
				"reason":             "old_version",
				"delete":             "eligible",
			}).Info("Image eligible for deletion")
		}
	}
	for _, v := range nsm {
		if v.Singleton == true {
			log.WithField("name", v.Name).Info("Image is a singleton")
		}
	}
	i.Items = il
}

// Expired removes the images older than the ExpiredBefore time.
func (i *JanitorMetadata) Expired() {
	iml := []*compute.Image{}

	for _, in := range i.Items {
		stamp, err := utils.ParseCreationTimestamp(in.CreationTimestamp)
		if err != nil {
			log.WithFields(log.Fields{
				"reason":             err,
				"creation_timestamp": in.CreationTimestamp,
				"image":              in.Name,
			}).Fatal("Failed to parse timestamp")
		}

		if stamp.Before(stamp) {
			log.WithFields(log.Fields{
				"image":              in.Name,
				"creation_timestamp": in.CreationTimestamp,
				"expired_by":         i.ExpiredBefore,
			}).Info("Image is older than expired_by date")
		} else {
			log.WithFields(log.Fields{
				"image":              in.Name,
				"creation_timestamp": in.CreationTimestamp,
				"expired_by":         i.ExpiredBefore,
			}).Info("Image is newer than expired_by date")
			iml = append(iml, in)
		}
	}
}

// Delete generates a list of DeleteMetadata structs that can be used to
// issue deletes for the list of images they wrap.
func (i JanitorMetadata) Delete(d chan<- delete.ResourceDeleter) {
	for _, im := range i.Items {
		log.WithFields(log.Fields{
			"name":    im.Name,
			"project": i.Project,
		}).Debug("Generating deletion call for image")
		d <- &DeleteMetadata{
			ImageJanitorMetadata: ImageJanitorMetadata{
				Service: i.Service,
				Project: i.Project,
				Image:   im,
			},
			Call: i.Service.Images.Delete(i.Project, im.Name),
		}
	}
	close(d)
}
