/*
Copyright 2026.

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

package controller

import (
	"context"

	"github.com/concourse/concourse/atc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

var _ = Describe("Worker Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-worker"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		worker := &concoursev1alpha1.Worker{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Worker")
			err := k8sClient.Get(ctx, typeNamespacedName, worker)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Worker{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.WorkerSpec{
						InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: "test-instance"},
						WorkerName:   "worker-1",
						DesiredState: concoursev1alpha1.WorkerDesiredStateActive,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Worker{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Worker")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when instance is missing", func() {
			By("Reconciling the created resource")
			reconciler := &WorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			// Instance doesn't exist: Get returns NotFound → IgnoreNotFound → no error, requeue.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set Ready=False with InstanceNotReady reason when instance exists but is not ready", func() {
			By("Creating an instance without Ready condition")
			notReadyInst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-instance-not-ready", Namespace: "default"},
				Spec:       concoursev1alpha1.InstanceSpec{URL: "https://ci.example.com"},
			}
			Expect(k8sClient.Create(ctx, notReadyInst)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyInst) })

			By("Creating a worker referencing the not-ready instance")
			notReadyWorker := &concoursev1alpha1.Worker{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-instance-check", Namespace: "default"},
				Spec: concoursev1alpha1.WorkerSpec{
					InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: "worker-instance-not-ready"},
					WorkerName:   "worker-1",
					DesiredState: concoursev1alpha1.WorkerDesiredStateActive,
				},
			}
			Expect(k8sClient.Create(ctx, notReadyWorker)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyWorker) })

			reconciler := &WorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "worker-instance-check", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False with InstanceNotReady reason")
			fetched := &concoursev1alpha1.Worker{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "worker-instance-check", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InstanceNotReady"))
		})
	})

	Context("When Concourse API responds successfully", func() {
		ctx := context.Background()

		It("should sync worker status from ListWorkers when DesiredState=active", func() {
			By("Setting up a ready instance with a fake client that returns a known worker")
			cache := concourse.NewCache()
			fake := &fakeClient{
				team:      &fakeTeam{name: "main"},
				getInfoFn: func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) {
					return []atc.Worker{
						{
							Name:             "worker-a",
							State:            "running",
							Platform:         "linux",
							Tags:             atc.Tags{"tag1"},
							ActiveContainers: 3,
							ActiveVolumes:    7,
						},
					}, nil
				},
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "wk-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })

			wk := &concoursev1alpha1.Worker{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-status", Namespace: "default"},
				Spec: concoursev1alpha1.WorkerSpec{
					InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: inst.Name},
					WorkerName:   "worker-a",
					DesiredState: concoursev1alpha1.WorkerDesiredStateActive,
				},
			}
			Expect(k8sClient.Create(ctx, wk)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, wk) })

			reconciler := &WorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "worker-status", Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying status fields are synced from Concourse")
			fetched := &concoursev1alpha1.Worker{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.ActualState).To(Equal("running"))
			Expect(fetched.Status.Platform).To(Equal("linux"))
			Expect(fetched.Status.Tags).To(ConsistOf("tag1"))
			Expect(fetched.Status.ActiveContainers).To(Equal(3))
			Expect(fetched.Status.ActiveVolumes).To(Equal(7))
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should call LandWorker when DesiredState=land", func() {
			cache := concourse.NewCache()
			landCalled := false
			fake := &fakeClient{
				team:      &fakeTeam{name: "main"},
				getInfoFn: func() (atc.Info, error) { return atc.Info{}, nil },
				landWorkerFn: func(name string) error {
					landCalled = true
					Expect(name).To(Equal("worker-land"))
					return nil
				},
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "wk-land-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })

			wk := &concoursev1alpha1.Worker{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-land-cr", Namespace: "default"},
				Spec: concoursev1alpha1.WorkerSpec{
					InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: inst.Name},
					WorkerName:   "worker-land",
					DesiredState: concoursev1alpha1.WorkerDesiredStateLand,
				},
			}
			Expect(k8sClient.Create(ctx, wk)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, wk) })

			reconciler := &WorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "worker-land-cr", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(landCalled).To(BeTrue(), "expected LandWorker to be called")
		})

		It("should call PruneWorker when DesiredState=prune", func() {
			cache := concourse.NewCache()
			pruneCalled := false
			fake := &fakeClient{
				team:      &fakeTeam{name: "main"},
				getInfoFn: func() (atc.Info, error) { return atc.Info{}, nil },
				pruneWorkerFn: func(name string) error {
					pruneCalled = true
					Expect(name).To(Equal("worker-prune"))
					return nil
				},
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "wk-prune-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })

			wk := &concoursev1alpha1.Worker{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-prune-cr", Namespace: "default"},
				Spec: concoursev1alpha1.WorkerSpec{
					InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: inst.Name},
					WorkerName:   "worker-prune",
					DesiredState: concoursev1alpha1.WorkerDesiredStatePrune,
				},
			}
			Expect(k8sClient.Create(ctx, wk)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, wk) })

			reconciler := &WorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "worker-prune-cr", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(pruneCalled).To(BeTrue(), "expected PruneWorker to be called")
		})

		It("should leave status.ActualState empty when worker not found in ListWorkers", func() {
			cache := concourse.NewCache()
			fake := &fakeClient{
				team:      &fakeTeam{name: "main"},
				getInfoFn: func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) {
					return []atc.Worker{{Name: "some-other-worker", State: "running"}}, nil
				},
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "wk-notfound-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })

			wk := &concoursev1alpha1.Worker{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-notfound-cr", Namespace: "default"},
				Spec: concoursev1alpha1.WorkerSpec{
					InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: inst.Name},
					WorkerName:   "worker-missing",
					DesiredState: concoursev1alpha1.WorkerDesiredStateActive,
				},
			}
			Expect(k8sClient.Create(ctx, wk)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, wk) })

			reconciler := &WorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "worker-notfound-cr", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			fetched := &concoursev1alpha1.Worker{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.ActualState).To(BeEmpty(), "worker not found in list should leave ActualState empty")
		})
	})
})
