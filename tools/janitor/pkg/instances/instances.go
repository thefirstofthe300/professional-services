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

package instances

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
// the functions necessary to issue an instance deletion call and watch the
// resulting delete operation's status.
type DeleteMetadata struct {
	InstanceJanitorMetadata
	Call *compute.InstancesDeleteCall
}

// Do executes the InstancesDeletionCall for the given instance.
func (id *DeleteMetadata) Do(s ...googleapi.CallOption) (*compute.Operation, error) {
	log.WithFields(log.Fields{
		"instance": id.InstanceJanitorMetadata.Instance.Name,
	}).Info("Issuing delete")
	return id.Call.Do()
}

// Project returns the project of the instance.
func (id *DeleteMetadata) Project() string {
	return id.InstanceJanitorMetadata.Project
}

// Service returns the *compute.Service client used to issue deletion calls.
func (id *DeleteMetadata) Service() *compute.Service {
	return id.InstanceJanitorMetadata.Service
}

// Zone returns the zone of the instance.
func (id *DeleteMetadata) Zone() string {
	return id.InstanceJanitorMetadata.Instance.Zone
}

// ZoneOperationsService returns the ZoneOperationsService attached to the
// *compute.Service
func (id *DeleteMetadata) ZoneOperationsService() *compute.ZoneOperationsService {
	return id.InstanceJanitorMetadata.Service.ZoneOperations
}

// RegionOperationsService returns the RegionOperationsService attached to the
// *compute.Service
func (id *DeleteMetadata) RegionOperationsService() *compute.RegionOperationsService {
	return id.InstanceJanitorMetadata.Service.RegionOperations
}

// GlobalOperationsService returns the GlobalOperationsService attached to the
// *compute.Service
func (id *DeleteMetadata) GlobalOperationsService() *compute.GlobalOperationsService {
	return id.InstanceJanitorMetadata.Service.GlobalOperations
}

// JanitorMetadata wraps the Google Compute API Instance service and provides a function
// library for easy manipulation of Instances.
type InstanceJanitorMetadata struct {
	Service  *compute.Service
	Project  string
	Instance *compute.Instance
}

// JanitorMetadata wraps the Google Compute API Instance service and provides a function
// library for easy manipulation of Instances.
type JanitorMetadata struct {
	Service           *compute.Service
	Project           string
	Items             []*compute.Instance
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

// Refresh queries the GCE API and populates i.Items with all current instances.
func (i *JanitorMetadata) Refresh() error {
	zones, err := i.Service.Zones.List(i.Project).Do()
	if err != nil {
		log.WithField("error", err).Fatal("Unable to list zones", err)
	}
	var itl []*compute.Instance
	for _, zone := range zones.Items {
		ilc := i.Service.Instances.List(i.Project, zone.Name).OrderBy("creationTimestamp desc")
		for {
			il, err := ilc.Do()
			if err != nil {
				log.WithField("error", err).Fatal("Error getting Instances")
			}

			for _, in := range il.Items {
				log.WithFields(log.Fields{
					"name":               in.Name,
					"creation_timestamp": in.CreationTimestamp,
					"project":            i.Project,
					"zone":               zone.Name,
				}).Info("Found instance")
				itl = append(itl, in)
			}

			if il.NextPageToken == "" {
				break
			}
			ilc = ilc.PageToken(il.NextPageToken)
		}
	}
	i.Items = itl
	return nil
}

// Blacklist removes blacklisted instances.
func (i *JanitorMetadata) Blacklist() {
	var bi []*compute.Instance
	if len(i.BlacklistPatterns) == 0 {
		log.Debug("No blacklist patterns")
		return
	}
	for _, in := range i.Items {
		for _, p := range i.BlacklistPatterns {
			r := regexp.MustCompile(p)
			if !r.MatchString(in.Name) {
				bi = append(bi, in)
			} else {
				log.WithFields(log.Fields{
					"instance":          in.Name,
					"blacklist_pattern": p,
					"status":            "blacklisted",
				}).Info("Instance is blacklisted")
			}
		}
	}
	i.Items = bi
}

// Singletons removes instances that are singleton by name.
func (i *JanitorMetadata) Singletons() {
	il := []*compute.Instance{}
	nsm := make(map[string]struct {
		Name      string
		Singleton bool
	})
	for _, in := range i.Items {
		if _, ok := nsm[utils.GetResourceBasename(in.Name, i.NameDelimiter)]; !ok {
			nsm[utils.GetResourceBasename(in.Name, i.NameDelimiter)] = struct {
				Name      string
				Singleton bool
			}{in.Name, true}
			log.WithFields(log.Fields{
				"name":               in.Name,
				"creation_timestamp": in.CreationTimestamp,
				"status":             "latest",
				"delete":             "ineligible",
			}).Info("Instance excluded from deletion")
		} else {
			nsm[utils.GetResourceBasename(in.Name, i.NameDelimiter)] = struct {
				Name      string
				Singleton bool
			}{in.Name, false}
			il = append(il, in)
			log.WithFields(log.Fields{
				"name":               in.Name,
				"creation_timestamp": in.CreationTimestamp,
				"status":             "superseded",
				"delete":             "eligible",
			}).Info("Instance eligible for deletion")
		}
	}
	for _, v := range nsm {
		if v.Singleton == true {
			log.WithField("name", v.Name).Info("Instance is a singleton")
		}
	}
	i.Items = il
}

// Expired removes the images older than the ExpiredBefore time.
func (i *JanitorMetadata) Expired() {
	inl := []*compute.Instance{}

	for _, in := range i.Items {
		stamp, err := utils.ParseCreationTimestamp(in.CreationTimestamp)
		if err != nil {
			log.WithFields(log.Fields{
				"reason":             err,
				"creation_timestamp": in.CreationTimestamp,
				"image":              in.Name,
			}).Fatal("Failed to parse timestamp")
		}

		if stamp.Before(i.ExpiredBefore) {
			log.WithFields(log.Fields{
				"image":              in.Name,
				"creation_timestamp": in.CreationTimestamp,
				"expired_by":         i.ExpiredBefore,
				"status":             "expired",
			}).Info("Instance is older than expired_by date")
		} else {
			log.WithFields(log.Fields{
				"image":              in.Name,
				"creation_timestamp": in.CreationTimestamp,
				"expired_by":         i.ExpiredBefore,
				"status":             "fresh",
			}).Info("Instance is newer than expired_by date")
			inl = append(inl, in)
		}
	}
}

// Delete generates a list of DeleteMetadata structs that can be used to
// issue deletes for the list of instances they wrap.
func (i JanitorMetadata) Delete(d chan<- delete.ResourceDeleter) {
	for _, in := range i.Items {
		log.WithFields(log.Fields{
			"name":    in.Name,
			"project": i.Project,
			"zone":    utils.GetResourceNameFromURL(in.Zone),
		}).Debug("Generating deletion call for instance")
		d <- &DeleteMetadata{
			InstanceJanitorMetadata: InstanceJanitorMetadata{
				Service:  i.Service,
				Project:  i.Project,
				Instance: in,
			},
			Call: i.Service.Instances.Delete(i.Project, utils.GetResourceNameFromURL(in.Zone), in.Name),
		}
	}
	close(d)
}
