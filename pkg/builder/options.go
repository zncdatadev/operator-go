/*
Copyright 2024 ZNCDataDev.

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

package builder

type Optioner interface {
	Apply(opts *Options)
}

type Options struct {
	ClusterName   string
	RoleName      string
	RoleGroupName string
	Labels        map[string]string
	Annotations   map[string]string
}

func (o *Options) Apply(opts *Options) {
	if opts == nil {
		return
	}
	if opts.ClusterName != "" {
		o.ClusterName = opts.ClusterName
	}
	if opts.RoleName != "" {
		o.RoleName = opts.RoleName
	}
	if opts.RoleGroupName != "" {
		o.RoleGroupName = opts.RoleGroupName
	}
	if opts.Labels != nil {
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		for k, v := range opts.Labels {
			o.Labels[k] = v
		}
	}
	if opts.Annotations != nil {
		if o.Annotations == nil {
			o.Annotations = make(map[string]string)
		}
		for k, v := range opts.Annotations {
			o.Annotations[k] = v
		}
	}
}

type Option func(*Options)
