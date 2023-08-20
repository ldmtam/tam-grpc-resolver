package tam_grpc_resolver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"google.golang.org/grpc/resolver"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// schemeName for the urls
	// All target URLs like 'tam://{app_name}.{namespace}:{port}' will be resolved by this resolver
	// example: "tam://calculator_server.default:8080"
	schemeName = "tam"

	defaultPort = "8080"
)

var (
	errMissingAddr = errors.New("tam resolver: missing address")

	// Addresses ending with a colon that is supposed to be the separator
	// between host and port is not allowed.  E.g. "::" is a valid address as
	// it is an IPv6 address (host only) and "[::]:" is invalid as it ends with
	// a colon as the host and port separator
	errEndsWithColon = errors.New("tam resolver: missing port after port-separator colon")

	errWrongHostFormat = errors.New("tam resolver: wrong host format")
)

// builder implements resolver.Builder and use for constructing all k8s resolvers
type builder struct{}

func (b *builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	host, port, err := parseTarget(target.Endpoint(), defaultPort)
	if err != nil {
		return nil, err
	}

	arr := strings.Split(host, ".")
	if len(arr) != 2 {
		return nil, errWrongHostFormat
	}

	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("get cluster config failed: %v", err)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("initialize clientset failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.TODO())

	t := &tamResolver{
		appName:   arr[0],
		namespace: arr[1],
		port:      port,
		label:     fmt.Sprintf("app=%v", arr[0]),

		ctx:    ctx,
		cancel: cancel,

		cc: cc,

		clientset: clientset,
	}

	t.wg.Add(1)
	go t.watcher()
	return t, nil
}

// Scheme returns the scheme supported by this resolver.
// Scheme is defined at https://github.com/grpc/grpc/blob/master/doc/naming.md.
func (b *builder) Scheme() string {
	return schemeName
}

// parseTarget takes the user input target string and default port, returns
// formatted host and port info. If target doesn't specify a port, set the port
// to be the defaultPort. If target is in IPv6 format and host-name is enclosed
// in square brackets, brackets are stripped when setting the host.
// examples:
// target: "www.google.com" defaultPort: "443" returns host: "www.google.com", port: "443"
// target: "ipv4-host:80" defaultPort: "443" returns host: "ipv4-host", port: "80"
// target: "[ipv6-host]" defaultPort: "443" returns host: "ipv6-host", port: "443"
// target: ":80" defaultPort: "443" returns host: "localhost", port: "80"
func parseTarget(target, defaultPort string) (host, port string, err error) {
	if target == "" {
		return "", "", errMissingAddr
	}
	if ip := net.ParseIP(target); ip != nil {
		// target is an IPv4 or IPv6(without brackets) address
		return target, defaultPort, nil
	}
	if host, port, err = net.SplitHostPort(target); err == nil {
		if port == "" {
			// If the port field is empty (target ends with colon), e.g. "[::1]:",
			// this is an error.
			return "", "", errEndsWithColon
		}
		// target has port, i.e ipv4-host:port, [ipv6-host]:port, host-name:port
		if host == "" {
			// Keep consistent with net.Dial(): If the host is empty, as in ":80",
			// the local system is assumed.
			host = "localhost"
		}
		return host, port, nil
	}
	if host, port, err = net.SplitHostPort(target + ":" + defaultPort); err == nil {
		// target doesn't have port
		return host, port, nil
	}
	return "", "", fmt.Errorf("invalid target address %v, error info: %v", target, err)
}
