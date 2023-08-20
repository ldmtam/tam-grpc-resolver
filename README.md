# TAM-GRPC-RESOLVER
gRPC name resolver by using kubernetes API that watches namespace and label to resolve new connection in realtime.

### Usage

```
import _ "github.com/ldmtam/tam-grpc-resolver" // don't forget to import the resolver

conn, err := grpc.DialContext(
	context.TODO(),
	"tam:///calculator-server.default:8080",
	grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	grpc.WithTransportCredentials(insecure.NewCredentials()),
)
if err != nil {
	log.Fatalf("dial to calculator server failed: %v", err)
}
defer conn.Close()
```

An URL should have the following format
```
tam:///{app_name}.{namespace}
tam:///{app_name}.{namespace}:{port}
```

Example Project
```
https://github.com/ldmtam/calculator-service
```