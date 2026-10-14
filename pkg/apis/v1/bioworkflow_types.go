package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ========== DeepCopy 方法 ==========

func (in *BioWorkflow) DeepCopyInto(out *BioWorkflow) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *BioWorkflow) DeepCopy() *BioWorkflow {
	if in == nil {
		return nil
	}
	out := new(BioWorkflow)
	in.DeepCopyInto(out)
	return out
}

func (in *BioWorkflow) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *BioWorkflowSpec) DeepCopyInto(out *BioWorkflowSpec) {
	*out = *in
	if in.Steps != nil {
		in, out := &in.Steps, &out.Steps
		*out = make([]WorkflowStep, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *BioWorkflowSpec) DeepCopy() *BioWorkflowSpec {
	if in == nil {
		return nil
	}
	out := new(BioWorkflowSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *WorkflowStep) DeepCopyInto(out *WorkflowStep) {
	*out = *in
	if in.DependsOn != nil {
		in, out := &in.DependsOn, &out.DependsOn
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *WorkflowStep) DeepCopy() *WorkflowStep {
	if in == nil {
		return nil
	}
	out := new(WorkflowStep)
	in.DeepCopyInto(out)
	return out
}

func (in *BioWorkflowStatus) DeepCopyInto(out *BioWorkflowStatus) {
	*out = *in
	if in.StepStatus != nil {
		in, out := &in.StepStatus, &out.StepStatus
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

func (in *BioWorkflowStatus) DeepCopy() *BioWorkflowStatus {
	if in == nil {
		return nil
	}
	out := new(BioWorkflowStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *BioWorkflowList) DeepCopyInto(out *BioWorkflowList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BioWorkflow, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *BioWorkflowList) DeepCopy() *BioWorkflowList {
	if in == nil {
		return nil
	}
	out := new(BioWorkflowList)
	in.DeepCopyInto(out)
	return out
}

func (in *BioWorkflowList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// ========== 类型定义 ==========

// BioWorkflowSpec 定义了工作流的期望状态
type BioWorkflowSpec struct {
	Steps []WorkflowStep `json:"steps"`
}

// WorkflowStep 定义了一个步骤
type WorkflowStep struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Command   []string `json:"command"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

// BioWorkflowStatus 定义了工作流的实际状态
type BioWorkflowStatus struct {
	Phase          string            `json:"phase,omitempty"`
	StepStatus     map[string]string `json:"stepStatus,omitempty"`
	StartTime      string            `json:"startTime,omitempty"`
	CompletionTime string            `json:"completionTime,omitempty"`
}

// BioWorkflow 是工作流自定义资源
type BioWorkflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BioWorkflowSpec   `json:"spec,omitempty"`
	Status            BioWorkflowStatus `json:"status,omitempty"`
}

// BioWorkflowList 是BioWorkflow列表
type BioWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BioWorkflow `json:"items"`
}
