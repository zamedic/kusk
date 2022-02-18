package options

type IngressOptions struct {
	// The ingress class to use.
	Class string `json:"class,omitempty" yaml:"class,omitempty"`
}

func (i IngressOptions) Validate() error {
	return nil
}
