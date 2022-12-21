/*
Copyright 2023 The KubeVirt Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakePodMonitors implements PodMonitorInterface
type FakePodMonitors struct {
	Fake *FakeMonitoringV1
	ns   string
}

var podmonitorsResource = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "podmonitors"}

var podmonitorsKind = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PodMonitor"}

// Get takes name of the podMonitor, and returns the corresponding podMonitor object, and an error if there is any.
func (c *FakePodMonitors) Get(ctx context.Context, name string, options v1.GetOptions) (result *monitoringv1.PodMonitor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(podmonitorsResource, c.ns, name), &monitoringv1.PodMonitor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*monitoringv1.PodMonitor), err
}

// List takes label and field selectors, and returns the list of PodMonitors that match those selectors.
func (c *FakePodMonitors) List(ctx context.Context, opts v1.ListOptions) (result *monitoringv1.PodMonitorList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(podmonitorsResource, podmonitorsKind, c.ns, opts), &monitoringv1.PodMonitorList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &monitoringv1.PodMonitorList{ListMeta: obj.(*monitoringv1.PodMonitorList).ListMeta}
	for _, item := range obj.(*monitoringv1.PodMonitorList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested podMonitors.
func (c *FakePodMonitors) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(podmonitorsResource, c.ns, opts))

}

// Create takes the representation of a podMonitor and creates it.  Returns the server's representation of the podMonitor, and an error, if there is any.
func (c *FakePodMonitors) Create(ctx context.Context, podMonitor *monitoringv1.PodMonitor, opts v1.CreateOptions) (result *monitoringv1.PodMonitor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(podmonitorsResource, c.ns, podMonitor), &monitoringv1.PodMonitor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*monitoringv1.PodMonitor), err
}

// Update takes the representation of a podMonitor and updates it. Returns the server's representation of the podMonitor, and an error, if there is any.
func (c *FakePodMonitors) Update(ctx context.Context, podMonitor *monitoringv1.PodMonitor, opts v1.UpdateOptions) (result *monitoringv1.PodMonitor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(podmonitorsResource, c.ns, podMonitor), &monitoringv1.PodMonitor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*monitoringv1.PodMonitor), err
}

// Delete takes name of the podMonitor and deletes it. Returns an error if one occurs.
func (c *FakePodMonitors) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(podmonitorsResource, c.ns, name), &monitoringv1.PodMonitor{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakePodMonitors) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(podmonitorsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &monitoringv1.PodMonitorList{})
	return err
}

// Patch applies the patch and returns the patched podMonitor.
func (c *FakePodMonitors) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *monitoringv1.PodMonitor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(podmonitorsResource, c.ns, name, pt, data, subresources...), &monitoringv1.PodMonitor{})

	if obj == nil {
		return nil, err
	}
	return obj.(*monitoringv1.PodMonitor), err
}
