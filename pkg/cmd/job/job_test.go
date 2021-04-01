package job_test

import (
	"github.com/jenkins-x-plugins/jx-verify/pkg/cmd/job"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestVerifyJob(t *testing.T) {
	ns := "jx"
	_, o := job.NewCmdVerifyJob()
	kubeClient := fake.NewSimpleClientset()
	o.KubeClient = kubeClient
	o.Namespace = ns

	// TODO
}
