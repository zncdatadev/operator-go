//go:build !ignore_autogenerated

/*
Copyright 2024 zncdatadev.

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import ()

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CACert) DeepCopyInto(out *CACert) {
	*out = *in
	if in.WebPki != nil {
		in, out := &in.WebPki, &out.WebPki
		*out = new(WebPki)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CACert.
func (in *CACert) DeepCopy() *CACert {
	if in == nil {
		return nil
	}
	out := new(CACert)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CPUResource) DeepCopyInto(out *CPUResource) {
	*out = *in
	out.Max = in.Max.DeepCopy()
	out.Min = in.Min.DeepCopy()
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CPUResource.
func (in *CPUResource) DeepCopy() *CPUResource {
	if in == nil {
		return nil
	}
	out := new(CPUResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterOperationSpec) DeepCopyInto(out *ClusterOperationSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterOperationSpec.
func (in *ClusterOperationSpec) DeepCopy() *ClusterOperationSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterOperationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Credentials) DeepCopyInto(out *Credentials) {
	*out = *in
	if in.Scope != nil {
		in, out := &in.Scope, &out.Scope
		*out = new(CredentialsScope)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Credentials.
func (in *Credentials) DeepCopy() *Credentials {
	if in == nil {
		return nil
	}
	out := new(Credentials)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CredentialsScope) DeepCopyInto(out *CredentialsScope) {
	*out = *in
	if in.Services != nil {
		in, out := &in.Services, &out.Services
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CredentialsScope.
func (in *CredentialsScope) DeepCopy() *CredentialsScope {
	if in == nil {
		return nil
	}
	out := new(CredentialsScope)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogLevelSpec) DeepCopyInto(out *LogLevelSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogLevelSpec.
func (in *LogLevelSpec) DeepCopy() *LogLevelSpec {
	if in == nil {
		return nil
	}
	out := new(LogLevelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoggingConfigSpec) DeepCopyInto(out *LoggingConfigSpec) {
	*out = *in
	if in.Loggers != nil {
		in, out := &in.Loggers, &out.Loggers
		*out = make(map[string]*LogLevelSpec, len(*in))
		for key, val := range *in {
			var outVal *LogLevelSpec
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = new(LogLevelSpec)
				**out = **in
			}
			(*out)[key] = outVal
		}
	}
	if in.Console != nil {
		in, out := &in.Console, &out.Console
		*out = new(LogLevelSpec)
		**out = **in
	}
	if in.File != nil {
		in, out := &in.File, &out.File
		*out = new(LogLevelSpec)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoggingConfigSpec.
func (in *LoggingConfigSpec) DeepCopy() *LoggingConfigSpec {
	if in == nil {
		return nil
	}
	out := new(LoggingConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoggingSpec) DeepCopyInto(out *LoggingSpec) {
	*out = *in
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make(map[string]LoggingConfigSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.EnableVectorAgent != nil {
		in, out := &in.EnableVectorAgent, &out.EnableVectorAgent
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoggingSpec.
func (in *LoggingSpec) DeepCopy() *LoggingSpec {
	if in == nil {
		return nil
	}
	out := new(LoggingSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MemoryResource) DeepCopyInto(out *MemoryResource) {
	*out = *in
	out.Limit = in.Limit.DeepCopy()
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MemoryResource.
func (in *MemoryResource) DeepCopy() *MemoryResource {
	if in == nil {
		return nil
	}
	out := new(MemoryResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MutualVerification) DeepCopyInto(out *MutualVerification) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MutualVerification.
func (in *MutualVerification) DeepCopy() *MutualVerification {
	if in == nil {
		return nil
	}
	out := new(MutualVerification)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NoneVerification) DeepCopyInto(out *NoneVerification) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NoneVerification.
func (in *NoneVerification) DeepCopy() *NoneVerification {
	if in == nil {
		return nil
	}
	out := new(NoneVerification)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodDisruptionBudgetSpec) DeepCopyInto(out *PodDisruptionBudgetSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodDisruptionBudgetSpec.
func (in *PodDisruptionBudgetSpec) DeepCopy() *PodDisruptionBudgetSpec {
	if in == nil {
		return nil
	}
	out := new(PodDisruptionBudgetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourcesSpec) DeepCopyInto(out *ResourcesSpec) {
	*out = *in
	if in.CPU != nil {
		in, out := &in.CPU, &out.CPU
		*out = new(CPUResource)
		(*in).DeepCopyInto(*out)
	}
	if in.Memory != nil {
		in, out := &in.Memory, &out.Memory
		*out = new(MemoryResource)
		(*in).DeepCopyInto(*out)
	}
	if in.Storage != nil {
		in, out := &in.Storage, &out.Storage
		*out = new(StorageResource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourcesSpec.
func (in *ResourcesSpec) DeepCopy() *ResourcesSpec {
	if in == nil {
		return nil
	}
	out := new(ResourcesSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerVerification) DeepCopyInto(out *ServerVerification) {
	*out = *in
	if in.CACert != nil {
		in, out := &in.CACert, &out.CACert
		*out = new(CACert)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerVerification.
func (in *ServerVerification) DeepCopy() *ServerVerification {
	if in == nil {
		return nil
	}
	out := new(ServerVerification)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageResource) DeepCopyInto(out *StorageResource) {
	*out = *in
	out.Capacity = in.Capacity.DeepCopy()
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageResource.
func (in *StorageResource) DeepCopy() *StorageResource {
	if in == nil {
		return nil
	}
	out := new(StorageResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageResourceSpec) DeepCopyInto(out *StorageResourceSpec) {
	*out = *in
	if in.Data != nil {
		in, out := &in.Data, &out.Data
		*out = new(StorageResource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageResourceSpec.
func (in *StorageResourceSpec) DeepCopy() *StorageResourceSpec {
	if in == nil {
		return nil
	}
	out := new(StorageResourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSVerificationSpec) DeepCopyInto(out *TLSVerificationSpec) {
	*out = *in
	if in.None != nil {
		in, out := &in.None, &out.None
		*out = new(NoneVerification)
		**out = **in
	}
	if in.Server != nil {
		in, out := &in.Server, &out.Server
		*out = new(ServerVerification)
		(*in).DeepCopyInto(*out)
	}
	if in.Mutual != nil {
		in, out := &in.Mutual, &out.Mutual
		*out = new(MutualVerification)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSVerificationSpec.
func (in *TLSVerificationSpec) DeepCopy() *TLSVerificationSpec {
	if in == nil {
		return nil
	}
	out := new(TLSVerificationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WebPki) DeepCopyInto(out *WebPki) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WebPki.
func (in *WebPki) DeepCopy() *WebPki {
	if in == nil {
		return nil
	}
	out := new(WebPki)
	in.DeepCopyInto(out)
	return out
}
