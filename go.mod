go 1.24.0

module github.com/L-F-Z/cri-t

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/L-F-Z/TaskC v1.0.0
	github.com/Microsoft/go-winio v0.6.2
	github.com/blang/semver/v4 v4.0.0
	github.com/checkpoint-restore/checkpointctl v1.3.0
	github.com/checkpoint-restore/go-criu/v7 v7.2.0
	github.com/containerd/cgroups v1.1.0
	github.com/containerd/containerd v1.7.24
	github.com/containerd/containerd/api v1.8.0
	github.com/containerd/fifo v1.1.0
	github.com/containerd/nri v0.9.0
	github.com/containerd/otelttrpc v0.0.0-20240305015340-ea5083fda723
	github.com/containerd/ttrpc v1.2.6-0.20240827082320-b5cd6e4b3287
	github.com/containerd/typeurl v1.0.3-0.20220422153119-7f6e6d160d67
	github.com/containernetworking/cni v1.2.3
	github.com/containernetworking/plugins v1.6.2
	github.com/containers/common v0.61.0
	github.com/containers/conmon v2.0.20+incompatible
	github.com/containers/conmon-rs v0.6.6
	github.com/containers/kubensmnt v1.2.0
	github.com/containers/storage v1.57.2
	github.com/coreos/go-systemd/v22 v22.5.1-0.20231103132048-7d375ecc2b09
	github.com/creack/pty v1.1.24
	github.com/cri-o/ocicni v0.4.3
	github.com/cyphar/filepath-securejoin v0.4.1
	github.com/docker/distribution v2.8.3+incompatible
	github.com/docker/go-units v0.5.0
	github.com/fsnotify/fsnotify v1.8.0
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-logr/logr v1.4.2
	github.com/godbus/dbus/v5 v5.1.1-0.20230522191255-76236955d466
	github.com/google/go-cmp v0.6.0
	github.com/google/renameio v1.0.1
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/intel/goresctrl v0.8.0
	github.com/json-iterator/go v1.1.12
	github.com/kata-containers/kata-containers/src/runtime v0.0.0-20240208092920-b99f57452225
	github.com/moby/sys/user v0.3.0
	github.com/moby/sys/userns v0.1.0
	github.com/onsi/ginkgo/v2 v2.22.0
	github.com/onsi/gomega v1.36.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opencontainers/runc v1.2.6
	github.com/opencontainers/runtime-spec v1.2.1
	github.com/opencontainers/runtime-tools v0.9.1-0.20241001195557-6c9570a1678f
	github.com/opencontainers/selinux v1.12.0
	github.com/prometheus/client_golang v1.20.5
	github.com/seccomp/libseccomp-golang v0.10.0
	github.com/sirupsen/logrus v1.9.3
	github.com/soheilhy/cmux v0.1.5
	github.com/stretchr/testify v1.10.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/uptrace/opentelemetry-go-extra/otellogrus v0.3.2
	github.com/urfave/cli/v2 v2.27.5
	github.com/vishvananda/netlink v1.3.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.57.0
	go.opentelemetry.io/otel v1.32.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.32.0
	go.opentelemetry.io/otel/sdk v1.32.0
	go.opentelemetry.io/otel/trace v1.32.0
	go.uber.org/mock v0.5.0
	golang.org/x/sync v0.12.0
	golang.org/x/sys v0.31.0
	google.golang.org/grpc v1.69.4
	google.golang.org/protobuf v1.36.6
	k8s.io/api v0.31.4
	k8s.io/apimachinery v0.31.4
	k8s.io/client-go v0.31.4
	k8s.io/cri-api v0.31.4
	k8s.io/cri-client v0.31.4
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubelet v0.31.4
	k8s.io/utils v0.0.0-20240711033017-18e509b52bc8
	sigs.k8s.io/release-sdk v0.12.1
	sigs.k8s.io/release-utils v0.8.5
	sigs.k8s.io/yaml v1.4.0
	tags.cncf.io/container-device-interface v0.8.1
)

replace github.com/L-F-Z/TaskC => ../TaskC

require (
	capnproto.org/go/capnp/v3 v3.0.1-alpha.2 // indirect
	dario.cat/mergo v1.0.1 // indirect
	github.com/ProtonMail/go-crypto v1.1.3 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cavaliergopher/grab/v3 v3.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	github.com/containerd/console v1.0.4 // indirect
	github.com/containerd/errdefs v0.3.0 // indirect
	github.com/containerd/go-runc v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.5.0 // indirect
	github.com/go-git/go-git/v5 v5.12.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.23.0 // indirect
	github.com/go-openapi/errors v0.22.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/loads v0.22.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/go-openapi/strfmt v0.23.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-openapi/validate v0.24.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.9-0.20230804172637-c7be7c783f49 // indirect
	github.com/google/go-github/v60 v60.0.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.23.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/knqyf263/go-plugin v0.8.1-0.20241122101054-d8d42059d8f1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-shellwords v1.0.12 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.4.0 // indirect
	github.com/moby/sys/capability v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.57.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/tchap/go-patricia/v2 v2.3.2 // indirect
	github.com/tetratelabs/wazero v1.8.2 // indirect
	github.com/ulikunitz/xz v0.5.12 // indirect
	github.com/uptrace/opentelemetry-go-extra/otelutil v0.3.2 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.54.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.32.0 // indirect
	go.opentelemetry.io/otel/log v0.6.0 // indirect
	go.opentelemetry.io/otel/metric v1.32.0 // indirect
	go.opentelemetry.io/proto/otlp v1.3.1 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/exp v0.0.0-20241217172543-b2144cdd0a67 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/oauth2 v0.25.0 // indirect
	golang.org/x/term v0.30.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/time v0.9.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20241209162323-e6fa225c2576 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250102185135-69823020774d // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiserver v0.31.4 // indirect
	k8s.io/component-base v0.31.4 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	tags.cncf.io/container-device-interface/specs-go v0.8.0 // indirect
)
