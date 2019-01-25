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

package utils

import (
	"strings"
	"time"
)

// GetTooOldTime takes in a number of seconds and returns a time.Time that is
// that number of seconds from the current time.
func GetTooOldTime(i int64) time.Time {
	return time.Unix(time.Now().Unix()-i, 0)
}

// ParseCreationTimestamp is used to parse the creationTimestamp fields
// returned by the GCE API.
func ParseCreationTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// GetResourceBasename returns the basename of an instance. If a resource named
// `Image-Base-Name-UniqueIdentifier` is added with the delimiter of `-`, only
// `Image-Base-Name` will be returned.
func GetResourceBasename(a string, delimiter string) string {
	aSplit := strings.Split(a, delimiter)
	return strings.Join(aSplit[:len(aSplit)-1], delimiter)
}

// GetResourceNameFrmoURL takes in a resource's self link and returns the
// resource name.
func GetResourceNameFromURL(z string) string {
	splitResource := strings.Split(z, "/")
	return splitResource[len(splitResource)-1]
}
