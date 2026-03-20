module github.com/abcxyz/github-action-dispatcher

go 1.25.0

require (
	cloud.google.com/go/asset v1.22.1
	cloud.google.com/go/cloudbuild v1.25.0
	cloud.google.com/go/kms v1.26.0
	github.com/abcxyz/pkg v1.5.4
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-redis/redismock/v8 v8.11.5
	github.com/google/go-cmp v0.7.0
	github.com/google/go-github/v69 v69.2.0
	github.com/google/uuid v1.6.0
	github.com/lestrrat-go/jwx/v2 v2.1.6
	github.com/sethvargo/go-envconfig v1.1.1
	github.com/sethvargo/go-gcpkms v0.3.0
	github.com/sethvargo/go-retry v0.3.0
	golang.org/x/oauth2 v0.36.0
	google.golang.org/api v0.272.0
	google.golang.org/protobuf v1.36.11
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/accesscontextmanager v1.9.7 // indirect
	cloud.google.com/go/auth v0.18.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.5.3 // indirect
	cloud.google.com/go/longrunning v0.8.0 // indirect
	cloud.google.com/go/orgpolicy v1.15.1 // indirect
	cloud.google.com/go/osconfig v1.16.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.14 // indirect
	github.com/googleapis/gax-go/v2 v2.19.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.3 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.6 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/posener/complete/v2 v2.1.0 // indirect
	github.com/posener/script v1.2.0 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.42.0 // indirect
	go.opentelemetry.io/otel/metric v1.42.0 // indirect
	go.opentelemetry.io/otel/trace v1.42.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto v0.0.0-20260316180232-0b37fe3546d5 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260319201613-d00831a3d3e7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260319201613-d00831a3d3e7 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace cloud.google.com/go/cloudbuild => ./third_party/cloudbuild_trusted_pools
