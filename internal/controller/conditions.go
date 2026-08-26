/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/License-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// setCondition updates or appends a condition in the slice, recording the
// object's generation so status consumers can tell stale conditions apart.
func setCondition(conditions *[]metav1.Condition, generation int64, condType string, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}
	meta.SetStatusCondition(conditions, cond)
}

func int32Ptr(v int) *int32 {
	i := int32(v) //nolint:gosec
	return &i
}

func boolPtr(v bool) *bool {
	return &v
}

func ptrValue[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func recordEvent(recorder record.EventRecorder, object runtime.Object, eventtype, reason, message string) {
	if recorder != nil {
		recorder.Event(object, eventtype, reason, message)
	}
}

func recordEventf(recorder record.EventRecorder, object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	if recorder != nil {
		recorder.Eventf(object, eventtype, reason, messageFmt, args...)
	}
}
