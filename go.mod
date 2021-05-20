module github.com/jenkins-x-plugins/jx-verify

go 1.15

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/genkiroid/cert v0.0.0-20191007122723-897560fbbe50
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/jenkins-x/jx-api/v4 v4.0.33
	github.com/jenkins-x/jx-helpers/v3 v3.0.114
	github.com/jenkins-x/jx-kube-client/v3 v3.0.2
	github.com/jenkins-x/jx-logging/v3 v3.0.6
	github.com/pkg/errors v0.9.1
	github.com/sergi/go-diff v1.1.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	k8s.io/api v0.20.7
	k8s.io/apimachinery v0.20.7
	k8s.io/client-go v11.0.0+incompatible
	sigs.k8s.io/structured-merge-diff/v4 v4.0.3 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.20.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.2
	k8s.io/client-go => k8s.io/client-go v0.20.2
)
