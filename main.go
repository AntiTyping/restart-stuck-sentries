package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

func Strip(str string) string {
	return re.ReplaceAllString(str, "")
}

func main() {
	clientset := getClientset()

	namespaces := NamespacesWithPrefix(clientset, "cosmos-sentry")
	for _, ns := range namespaces {
		fmt.Printf("Processing namespace: %s\n", ns)

		pods := PodsInNamespace(clientset, ns)

		for _, pod := range pods {
			killHangedPods(clientset, ns, pod)
		}
	}
}

func getClientset() *kubernetes.Clientset {
	config, err :=
		clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			nil,
		).ClientConfig()

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}
	return clientset
}

func NamespacesWithPrefix(clientset *kubernetes.Clientset, prefix string) []string {
	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	var filteredNamespaces []string
	for _, namespace := range namespaces.Items {
		if strings.HasPrefix(namespace.Name, prefix) {
			filteredNamespaces = append(filteredNamespaces, namespace.Name)
		}
	}

	return filteredNamespaces
}

func PodsInNamespace(clientset *kubernetes.Clientset, namespace string) []string {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	var podNames []string
	for _, pod := range pods.Items {
		podNames = append(podNames, pod.Name)
	}

	return podNames
}

func killHangedPods(clientset *kubernetes.Clientset, namespace string, name string) bool {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	podLogOpts := corev1.PodLogOptions{Container: "node"}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		log.Fatal(err)
	}

	sc := bufio.NewScanner(podLogs)
	var last_line string
	for sc.Scan() {
		last_line = sc.Text()
	}

	if strings.Contains(last_line, "SignerListener: Connected module=privval") == true {
		fmt.Printf(" ☠️  Killing hanged pod: %s\n", name)
		err = clientset.
			CoreV1().
			Pods(namespace).
			Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil {
			log.Fatal(err)
		}
		return true
	} else {
		fmt.Printf(" ✅  %s\n", name)
	}
	return false
}
