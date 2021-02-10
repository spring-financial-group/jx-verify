package job

import (
	"context"
	"fmt"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jobs"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/podlogs"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/pods"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	logger "github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Options contains the command line arguments for this command
type Options struct {
	options.BaseOptions

	Namespace     string
	JobSelector   string
	ContainerName string
	Duration      time.Duration
	PollPeriod    time.Duration
	NoTail        bool
	ErrOut        io.Writer
	Out           io.Writer
	KubeClient    kubernetes.Interface
	Input         input.Interface
	timeEnd       time.Time
	podStatusMap  map[string]string
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Verifies that the job(s) with the given label succeeds and tails the log as it executes

`)

	cmdExample = templates.Examples(`
		# verify the BDD job succeeds
		jx verify job -l app=jx-bdd
`)
)

// NewCmdVerifyJob creates the new command
func NewCmdVerifyJob() (*cobra.Command, *Options) {
	options := &Options{}
	command := &cobra.Command{
		Use:     "job",
		Short:   "Verifies that the job(s) with the given label succeeds and tails the log as it executes",
		Aliases: []string{"logs"},
		Long:    cmdLong,
		Example: cmdExample,
		Run: func(command *cobra.Command, args []string) {
			err := options.Run()
			helper.CheckErr(err)
		},
	}
	command.Flags().StringVarP(&options.Namespace, "namespace", "n", "", "the namespace where the jobs run. If not specified it will look in: jx-git-operator and jx")
	command.Flags().StringVarP(&options.JobSelector, "selector", "l", "", "the selector of the job pods")
	command.Flags().StringVarP(&options.ContainerName, "container", "c", "", "the name of the container in the job to log")
	command.Flags().DurationVarP(&options.Duration, "duration", "d", time.Minute*60, "how long to wait for a Job to be active and a Pod to be ready")
	command.Flags().DurationVarP(&options.PollPeriod, "poll", "", time.Second*1, "duration between polls for an active Job or Pod")

	options.BaseOptions.AddBaseFlags(command)

	return command, options
}

func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return err
	}

	client := o.KubeClient
	selector := o.JobSelector
	ns := o.Namespace

	jobs, err := GetSortedJobs(client, ns, selector)
	if err != nil {
		return errors.Wrapf(err, "failed to get jobs")
	}

	return o.pickJobToLog(client, ns, selector, jobs)
}

func (o *Options) viewActiveJobLog(client kubernetes.Interface, ns string, selector string, job *batchv1.Job) error {
	o.timeEnd = time.Now().Add(o.Duration)

	var foundPods []string
	for {
		complete, pod, err := o.waitForJobCompleteOrPodRunning(client, ns, selector, job.Name)
		if err != nil {
			return err
		}
		if complete {
			return nil
		}
		if pod == nil {
			return errors.Errorf("No pod found for namespace %s with selector %v", ns, selector)
		}

		if time.Now().After(o.timeEnd) {
			return errors.Errorf("timed out after waiting for duration %s", o.Duration.String())
		}

		// lets verify the container name
		containerName := o.ContainerName
		if containerName == "" {
			containerName = pod.Spec.Containers[0].Name
		}
		err = verifyContainerName(pod, containerName)
		if err != nil {
			return err
		}
		podName := pod.Name
		if stringhelpers.StringArrayIndex(foundPods, podName) < 0 {
			foundPods = append(foundPods, podName)
		}
		logger.Logger().Infof("\ntailing boot Job pod %s\n\n", info(podName))

		err = podlogs.TailLogs(ns, podName, containerName, o.ErrOut, o.Out)
		if err != nil {
			logger.Logger().Warnf("failed to tail log: %s", err.Error())
		}
		pod, err = client.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to get pod %s in namespace %s", podName, ns)
		}
		if pods.IsPodCompleted(pod) {
			if pods.IsPodSucceeded(pod) {
				logger.Logger().Infof("boot Job pod %s has %s", info(podName), info("Succeeded"))
			} else {
				logger.Logger().Infof("boot Job pod %s has %s", info(podName), termcolor.ColorError(string(pod.Status.Phase)))
			}
		} else if pod.DeletionTimestamp != nil {
			logger.Logger().Infof("boot Job pod %s is %s", info(podName), termcolor.ColorWarning("Terminating"))
		}
	}
}

// Validate verifies the settings are correct and we can lazy create any required resources
func (o *Options) Validate() error {
	if o.JobSelector == "" {
		return options.MissingOption("selector")
	}
	if o.NoTail {
		return nil
	}
	if o.ErrOut == nil {
		o.ErrOut = os.Stderr
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}

	var err error
	o.KubeClient, err = kube.LazyCreateKubeClientWithMandatory(o.KubeClient, true)
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes client")
	}
	if o.Namespace == "" {
		o.Namespace, err = kubeclient.CurrentNamespace()
		if err != nil {
			return errors.Wrapf(err, "failed to detect current namespace. Try supply --namespace")
		}
	}
	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
	}
	return nil
}

func (o *Options) waitForLatestJob(client kubernetes.Interface, ns, selector string) (*batchv1.Job, error) {
	for {
		job, err := o.getLatestJob(client, ns, selector)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to ")
		}

		if job != nil {
			if !jobs.IsJobFinished(job) {
				return job, nil
			}
		}

		if time.Now().After(o.timeEnd) {
			return nil, errors.Errorf("timed out after waiting for duration %s", o.Duration.String())
		}
		time.Sleep(o.PollPeriod)
	}
}

func (o *Options) waitForJobCompleteOrPodRunning(client kubernetes.Interface, ns, selector, jobName string) (bool, *corev1.Pod, error) {
	if o.podStatusMap == nil {
		o.podStatusMap = map[string]string{}
	}

	for {
		complete, job, err := o.checkIfJobComplete(client, ns, jobName)
		if err != nil {
			return false, nil, errors.Wrapf(err, "failed to check for Job %s complete", jobName)
		}
		if complete {
			if job != nil && !jobs.IsJobSucceeded(job) {
				return true, nil, errors.Errorf("job %s failed", jobName)
			}
			return true, nil, nil
		}

		pod, err := pods.GetReadyPodForSelector(client, ns, selector)
		if err != nil {
			return false, pod, errors.Wrapf(err, "failed to query ready pod in namespace %s with selector %s", ns, selector)
		}
		if pod != nil {
			status := pods.PodStatus(pod)
			if o.podStatusMap[pod.Name] != status && !pods.IsPodCompleted(pod) && pod.DeletionTimestamp == nil {
				logger.Logger().Infof("pod %s has status %s", termcolor.ColorInfo(pod.Name), termcolor.ColorInfo(status))
				o.podStatusMap[pod.Name] = status
			}
			if pod.Status.Phase == v1.PodRunning || pods.IsPodReady(pod) {
				return false, pod, nil
			}
		}

		if time.Now().After(o.timeEnd) {
			return false, nil, errors.Errorf("timed out after waiting for duration %s", o.Duration.String())
		}
		time.Sleep(o.PollPeriod)
	}
}

func (o *Options) getLatestJob(client kubernetes.Interface, ns, selector string) (*batchv1.Job, error) {
	jobList, err := client.BatchV1().Jobs(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to list jobList in namespace %s selector %s", ns, selector)
	}
	if len(jobList.Items) == 0 {
		return nil, nil
	}

	// lets find the newest job...
	latest := jobList.Items[0]
	for i := 1; i < len(jobList.Items); i++ {
		job := jobList.Items[i]
		if job.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = job
		}
	}
	return &latest, nil
}

func (o *Options) checkIfJobComplete(client kubernetes.Interface, ns, name string) (bool, *batchv1.Job, error) {
	job, err := client.BatchV1().Jobs(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if job == nil || err != nil {
		return false, nil, errors.Wrapf(err, "failed to list jobList in namespace %s name %s", ns, name)
	}
	if jobs.IsJobFinished(job) {
		if jobs.IsJobSucceeded(job) {
			logger.Logger().Infof("job %s has %s", info(job.Name), info("Succeeded"))
			return true, job, nil
		}
		logger.Logger().Infof("job %s has %s", info(job.Name), termcolor.ColorError("Failed"))
		return true, job, nil
	}
	logger.Logger().Debugf("job %s is not completed yet", info(job.Name))
	return false, job, nil
}

func (o *Options) pickJobToLog(client kubernetes.Interface, ns string, selector string, jobs []batchv1.Job) error {
	var names []string
	m := map[string]*batchv1.Job{}
	for i := range jobs {
		j := &jobs[i]
		name := toJobName(j, len(jobs)-i)
		m[name] = j
		names = append(names, name)
	}

	name, err := o.Input.PickNameWithDefault(names, "select the Job to view:", "", "select which job you wish to log")
	if err != nil {
		return errors.Wrapf(err, "failed to pick a job name")
	}
	if name == "" {
		return errors.Errorf("no jobs to view. Try add --active to wait for the next job")
	}
	job := m[name]
	if job == nil {
		return errors.Errorf("cannot find Job %s", name)
	}
	return o.viewActiveJobLog(client, ns, selector, job)
}

func toJobName(j *batchv1.Job, number int) string {
	status := JobStatus(j)
	d := time.Now().Sub(j.CreationTimestamp.Time).Round(time.Minute)
	return fmt.Sprintf("#%d started %s %s", number, d.String(), status)
}

func JobStatus(j *batchv1.Job) string {
	if jobs.IsJobSucceeded(j) {
		return "Succeeded"
	}
	if jobs.IsJobFinished(j) {
		return "Failed"
	}
	if j.Status.Active > 0 {
		return "Running"
	}
	return "Pending"
}

func verifyContainerName(pod *corev1.Pod, name string) error {
	var names []string
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return nil
		}
		names = append(names, pod.Spec.Containers[i].Name)
	}
	sort.Strings(names)
	return errors.Errorf("invalid container name %s for pod %s. Available names: %s", name, pod.Name, strings.Join(names, ", "))
}

// GetSortedJobs gets the jobs with an optional commit sha filter
func GetSortedJobs(client kubernetes.Interface, ns string, selector string) ([]batchv1.Job, error) {
	jobList, err := client.BatchV1().Jobs(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to list jobList in namespace %s selector %s", ns, selector)
	}

	answer := jobList.Items
	sort.Slice(answer, func(i, j int) bool {
		j1 := answer[i]
		j2 := answer[j]
		return j2.CreationTimestamp.Before(&j1.CreationTimestamp)
	})
	return answer, nil
}
