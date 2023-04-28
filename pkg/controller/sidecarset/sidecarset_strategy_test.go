/*
Copyright 2020 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sidecarset

import (
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"math/rand"
	"reflect"
	"sigs.k8s.io/yaml"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	utilpointer "k8s.io/utils/pointer"
)

type FactorySidecarSet func() *appsv1alpha1.SidecarSet
type FactoryPods func(int, int, int) []*corev1.Pod

func factoryPodsCommon(count, upgraded int, sidecarSet *appsv1alpha1.SidecarSet) []*corev1.Pod {
	control := sidecarcontrol.New(sidecarSet)
	pods := make([]*corev1.Pod, 0, count)
	for i := 0; i < count; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					sidecarcontrol.SidecarSetHashAnnotation:             `{"test-sidecarset":{"hash":"aaa","sidecarList":["test-sidecar"]}}`,
					sidecarcontrol.SidecarSetHashWithoutImageAnnotation: `{"test-sidecarset":{"hash":"without-aaa","sidecarList":["test-sidecar"]}}`,
				},
				Name: fmt.Sprintf("pod-%d", i),
				Labels: map[string]string{
					"app": "sidecar",
				},
				CreationTimestamp: metav1.Now(),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:1.15.1",
					},
					{
						Name:  "test-sidecar",
						Image: "test-image:v1",
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:    "nginx",
						Image:   "nginx:1.15.1",
						ImageID: "docker-pullable://nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
						Ready:   true,
					},
					{
						Name:    "test-sidecar",
						Image:   "test-image:v1",
						ImageID: testImageV1ImageID,
						Ready:   true,
					},
				},
			},
		}
		pods = append(pods, pod)
	}
	for i := 0; i < upgraded; i++ {
		pods[i].Spec.Containers[1].Image = "test-image:v2"
		control.UpdatePodAnnotationsInUpgrade([]string{"test-sidecar"}, pods[i])
	}
	return pods
}

func factoryPods(count, upgraded, upgradedAndReady int) []*corev1.Pod {
	sidecarSet := factorySidecarSet()
	pods := factoryPodsCommon(count, upgraded, sidecarSet)
	for i := 0; i < upgradedAndReady; i++ {
		pods[i].Status.ContainerStatuses[1].Image = "test-image:v2"
		pods[i].Status.ContainerStatuses[1].ImageID = testImageV2ImageID
	}

	return pods
}

func factorySidecarSet() *appsv1alpha1.SidecarSet {
	sidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation:             "bbb",
				sidecarcontrol.SidecarSetHashWithoutImageAnnotation: "without-aaa",
			},
			Name:   "test-sidecarset",
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "test-sidecar",
						Image: "test-image:v2",
					},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "sidecar"},
			},
			UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
				//Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			},
			RevisionHistoryLimit: utilpointer.Int32Ptr(10),
		},
	}

	return sidecarSet
}

func TestGetNextUpgradePods(t *testing.T) {
	testGetNextUpgradePods(t, factoryPods, factorySidecarSet)
}

func testGetNextUpgradePods(t *testing.T, factoryPods FactoryPods, factorySidecar FactorySidecarSet) {
	cases := []struct {
		name                   string
		getPods                func() []*corev1.Pod
		getSidecarset          func() *appsv1alpha1.SidecarSet
		exceptNeedUpgradeCount int
	}{
		{
			name: "only maxUnavailable(int=10), and pods(count=100, upgraded=30, upgradedAndReady=26)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 30, 26)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 6,
		},
		{
			name: "only maxUnavailable(string=10%), and pods(count=1000, upgraded=300, upgradedAndReady=260)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 300, 260)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "10%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 60,
		},
		{
			name: "only maxUnavailable(string=5%), and pods(count=1000, upgraded=300, upgradedAndReady=250)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 300, 250)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "5%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 0,
		},
		{
			name: "only maxUnavailable(int=100), and pods(count=100, upgraded=30, upgradedAndReady=27)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 30, 27)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 70,
		},
		{
			name: "partition(int=180) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 180,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 20,
		},
		{
			name: "partition(int=100) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 60,
		},
		{
			name: "partition(string=18%) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "18%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 20,
		},
		{
			name: "partition(string=10%) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "10%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 60,
		},
		{
			name: "selector(app=test, count=30) maxUnavailable(int=100), and pods(count=1000, upgraded=0, upgradedAndReady=0)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 0, 0)
				for i := 0; i < 30; i++ {
					pods[i].Labels["app"] = "test"
				}
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount: 30,
		},
	}
	strategy := NewStrategy()
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := sidecarcontrol.New(cs.getSidecarset())
			pods := cs.getPods()
			injectedPods := strategy.GetNextUpgradePods(control, pods)
			if cs.exceptNeedUpgradeCount != len(injectedPods) {
				t.Fatalf("except NeedUpgradeCount(%d), but get value(%d)", cs.exceptNeedUpgradeCount, len(injectedPods))
			}
		})
	}
}

func TestParseUpdateScatterTerms(t *testing.T) {
	cases := []struct {
		name                  string
		getPods               func() []*corev1.Pod
		getScatterStrategy    func() appsv1alpha1.UpdateScatterStrategy
		exceptScatterStrategy func() appsv1alpha1.UpdateScatterStrategy
	}{
		{
			name: "only scatter terms",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 0, 0)
				return pods
			},
			getScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
				}
				return scatter
			},
			exceptScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
				}
				return scatter
			},
		},
		{
			name: "regular and scatter terms",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 0, 0)
				pods[0].Labels["key-4"] = "value-4-0"
				pods[1].Labels["key-4"] = "value-4-1"
				pods[2].Labels["key-4"] = "value-4-2"
				pods[3].Labels["key-4"] = "value-4"
				pods[4].Labels["key-4"] = "value-4"
				pods[5].Labels["key-4"] = "value-4"
				return pods
			},
			getScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
					{
						Key:   "key-4",
						Value: "*",
					},
				}
				return scatter
			},
			exceptScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
					{
						Key:   "key-4",
						Value: "value-4-0",
					},
					{
						Key:   "key-4",
						Value: "value-4-1",
					},
					{
						Key:   "key-4",
						Value: "value-4-2",
					},
					{
						Key:   "key-4",
						Value: "value-4",
					},
				}
				return scatter
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pods := cs.getPods()
			scatter := cs.getScatterStrategy()
			exceptScatter := cs.exceptScatterStrategy()
			newScatter := parseUpdateScatterTerms(scatter, pods)
			if !reflect.DeepEqual(newScatter, exceptScatter) {
				except, _ := json.Marshal(exceptScatter)
				new, _ := json.Marshal(newScatter)
				t.Fatalf("except scatter(%s), but get scatter(%s)", string(except), string(new))
			}
		})
	}
}

func Random(pods []*corev1.Pod) []*corev1.Pod {
	for i := len(pods) - 1; i > 0; i-- {
		num := rand.Intn(i + 1)
		pods[i], pods[num] = pods[num], pods[i]
	}
	return pods
}

func TestSortNextUpgradePods(t *testing.T) {
	testSortNextUpgradePods(t, factoryPods, factorySidecarSet)
}

func testSortNextUpgradePods(t *testing.T, factoryPods FactoryPods, factorySidecar FactorySidecarSet) {
	cases := []struct {
		name                  string
		getPods               func() []*corev1.Pod
		getSidecarset         func() *appsv1alpha1.SidecarSet
		exceptNextUpgradePods []string
	}{
		{
			name: "sort by pod.CreationTimestamp, maxUnavailable(int=10) and pods(count=20, upgraded=10, upgradedAndReady=5)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(20, 10, 5)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				return sidecarSet
			},
			exceptNextUpgradePods: []string{"pod-19", "pod-18", "pod-17", "pod-16", "pod-15"},
		},
		{
			name: "not ready priority, maxUnavailable(int=10) and pods(count=20, upgraded=10, upgradedAndReady=5)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(20, 10, 5)
				podutil.GetPodReadyCondition(pods[10].Status).Status = corev1.ConditionFalse
				podutil.GetPodReadyCondition(pods[13].Status).Status = corev1.ConditionFalse
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				return sidecarSet
			},
			exceptNextUpgradePods: []string{"pod-13", "pod-10", "pod-19", "pod-18", "pod-17", "pod-16", "pod-15"},
		},
	}

	strategy := NewStrategy()
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := sidecarcontrol.New(cs.getSidecarset())
			pods := cs.getPods()
			injectedPods := strategy.GetNextUpgradePods(control, pods)
			if len(cs.exceptNextUpgradePods) != len(injectedPods) {
				t.Fatalf("except NeedUpgradeCount(%d), but get value(%d)", len(cs.exceptNextUpgradePods), len(injectedPods))
			}

			for i, name := range cs.exceptNextUpgradePods {
				if injectedPods[i].Name != name {
					t.Fatalf("except NextUpgradePods[%d:%s], but get pods[%s]", i, name, injectedPods[i])
				}
			}
		})
	}
}
func TestName(t *testing.T) {
	po := `apiVersion: v1
kind: Pod
metadata:
  annotations:
    ProviderCreate: done
    k8s.aliyun.com/cluster-dns: 192.168.0.10
    k8s.aliyun.com/cluster-domain: cluster.local
    k8s.aliyun.com/eci-client-token: 7f71ba34-42eb-4d1f-9118-fac96815d2e6
    k8s.aliyun.com/eci-created-by-template: "true"
    k8s.aliyun.com/eci-drop-apiserver-interactive-caps: ""
    k8s.aliyun.com/eci-instance-cpu: "2.0"
    k8s.aliyun.com/eci-instance-id: eci-wz9c9z4zt9zfxlf4dd8v
    k8s.aliyun.com/eci-instance-mem: "4.0"
    k8s.aliyun.com/eci-instance-spec: 2.0-4.0Gi
    k8s.aliyun.com/eci-instance-zone: cn-shenzhen-d
    k8s.aliyun.com/eci-kube-proxy-enabled: "true"
    k8s.aliyun.com/eci-matched-image-cache: imc-wz9dxo0xw2w9q4s6wnf1
    k8s.aliyun.com/eci-request-id: 73A3E610-54E4-5010-94CC-341C03003909
    k8s.aliyun.com/eci-schedule-result: finished
    k8s.aliyun.com/eci-security-group: sg-wz90jvapmq6ckfqrjd38
    k8s.aliyun.com/eci-vpc: vpc-wz9v17plp9fvtk7ku6eaw
    k8s.aliyun.com/eci-vswitch: vsw-wz9qk76jf6wja3clt4flc
    k8s.aliyun.com/eni-instance-id: eni-wz9hdgyzaznsv8h4luzb
    k8s.aliyun.com/k8s-version: v1.22.3-aliyun.1
    k8s.aliyun.com/vk-version: v2.8.5-2023-02-21-08-59-UTC
    kruise.io/sidecarset-hash: '{"test-sidecarset":{"updateTimestamp":"2023-04-27T10:05:54Z","hash":"xvwzb9v5d8cdz567x2x9w2dz4wd9c6xb82v46xc8f4wb4c88xzd5bbwzbdd84wc7","sidecarSetName":"test-sidecarset","sidecarList":["sidecar1"],"controllerRevision":"test-sidecarset-7fffd8d6c8"}}'
    kruise.io/sidecarset-hash-without-image: '{"test-sidecarset":{"updateTimestamp":"2023-04-27T10:04:45Z","hash":"49b4cbbdzf8v7b2z6wwcdzd4252cd7b8zv9cbf66wxxxzw444bbbfw82b54f8246","sidecarSetName":"test-sidecarset","sidecarList":["sidecar1"],"controllerRevision":""}}'
    kruise.io/sidecarset-injected-list: test-sidecarset
    kruise.io/sidecarset-inplace-update-state: '{"test-sidecarset":{"revision":"xvwzb9v5d8cdz567x2x9w2dz4wd9c6xb82v46xc8f4wb4c88xzd5bbwzbdd84wc7","updateTimestamp":"2023-04-27T10:05:54Z","lastContainerStatuses":{"sidecar1":{"imageID":"docker.io/library/nginx@sha256:0d17b565c37bcbd895e9d92315a05c1c3c9a29f762b011a10c54a66cd53c9b31"}}}}'
    kubernetes.io/pod-stream-port: "10250"
    kubernetes.io/psp: ack.privileged
    sidecar.istio.io/inject: "false"
    traffic.sidecar.istio.io/excludeInboundPorts: "9999"
  creationTimestamp: "2023-04-27T10:04:45Z"
  generateName: appp-76c844fd67-
  labels:
    app: appp
    pod-template-hash: 76c844fd67
  name: appp-76c844fd67-pvk4w
  namespace: test-infra
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicaSet
    name: appp-76c844fd67
    uid: f59a5a27-6fa8-4d91-98bd-df874ce6ab1e
  resourceVersion: "679242411"
  uid: c27310d8-233e-4b40-abbd-ae3036bfa871
spec:
  containers:
  - command:
    - sleep
    - 999d
    env:
    - name: IS_INJECTED
      value: "true"
    image: nginx
    imagePullPolicy: Always
    name: sidecar1
    resources: {}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /var/log
      name: log-volume
    - mountPath: /var/run/secrets/kubernetes.io/serviceaccount
      name: kube-api-access-mzzbb
      readOnly: true
  - command:
    - sleep
    - 9999h
    image: centos
    imagePullPolicy: Always
    name: app
    resources: {}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /var/run/secrets/kubernetes.io/serviceaccount
      name: kube-api-access-mzzbb
      readOnly: true
  dnsPolicy: ClusterFirst
  enableServiceLinks: true
  imagePullSecrets:
  - name: acr-credential-858c8a8356caece07dbb6311eb4adb2c
  nodeName: virtual-kubelet-cn-shenzhen-d
  preemptionPolicy: PreemptLowerPriority
  priority: 0
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  serviceAccount: default
  serviceAccountName: default
  terminationGracePeriodSeconds: 0
  tolerations:
  - effect: NoExecute
    key: node.kubernetes.io/not-ready
    operator: Exists
  - effect: NoExecute
    key: node.kubernetes.io/unreachable
    operator: Exists
  volumes:
  - name: kube-api-access-mzzbb
    projected:
      defaultMode: 420
      sources:
      - serviceAccountToken:
          expirationSeconds: 3607
          path: token
      - configMap:
          items:
          - key: ca.crt
            path: ca.crt
          name: kube-root-ca.crt
      - downwardAPI:
          items:
          - fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
            path: namespace
  - emptyDir: {}
    name: log-volume
status:
  conditions:
  - lastProbeTime: null
    lastTransitionTime: "2023-04-27T10:04:59Z"
    status: "True"
    type: Initialized
  - lastProbeTime: null
    lastTransitionTime: "2023-04-27T10:05:36Z"
    status: "True"
    type: Ready
  - lastProbeTime: null
    lastTransitionTime: "2023-04-27T10:05:36Z"
    status: "True"
    type: ContainersReady
  - lastProbeTime: null
    lastTransitionTime: "2023-04-27T10:04:59Z"
    status: "True"
    type: PodScheduled
  - lastProbeTime: null
    lastTransitionTime: "2023-04-27T10:04:59Z"
    status: "True"
    type: ContainerHasSufficientDisk
  containerStatuses:
  - containerID: containerd://1c8fe5e2da9ceb5f06cdbc3af1abf41e010f5848539696258366577f048ee8bb
    image: docker.io/library/centos:latest
    imageID: docker.io/library/centos@sha256:a27fd8080b517143cbbbab9dfb7c8571c40d67d534bbdee55bd6c473f432b177
    lastState: {}
    name: app
    ready: true
    restartCount: 0
    started: true
    state:
      running:
        startedAt: "2023-04-27T10:05:36Z"
  - containerID: containerd://51b875a50e7a0eb665c6ec9a9afdffbb37e1fdfe0001a91742e747dd4b22c8c5
    image: docker.io/library/nginx:latest
    imageID: docker.io/library/nginx@sha256:0d17b565c37bcbd895e9d92315a05c1c3c9a29f762b011a10c54a66cd53c9b31
    lastState: {}
    name: sidecar1
    ready: true
    restartCount: 1
    started: true
    state:
      running:
        startedAt: "2023-04-27T10:05:56Z"
  hostIP: 10.111.84.96
  phase: Running
  podIP: 10.111.84.96
  podIPs:
  - ip: 10.111.84.96
  qosClass: BestEffort
  startTime: "2023-04-27T10:04:59Z"
`
	p := &corev1.Pod{}
	err := yaml.Unmarshal([]byte(po), p)
	if err != nil {
		panic(err)
	}
	pods := []*corev1.Pod{
		p,
	}
	sc := `apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  annotations:
    kruise.io/sidecarset-hash: f5f27fzv7d49f4745d4xbcf6v78d42wfvw4xxd8w5f89f6444b6bfdv84xbvbcx8
    kruise.io/sidecarset-hash-without-image: 49b4cbbdzf8v7b2z6wwcdzd4252cd7b8zv9cbf66wxxxzw444bbbfw82b54f8246
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"apps.kruise.io/v1alpha1","kind":"SidecarSet","metadata":{"annotations":{},"name":"test-sidecarset"},"spec":{"containers":[{"command":["sleep","999d"],"image":"bb","imagePullPolicy":"Always","name":"sidecar1","volumeMounts":[{"mountPath":"/var/log","name":"log-volume"}]}],"selector":{"matchLabels":{"app":"appp"}},"updateStrategy":{"maxUnavailable":20,"type":"RollingUpdate"},"volumes":[{"emptyDir":{},"name":"log-volume"}]}}
  creationTimestamp: "2023-04-27T07:22:59Z"
  generation: 56
  name: test-sidecarset
  resourceVersion: "679386615"
  uid: 2b87576d-c710-4c44-a66b-a958ccbf730c
spec:
  containers:
  - command:
    - sleep
    - 999d
    image: asd
    imagePullPolicy: Always
    name: sidecar1
    podInjectPolicy: BeforeAppContainer
    resources: {}
    shareVolumePolicy:
      type: disabled
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    upgradeStrategy:
      upgradeType: ColdUpgrade
    volumeMounts:
    - mountPath: /var/log
      name: log-volume
  injectionStrategy: {}
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: appp
  updateStrategy:
    maxUnavailable: 20
    partition: 0
    type: RollingUpdate
  volumes:
  - emptyDir: {}
    name: log-volume
status:
  collisionCount: 0
  latestRevision: test-sidecarset-647888bd78
  matchedPods: 1
  observedGeneration: 56
  readyPods: 0
  updatedPods: 0
`

	sidecarset := &appsv1alpha1.SidecarSet{}
	err = yaml.Unmarshal([]byte(sc), sidecarset)
	if err != nil {
		panic(err)
	}
	control := sidecarcontrol.New(sidecarset)
	// wait to upgrade pod index
	var waitUpgradedIndexes []int
	strategy := sidecarset.Spec.UpdateStrategy
	isSelected := func(pod *corev1.Pod) bool {
		//when selector is nil, always return true
		if strategy.Selector == nil {
			return true
		}
		// if selector failed, always return false
		selector, err := metav1.LabelSelectorAsSelector(strategy.Selector)
		if err != nil {
			klog.Errorf("sidecarSet(%s) rolling selector error, err: %v", sidecarset.Name, err)
			return false
		}
		//matched
		if selector.Matches(labels.Set(pod.Labels)) {
			return true
		}
		//Not matched, then return false
		return false
	}

	for index, pod := range pods {
		isUpdated := sidecarcontrol.IsPodSidecarUpdated(sidecarset, pod)
		selected := isSelected(pod)
		upgradable := control.IsSidecarSetUpgradable(pod)
		fmt.Println(isUpdated, selected, upgradable)
		if !isUpdated && selected && upgradable {
			waitUpgradedIndexes = append(waitUpgradedIndexes, index)
		}
	}
	fmt.Println(waitUpgradedIndexes)
}
