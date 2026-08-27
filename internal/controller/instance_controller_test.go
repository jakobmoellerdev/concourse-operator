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
	"fmt"
	"time"

	"github.com/concourse/concourse/atc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

var _ = Describe("Instance Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-instance"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		instance := &concoursev1alpha1.Instance{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Instance")
			err := k8sClient.Get(ctx, typeNamespacedName, instance)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Instance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: testInstanceSpec(),
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Instance{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Instance")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource without error", func() {
			By("Reconciling the created resource")
			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			// Reconcile will attempt to connect to Concourse (which doesn't exist in tests).
			// We expect a requeue without error — auth failure sets status conditions, not returns error.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add the instance finalizer on first reconcile", func() {
			By("Creating a fresh instance for this test")
			finalizerInst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance-finalizer", Namespace: "default"},
				Spec:       testInstanceSpec(),
			}
			Expect(k8sClient.Create(ctx, finalizerInst)).To(Succeed())
			DeferCleanup(func() {
				// Remove finalizer manually before deleting to avoid test interference.
				latest := &concoursev1alpha1.Instance{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-instance-finalizer", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, instanceFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			finalizerNSN := types.NamespacedName{Name: "test-instance-finalizer", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: finalizerNSN})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the finalizer was added")
			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, finalizerNSN, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, instanceFinalizer)).To(BeTrue())
		})

		It("should set Authenticated=False and Ready=False when Concourse is unreachable", func() {
			By("Creating a fresh instance for this test")
			authInst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance-auth", Namespace: "default"},
				Spec:       testInstanceSpec(),
			}
			Expect(k8sClient.Create(ctx, authInst)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Instance{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-instance-auth", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, instanceFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			authNSN := types.NamespacedName{Name: "test-instance-auth", Namespace: "default"}

			By("Reconciling against an unreachable Concourse URL")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: authNSN})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying status conditions reflect auth failure")
			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, authNSN, fetched)).To(Succeed())
			readyCond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When deleting a resource", func() {
		const deleteName = "test-instance-delete"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: deleteName, Namespace: "default"}

		It("should remove the finalizer and not error on deletion", func() {
			By("Creating the instance")
			resource := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: deleteName, Namespace: "default"},
				Spec:       testInstanceSpec(),
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}

			By("First reconcile adds finalizer")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, instanceFinalizer)).To(BeTrue())

			By("Deleting the resource triggers finalizer removal")
			Expect(k8sClient.Delete(ctx, fetched)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer is removed")
			afterDeletion := &concoursev1alpha1.Instance{}
			err = k8sClient.Get(ctx, typeNamespacedName, afterDeletion)
			if err == nil {
				Expect(controllerutil.ContainsFinalizer(afterDeletion, instanceFinalizer)).To(BeFalse())
			}
		})
	})

	Context("When Concourse API responds successfully", func() {
		ctx := context.Background()

		It("should set Ready=True, Version, and WorkerCount from fake client", func() {
			By("Creating a fresh instance")
			inst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "instance-happy", Namespace: "default"},
				Spec:       testInstanceSpec(),
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
			nsn := types.NamespacedName{Name: "instance-happy", Namespace: "default"}
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Instance{}
				if err := k8sClient.Get(ctx, nsn, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, instanceFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{name: "main"},
				getInfoFn: func() (atc.Info, error) {
					return atc.Info{Version: "7.11.0"}, nil
				},
				listWorkersFn: func() ([]atc.Worker, error) {
					return []atc.Worker{{Name: "worker-1"}, {Name: "worker-2"}}, nil
				},
			}

			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}

			By("First reconcile adds finalizer (cache miss expected)")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			By("Re-fetch to get post-finalizer ResourceVersion, then seed cache")
			afterFinalizer := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, afterFinalizer)).To(Succeed())
			cache.Set(afterFinalizer, fake)

			By("Second reconcile hits cache and succeeds")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Minute))

			By("Verifying status fields and conditions")
			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Version).To(Equal("7.11.0"))
			Expect(fetched.Status.WorkerCount).NotTo(BeNil())
			Expect(*fetched.Status.WorkerCount).To(Equal(int32(2)))
			readyCond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			authCond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionAuthenticated)
			Expect(authCond).NotTo(BeNil())
			Expect(authCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should respect a custom interval from spec", func() {
			By("Creating an instance with a 2-minute interval")
			twoMin := metav1.Duration{Duration: 2 * time.Minute}
			inst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "instance-interval", Namespace: "default"},
				Spec: concoursev1alpha1.InstanceSpec{
					URL:                 "https://ci.example.com",
					Auth:                testInstanceAuth(),
					HealthProbeInterval: &twoMin,
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
			nsn := types.NamespacedName{Name: "instance-interval", Namespace: "default"}
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Instance{}
				if err := k8sClient.Get(ctx, nsn, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, instanceFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			cache := concourse.NewCache()
			fake := &fakeClient{
				team:          &fakeTeam{name: "main"},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{Version: "7.11.0"}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}

			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}

			By("First reconcile adds finalizer (cache miss expected)")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			By("Re-fetch to get post-finalizer ResourceVersion, then seed cache")
			afterFinalizer := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, afterFinalizer)).To(Succeed())
			cache.Set(afterFinalizer, fake)

			By("Second reconcile uses custom interval")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(2 * time.Minute))
		})
	})

	Context("When reconciling the wall banner", func() {
		ctx := context.Background()

		// setupWallInstance creates a fresh Instance, runs the first reconcile to
		// attach the finalizer, then seeds the cache with the given fake client
		// so the second reconcile exercises syncWall against it.
		setupWallInstance := func(name string, spec concoursev1alpha1.InstanceSpec, fake *fakeClient) (types.NamespacedName, *InstanceReconciler, *concourse.Cache) {
			inst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec:       spec,
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
			nsn := types.NamespacedName{Name: name, Namespace: "default"}
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Instance{}
				if err := k8sClient.Get(ctx, nsn, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, instanceFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			cache := concourse.NewCache()
			reconciler := &InstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}

			By("First reconcile adds finalizer (cache miss expected)")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			By("Re-fetch to get post-finalizer ResourceVersion, then seed cache")
			afterFinalizer := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, afterFinalizer)).To(Succeed())
			cache.Set(afterFinalizer, fake)

			return nsn, reconciler, cache
		}

		It("should call SetWall and record WallMessage when spec.Wall is set", func() {
			var setWallCalls int
			var lastWall atc.Wall
			fake := &fakeClient{
				team:          &fakeTeam{name: "main"},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{Version: "7.11.0"}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
				setWallFn: func(w atc.Wall) error {
					setWallCalls++
					lastWall = w
					return nil
				},
			}

			spec := testInstanceSpec()
			ttl := metav1.Duration{Duration: 10 * time.Minute}
			spec.Wall = &concoursev1alpha1.WallConfig{Message: "scheduled maintenance", TTL: &ttl}

			nsn, reconciler, _ := setupWallInstance("instance-wall-set", spec, fake)

			By("Second reconcile hits cache and sets the wall")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			Expect(setWallCalls).To(Equal(1))
			Expect(lastWall.Message).To(Equal("scheduled maintenance"))
			Expect(lastWall.TTL).To(Equal(10 * time.Minute))

			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.WallMessage).To(Equal("scheduled maintenance"))

			readyCond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should not call SetWall again when the desired message is unchanged", func() {
			var setWallCalls int
			fake := &fakeClient{
				team:          &fakeTeam{name: "main"},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{Version: "7.11.0"}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
				setWallFn: func(_ atc.Wall) error {
					setWallCalls++
					return nil
				},
			}

			spec := testInstanceSpec()
			spec.Wall = &concoursev1alpha1.WallConfig{Message: "steady state"}

			nsn, reconciler, _ := setupWallInstance("instance-wall-idempotent", spec, fake)

			By("Second reconcile sets the wall the first time")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(setWallCalls).To(Equal(1))

			By("Third reconcile with the same spec message does not call SetWall again")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(setWallCalls).To(Equal(1))

			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.WallMessage).To(Equal("steady state"))
		})

		It("should call ClearWall and reset WallMessage when spec.Wall is removed", func() {
			var clearWallCalls int
			fake := &fakeClient{
				team:          &fakeTeam{name: "main"},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{Version: "7.11.0"}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
				clearWallFn: func() error {
					clearWallCalls++
					return nil
				},
			}

			spec := testInstanceSpec()
			spec.Wall = &concoursev1alpha1.WallConfig{Message: "will be cleared"}

			nsn, reconciler, cache := setupWallInstance("instance-wall-clear", spec, fake)

			By("Second reconcile sets the wall")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			By("Removing spec.Wall")
			latest := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, latest)).To(Succeed())
			latest.Spec.Wall = nil
			Expect(k8sClient.Update(ctx, latest)).To(Succeed())

			By("Re-fetch to get post-update ResourceVersion, then re-seed cache")
			afterUpdate := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, afterUpdate)).To(Succeed())
			cache.Set(afterUpdate, fake)

			By("Reconciling clears the wall")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(clearWallCalls).To(Equal(1))

			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.WallMessage).To(BeEmpty())
		})

		It("should set Ready=False and record a Warning event when SetWall fails", func() {
			fake := &fakeClient{
				team:          &fakeTeam{name: "main"},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{Version: "7.11.0"}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
				setWallFn: func(_ atc.Wall) error {
					return fmt.Errorf("wall endpoint unavailable")
				},
			}

			spec := testInstanceSpec()
			spec.Wall = &concoursev1alpha1.WallConfig{Message: "failing wall"}

			nsn, reconciler, _ := setupWallInstance("instance-wall-fail", spec, fake)

			recorder := record.NewFakeRecorder(10)
			reconciler.Recorder = recorder

			By("Reconciling fails to set the wall and requeues")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			fetched := &concoursev1alpha1.Instance{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.WallMessage).To(BeEmpty())

			readyCond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("WallFailed"))

			Expect(recorder.Events).To(Receive(ContainSubstring("WallFailed")))
		})
	})
})
