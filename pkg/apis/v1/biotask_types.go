package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// 定义 GroupVersion
var SchemeGroupVersion = schema.GroupVersion{
	Group:   "workflow.example.com",
	Version: "v1",
}

// AddToScheme 将类型注册到Scheme
func AddToScheme(scheme *runtime.Scheme) error {
	// 注册 BioTask
	scheme.AddKnownTypes(SchemeGroupVersion, &BioTask{}, &BioTaskList{})
	// 注册 BioWorkflow
	scheme.AddKnownTypes(SchemeGroupVersion, &BioWorkflow{}, &BioWorkflowList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// DeepCopyInto 实现 runtime.Object 接口
func (in *BioTask) DeepCopyInto(out *BioTask) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy 实现 runtime.Object 接口
func (in *BioTask) DeepCopy() *BioTask {
	if in == nil {
		return nil
	}
	out := new(BioTask)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject 实现 runtime.Object 接口
func (in *BioTask) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto 实现 runtime.Object 接口
func (in *BioTaskStatus) DeepCopyInto(out *BioTaskStatus) {
	*out = *in
}

// DeepCopy 实现 runtime.Object 接口
func (in *BioTaskList) DeepCopyInto(out *BioTaskList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BioTask, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *BioTaskList) DeepCopy() *BioTaskList {
	if in == nil {
		return nil
	}
	out := new(BioTaskList)
	in.DeepCopyInto(out)
	return out
}

func (in *BioTaskList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// GetGroupVersionKind 返回资源的GVK
func (in *BioTask) GetObjectKind() schema.ObjectKind {
	return in
}

// GetGroupVersionKind 返回资源的GVK
func (in *BioTaskList) GetObjectKind() schema.ObjectKind {
	return in
}

// BioTaskSpec 定义了用户的期望状态
type BioTaskSpec struct {
	Image   string   `json:"image,omitempty"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// BioTaskStatus 定义了任务的实际状态
type BioTaskStatus struct {
	Phase          string `json:"phase,omitempty"`
	JobName        string `json:"jobName,omitempty"`
	StartTime      string `json:"startTime,omitempty"`
	CompletionTime string `json:"completionTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BioTask 是自定义资源
type BioTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BioTaskSpec   `json:"spec,omitempty"`
	Status BioTaskStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BioTaskList 是BioTask列表
type BioTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BioTask `json:"items"`
}
