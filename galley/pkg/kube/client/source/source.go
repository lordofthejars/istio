//  Copyright 2018 Istio Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package source

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/istio/galley/pkg/change"
	"istio.io/istio/galley/pkg/kube"
	"istio.io/istio/galley/pkg/kube/client"
	"istio.io/istio/galley/pkg/kube/convert"
	"istio.io/istio/galley/pkg/kube/types"
	"istio.io/istio/galley/pkg/model/provider"
	"istio.io/istio/galley/pkg/model/resource"
	"istio.io/istio/pkg/log"
)

// source is an implementation of provider.Interface.
type source struct {
	k  kube.Kube
	ch chan provider.Event

	scAccessor *client.Accessor
}

var _ provider.Interface = &source{}

// New returns a Kubernetes implementation of provider.Interface.
func New(k kube.Kube, resyncPeriod time.Duration) (provider.Interface, error) {
	s := &source{
		k: k,
	}

	scAccessor, err := client.NewAccessor(k, resyncPeriod, types.ProducerService, s.process)

	if err != nil {
		return nil, err
	}
	s.scAccessor = scAccessor

	return s, nil
}

// Start implements provider.Interface
func (s *source) Start() (chan provider.Event, error) {
	s.ch = make(chan provider.Event, 1024)

	s.scAccessor.Start()

	return s.ch, nil
}

// Stop implements provider.Interface
func (s *source) Stop() {
	s.scAccessor.Stop()
	s.ch = nil
}

// Get implements provider.Interface
func (s *source) Get(id resource.Key) (resource.Entry, error) {
	parts := strings.Split(id.FullName, "/")
	ns := parts[0]
	name := parts[1]
	u, err := s.scAccessor.Client.Resource(types.ProducerService.APIResource(), ns).Get(name, metav1.GetOptions{})
	if err != nil {
		return resource.Entry{}, err
	}

	item, err := convert.ToProto(types.ProducerService, u)
	if err != nil {
		return resource.Entry{}, err
	}

	rid := resource.VersionedKey{
		Key: resource.Key{
			Kind:     resource.ProducerServiceKind,
			FullName: id.FullName,
		},
		Version: resource.Version(u.GetResourceVersion()),
	}

	return resource.Entry{
		ID:   rid,
		Item: item,
	}, nil
}

func (s *source) process(c *change.Info) {
	var kind provider.EventKind
	switch c.Type {
	case change.Add:
		kind = provider.Added
	case change.Update:
		kind = provider.Updated
	case change.Delete:
		kind = provider.Deleted
	case change.FullSync:
		kind = provider.FullSync
	default:
		log.Errorf("Unknown change kind: %v", c.Type)
	}

	rid := resource.VersionedKey{
		Key: resource.Key{
			Kind:     resource.Kind(types.ProducerService.Kind),
			FullName: c.Name,
		},
		Version: resource.Version(c.Version),
	}

	e := provider.Event{
		ID:   rid,
		Kind: kind,
	}

	log.Debugf("Dispatching source event: %v", e)
	s.ch <- e
}
