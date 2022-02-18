package ingress

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	"github.com/kubeshop/kusk/generators"
	"github.com/kubeshop/kusk/options"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
	"sort"
	"strings"
)

const (
	ingressAPIVersion = "networking.k8s.io/v1"
	ingressKind       = "Ingress"
)

var (
	openApiPathVariableRegex = regexp.MustCompile(`{[A-z]+}`)
)

func init() {
	generators.Registry["ingress"] = &Generator{}
}

type Generator struct {
}

func (g Generator) Cmd() string {
	return "ingress"
}

func (g Generator) Flags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("ingress", pflag.ExitOnError)

	fs.String(
		"path.base",
		"/",
		"a base path for Service endpoints",
	)

	fs.Bool(
		"path.split",
		false,
		"force Kusk to generate a separate Ingress for each operation",
	)

	fs.String(
		"ingress.class",
		"",
		"if omitted, a default Ingress class should be defined",
	)

	return fs

}

func (g Generator) ShortDescription() string {
	return "generates a generic ingress definition for your service"
}

func (g Generator) LongDescription() string {
	return g.ShortDescription()
}

func (g Generator) Generate(opts *options.Options, spec *openapi3.T) (string, error) {
	if err := opts.FillDefaultsAndValidate(); err != nil {
		return "", fmt.Errorf("failed to validate opts: %w", err)
	}

	if g.shouldSplit(opts, spec) {

		return g.splitPath(opts, spec)

	}

	ingress := g.newIngressResource(
		fmt.Sprintf("%s-ingress", opts.Service.Name),
		opts.Namespace,
		opts.Path.Base,
		v1.PathTypePrefix,
		&opts.Service,
		opts.Host,
		opts.Ingress.Class,
	)

	b, err := yaml.Marshal(ingress)

	return string(b), err
}

func (g Generator) generateServiceProfileSpec(o *options.Options, spec *openapi3.T) v1.IngressSpec {
	return v1.IngressSpec{
		IngressClassName: &o.Ingress.Class,
	}
}

func (g Generator) splitPath(opts *options.Options, spec *openapi3.T) (string, error) {
	ingresses := make([]v1.Ingress, 0)

	for path := range spec.Paths {
		if opts.IsPathDisabled(path) {
			continue
		}
		name := fmt.Sprintf("%s-%s", opts.Service.Name, ingressResourceNameFromPath(path))

		var pathField string
		if openApiPathVariableRegex.MatchString(path) {
			pathField = opts.Path.Base + string(openApiPathVariableRegex.ReplaceAll([]byte(path), []byte("([A-z0-9]+)")))

		} else if path == "/" {
			pathField = opts.Path.Base + "$"
		} else {
			pathField = opts.Path.Base + path

		}

		// Replace // with /
		pathField = strings.ReplaceAll(pathField, "//", "/")

		ingress := g.newIngressResource(
			name,
			opts.Namespace,
			pathField,
			v1.PathTypeExact,
			&opts.Service,
			opts.Host,
			opts.Ingress.Class,
		)

		ingresses = append(ingresses, ingress)
	}
	sort.Slice(ingresses, func(i, j int) bool {
		return ingresses[i].Name < ingresses[j].Name
	})

	return buildOutput(ingresses)
}

func (g *Generator) newIngressResource(
	name,
	namespace,
	path string,
	pathType v1.PathType,
	serviceOpts *options.ServiceOptions,
	host string,
	ingressClass string,
) v1.Ingress {
	return v1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ingressAPIVersion,
			Kind:       ingressKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []v1.IngressRule{
				{
					Host: host,
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									PathType: &pathType,
									Path:     path,
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: serviceOpts.Name,
											Port: v1.ServiceBackendPort{
												Number: serviceOpts.Port,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (g Generator) shouldSplit(opts *options.Options, spec *openapi3.T) bool {
	if opts.Path.Split {
		return true
	}

	for path, _ := range spec.Paths {
		// a path is disabled
		if opts.IsPathDisabled(path) {
			return true
		}
	}

	return false

}

func ingressResourceNameFromPath(path string) string {
	if len(path) == 0 || path == "/" {
		return "root"
	}

	var b strings.Builder
	for _, pathItem := range strings.Split(path, "/") {
		if pathItem == "" {
			continue
		}

		// remove openapi path variable curly braces from path item
		strippedPathItem := strings.ReplaceAll(strings.ReplaceAll(pathItem, "{", ""), "}", "")
		fmt.Fprintf(&b, "%s-", strippedPathItem)
	}

	// remove trailing - character
	return strings.ToLower(strings.TrimSuffix(b.String(), "-"))
}

func buildOutput(ingresses []v1.Ingress) (string, error) {
	var builder strings.Builder

	for _, ingress := range ingresses {
		builder.WriteString("---\n") // indicate start of YAML resource
		b, err := yaml.Marshal(ingress)
		if err != nil {
			return "", fmt.Errorf("unable to marshal ingress resource: %+v: %s", ingress, err.Error())
		}
		builder.WriteString(string(b))
	}

	return builder.String(), nil
}
