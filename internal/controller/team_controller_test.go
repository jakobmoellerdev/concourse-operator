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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

var _ = Describe("Team Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-team"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		team := &concoursev1alpha1.Team{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Team")
			err := k8sClient.Get(ctx, typeNamespacedName, team)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Team{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.TeamSpec{
						InstanceRef: concoursev1alpha1.LocalObjectReference{Name: "test-instance"},
						TeamName:    "test-team",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Team{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Team")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when instance is missing", func() {
			By("Reconciling the created resource")
			reconciler := &TeamReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add the team finalizer on first reconcile", func() {
			By("Creating a fresh team for this test")
			finalizerTeam := &concoursev1alpha1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "test-team-finalizer", Namespace: "default"},
				Spec: concoursev1alpha1.TeamSpec{
					InstanceRef: concoursev1alpha1.LocalObjectReference{Name: "test-instance"},
				},
			}
			Expect(k8sClient.Create(ctx, finalizerTeam)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Team{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-team-finalizer", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, teamFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &TeamReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			finalizerNSN := types.NamespacedName{Name: "test-team-finalizer", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: finalizerNSN})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the finalizer was added")
			fetched := &concoursev1alpha1.Team{}
			Expect(k8sClient.Get(ctx, finalizerNSN, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, teamFinalizer)).To(BeTrue())
		})

		It("should set Ready=False when instance exists but is not ready", func() {
			By("Creating an instance without Ready condition")
			inst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance-not-ready", Namespace: "default"},
				Spec:       concoursev1alpha1.InstanceSpec{URL: "https://ci.example.com"},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })

			By("Creating a team referencing the not-ready instance")
			notReadyTeam := &concoursev1alpha1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "team-instance-not-ready", Namespace: "default"},
				Spec: concoursev1alpha1.TeamSpec{
					InstanceRef: concoursev1alpha1.LocalObjectReference{Name: "test-instance-not-ready"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyTeam)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyTeam) })

			reconciler := &TeamReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "team-instance-not-ready", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False is set")
			fetched := &concoursev1alpha1.Team{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "team-instance-not-ready", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})
