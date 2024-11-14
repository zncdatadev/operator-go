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
