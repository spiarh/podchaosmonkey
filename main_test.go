package main

import (
	"context"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeleteRandomPod(t *testing.T) {
	namespace := "workloads"
	pods := newPods(namespace, 1)
	client := fake.NewSimpleClientset(pods[0])
	informerFactory := newInformerFactory(client, namespace, "")
	podChaosMonkey := New(client, informerFactory, namespace, false)

	err := informerFactory.Core().V1().Pods().Informer().GetIndexer().Add(pods[0])
	if err != nil {
		t.Error(err)
	}

	t.Log("there is one pod in the namespace and the pod is in the cache")
	err = podChaosMonkey.deleteRandomPod(getRandomPodKey)
	if err != nil {
		t.Error(err)
	}

	_, err = client.CoreV1().Pods(namespace).Get(context.TODO(), "pod0", metav1.GetOptions{})
	if !apierror.IsNotFound(err) {
		t.Errorf("pod0 should have been deleted")
	}

	t.Log("there are no pods in the namespace but the pod is still in the cache")
	err = podChaosMonkey.deleteRandomPod(getRandomPodKey)
	if err != nil {
		t.Error(err)
	}

	// manually remove the pod from the cache
	err = informerFactory.Core().V1().Pods().Informer().GetIndexer().Delete(pods[0])
	if err != nil {
		t.Error(err)
	}

	t.Log("there are no pods in the namespace and the cache is empty")
	err = podChaosMonkey.deleteRandomPod(getRandomPodKey)
	if err != nil {
		t.Error(err)
	}
}

func TestDeleteRandomPods(t *testing.T) {
	namespace := "workloads"
	pods := newPods(namespace, 3)
	client := fake.NewSimpleClientset(pods[0], pods[1], pods[2])
	informerFactory := newInformerFactory(client, namespace, "")
	podChaosMonkey := New(client, informerFactory, namespace, false)

	for _, pod := range pods {
		err := informerFactory.Core().V1().Pods().Informer().GetIndexer().Add(pod)
		if err != nil {
			t.Error(err)
		}
	}

	fn := func(keys []string) string { return "workloads/pod0" }

	t.Log("there are three pods in the namespace and the pods are in the cache")
	t.Logf("delete a pod (pod0) pseudorandomly")
	err := podChaosMonkey.deleteRandomPod(fn)
	if err != nil {
		t.Error(err)
	}
	_, err = client.CoreV1().Pods(namespace).Get(context.TODO(), "pod0", metav1.GetOptions{})
	if !apierror.IsNotFound(err) {
		t.Errorf("pod0 should have been deleted")
	}

	t.Log("pod0 does not exist but still in the cache")
	err = podChaosMonkey.deleteRandomPod(fn)
	if err != nil {
		t.Error(err)
	}

	fnUnknownKey := func(keys []string) string { return "namespace/idontexist" }
	t.Log("the pod key is not in the cache")
	err = podChaosMonkey.deleteRandomPod(fnUnknownKey)
	if err != nil {
		t.Error(err)
	}

	// manually remove the pod from the cache
	err = informerFactory.Core().V1().Pods().Informer().GetIndexer().Delete(pods[0])
	if err != nil {
		t.Error(err)
	}

	t.Log("pod1 and pod2 still exist and they are in the cache")
	err = podChaosMonkey.deleteRandomPod(getRandomPodKey)
	if err != nil {
		t.Error(err)
	}
}

func newPods(namespace string, count int) []*corev1.Pod {
	pods := make([]*corev1.Pod, 0, count)

	for i := 0; i <= count-1; i++ {
		pods = append(pods, &corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod" + strconv.Itoa(i),
				Namespace: namespace,
				//NOTE: label selector do not work by default in unit test,
				// need to investigate
				Labels: map[string]string{
					"key": "value",
				},
			},
			Status: corev1.PodStatus{
				//NOTE: field selector do not work by default in unit test,
				// need to investigate
				Phase: corev1.PodRunning,
			}})
	}
	return pods
}
