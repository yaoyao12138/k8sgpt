package instana

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/k8sgpt-ai/k8sgpt/pkg/common"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type SelHostedInstallAnalyzer struct {
	instana *Instana
}

func (s *SelHostedInstallAnalyzer) Analyze(analyzer common.Analyzer) ([]common.Result, error) {
	var results []common.Result

	namespaces, err := s.getNamespacesWithKeywords("kafka", "elasticsearch", "postgres", "clickhouse", "cassandra", "beeinstana", "instana-operator")
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaces {
		fmt.Print(color.YellowString("Namespace: %s\n", ns))

		results = append(results, s.checkStatefulSets(ns)...)
		results = append(results, s.checkDeployments(ns)...)
		results = append(results, s.checkPods(ns)...)
	}

	coreNamespace, err := s.getNamespacesWithKeywords("core")
	if err != nil {
		return nil, err
	}
	if len(coreNamespace) > 0 {
		coreResults, err := s.checkCustomResource("core", "Core", coreNamespace[0])
		if err != nil {
			return nil, err
		}
		if len(coreResults) > 0 && len(coreResults[0].Error) > 0 {
			logs, err := s.getInstanaOperatorLogs("instana-operator")
			if err != nil {
				return nil, err
			} else {
				coreResults[0].Error = append(coreResults[0].Error, common.Failure{
					Text:      fmt.Sprintf("Core is not ready\nExternal instana-operator Log:\n%s", logs),
					Sensitive: []common.Sensitive{},
				})
			}
		}
		results = append(results, coreResults...)
	}

	unitNamespace, err := s.getNamespacesWithKeywords("unit")
	if err != nil {
		return nil, err
	}
	if len(unitNamespace) > 0 {
		unitResults, err := s.checkCustomResource("unit", "Unit", unitNamespace[0])
		if err != nil {
			return nil, err
		}
		results = append(results, unitResults...)
	}

	return results, nil
}

func (s *SelHostedInstallAnalyzer) getNamespacesWithKeywords(keywords ...string) ([]string, error) {
	var matchedNamespaces []string
	namespaceList := &v1.NamespaceList{}

	err := s.instana.client.List(context.Background(), namespaceList)
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaceList.Items {
		for _, keyword := range keywords {
			if strings.Contains(ns.Name, keyword) {
				matchedNamespaces = append(matchedNamespaces, ns.Name)
				break
			}
		}
	}

	return matchedNamespaces, nil
}

func (s *SelHostedInstallAnalyzer) checkStatefulSets(namespace string) []common.Result {
	var results []common.Result
	stsList := &unstructured.UnstructuredList{}
	stsList.SetKind("StatefulSet")
	stsList.SetAPIVersion("apps/v1")
	if err := s.instana.client.List(context.Background(), stsList, client.InNamespace(namespace)); err != nil {
		return []common.Result{{Error: []common.Failure{{Text: err.Error()}}}}
	}

	for _, sts := range stsList.Items {
		readyReplicas, found, err := unstructured.NestedInt64(sts.Object, "status", "readyReplicas")
		if err != nil || !found {
			readyReplicas = 0
		}
		replicas, found, err := unstructured.NestedInt64(sts.Object, "spec", "replicas")
		if err != nil || !found {
			replicas = 0
		}
		if readyReplicas == replicas {
			fmt.Print(color.GreenString("StatefulSet: %s, ReadyReplicas: %d/%d\n", sts.GetName(), readyReplicas, replicas))
		} else {
			fmt.Print(color.RedString("StatefulSet: %s, ReadyReplicas: %d/%d\n", sts.GetName(), readyReplicas, replicas))
			result := common.Result{
				Kind:    "StatefulSet",
				Name:    sts.GetName(),
				Error: []common.Failure{
					{
						Text:      fmt.Sprintf("In namespace %s, statefulSet %s is not running", namespace, sts.GetName()),
						Sensitive: []common.Sensitive{},
					},
				},
			}
			results = append(results, result)
		}
	}

	return results
}

func (s *SelHostedInstallAnalyzer) checkDeployments(namespace string) []common.Result {
	var results []common.Result
	deployList := &unstructured.UnstructuredList{}
	deployList.SetKind("Deployment")
	deployList.SetAPIVersion("apps/v1")
	if err := s.instana.client.List(context.Background(), deployList, client.InNamespace(namespace)); err != nil {
		return []common.Result{{Error: []common.Failure{{Text: err.Error()}}}}
	}

	for _, deploy := range deployList.Items {
		availableReplicas, found, err := unstructured.NestedInt64(deploy.Object, "status", "availableReplicas")
		if err != nil || !found {
			availableReplicas = 0
		}
		replicas, found, err := unstructured.NestedInt64(deploy.Object, "spec", "replicas")
		if err != nil || !found {
			replicas = 0
		}
		if availableReplicas == replicas {
			fmt.Print(color.GreenString("Deployment: %s, AvailableReplicas: %d/%d\n", deploy.GetName(), availableReplicas, replicas))
		} else {
			fmt.Print(color.RedString("Deployment: %s, AvailableReplicas: %d/%d\n", deploy.GetName(), availableReplicas, replicas))
			result := common.Result{
				Kind:    "Deployment",
				Name:    deploy.GetName(),
				Error: []common.Failure{
					{
						Text:      fmt.Sprintf("In namespace %s, deployment %s is not available" , namespace, deploy.GetName()),
						Sensitive: []common.Sensitive{},
					},
				},
			}
			results = append(results, result)
		}
	}
	return results
}

func (s *SelHostedInstallAnalyzer) checkPods(namespace string) []common.Result {
	var results []common.Result
	podList := &unstructured.UnstructuredList{}
	podList.SetKind("Pod")
	podList.SetAPIVersion("v1")
	if err := s.instana.client.List(context.Background(), podList, client.InNamespace(namespace)); err != nil {
		return []common.Result{{Error: []common.Failure{{Text: err.Error()}}}}
	}

	for _, pod := range podList.Items {
		phase, found, err := unstructured.NestedString(pod.Object, "status", "phase")
		if err != nil || !found {
			phase = "Unknown"
		}
		if phase == "Running" {
			fmt.Print(color.GreenString("Pod: %s, Status: %s\n", pod.GetName(), phase))
		} else {
			fmt.Print(color.RedString("Pod: %s, Status: %s\n", pod.GetName(), phase))
			result := common.Result{
				Kind:    "Pod",
				Name:    pod.GetName(),
				Error: []common.Failure{
					{
						Text:      fmt.Sprintf("In namespace %s, pod %s is not running", namespace, pod.GetName()),
						Sensitive: []common.Sensitive{},
					},
				},
			}
			results = append(results, result)
		}
	}

	return results
}

func (s *SelHostedInstallAnalyzer) checkCustomResource(keyword, kind, namespace string) ([]common.Result, error) {
	fmt.Print(color.YellowString("Namespace: %s\n", namespace))

	var results []common.Result

	crList := &unstructured.UnstructuredList{}
	crList.SetKind(kind)
	crList.SetAPIVersion("instana.io/v1beta2")

	err := s.instana.client.List(context.Background(), crList, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}

	var crName string
	for _, cr := range crList.Items {
		if strings.Contains(cr.GetName(), keyword) {
			crName = cr.GetName()
			break
		}
	}

	cr := &unstructured.Unstructured{}
	cr.SetKind(kind)
	cr.SetAPIVersion("instana.io/v1beta2")
	if err := s.instana.client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: crName}, cr); err != nil {
		return []common.Result{{Error: []common.Failure{{Text: err.Error()}}}}, nil
	}
	status, found, err := unstructured.NestedString(cr.Object, "status", "componentsStatus")
	if err != nil || !found || status != "Ready" {
		fmt.Print(color.RedString("Custom Resource: %s status: NotReady\n", crName))
		return []common.Result{{
			Kind:    kind,
			Name:    crName,
			Error: []common.Failure{
				{
					Text:      fmt.Sprintf("In namespace %s, %s is not ready: %v", namespace, crName, err),
					Sensitive: []common.Sensitive{},
				},
			},
		}}, nil
	} else {
		fmt.Print(color.GreenString("Custom Resource: %s status: Ready\n", crName))
	}
	return results, nil
}

func (s *SelHostedInstallAnalyzer) getInstanaOperatorLogs(namespace string) (string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=operator",
	})
	if err != nil {
		return "", err
	}

	var logs strings.Builder
	var lastErrorIndex int

	for _, pod := range podList.Items {
		logOptions := &v1.PodLogOptions{}
		req := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, logOptions)
		podLogs, err := req.Stream(context.TODO())
		if err != nil {
			return "", err
		}
		defer podLogs.Close()

		buf := new(strings.Builder)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			return "", err
		}

		logLines := strings.Split(buf.String(), "\n")
		for i, line := range logLines {
			if strings.Contains(line, "ERROR") {
				logs.WriteString(line + "\n")
				start := max(lastErrorIndex, i-10)
				for j := start; j < i; j++ {
					if strings.Contains(logLines[j], "WARNING") {
						logs.WriteString(logLines[j] + "\n")
					}
				}
				lastErrorIndex = i + 1
			}
		}
	}

	return logs.String(), nil
}

func max(x, y int) int {
	if x < y {
		return y
	}
	return x
}
