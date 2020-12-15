/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// See k8s.io/apimachinery/pkg/util/rand/rand.go
package state

const (
	// We omit vowels from the set of available characters to reduce the chances
	// of "bad words" being formed.
	alphanums = "bcdfghjklmnpqrstvwxz2456789"
)

// SafeEncodeString encodes s using the same characters as rand.String. This reduces the chances of bad words and
// ensures that strings generated from hash functions appear consistent throughout the API.
func SafeEncodeString(s string) string {
	r := make([]byte, len(s))
	for i, b := range []rune(s) {
		r[i] = alphanums[(int(b) % len(alphanums))]
	}
	return string(r)
}
