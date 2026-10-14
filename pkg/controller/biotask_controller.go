package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workflowv1 "github.com/yourusername/bio-workflow-engine/pkg/apis/v1"
)

// BioTaskController 是BioTask的控制器
type BioTaskController struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile 是控制器的核心调谐函数
func (r *BioTaskController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Printf("Reconciling BioTask: %s/%s", req.Namespace, req.Name)

	// 1. 获取BioTask对象
	var bioTask workflowv1.BioTask
	if err := r.Get(ctx, req.NamespacedName, &bioTask); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. 如果任务已完成，不再处理
	if bioTask.Status.Phase == "Completed" || bioTask.Status.Phase == "Failed" {
		return ctrl.Result{}, nil
	}

	// 3. 初始化状态
	if bioTask.Status.Phase == "" {
		bioTask.Status.Phase = "Pending"
		if err := r.Status().Update(ctx, &bioTask); err != nil {
			return ctrl.Result{}, err
		}
		log.Printf("BioTask %s initialized to Pending", bioTask.Name)
		return ctrl.Result{}, nil
	}

	// 4. 检查是否存在对应的Job
	jobName := fmt.Sprintf("biotask-%s", bioTask.Name)
	var job batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Namespace: bioTask.Namespace, Name: jobName}, &job)

	// 使用 IsNotFound 检查
	if err != nil && client.IgnoreNotFound(err) != nil {
		// 如果是真正的错误，返回
		return ctrl.Result{}, err
	}

	if err != nil && client.IgnoreNotFound(err) == nil {
		// 5. 没有Job，创建一个
		log.Printf("Creating Job %s for BioTask %s", jobName, bioTask.Name)
		job = r.buildJob(&bioTask, jobName)

		// 设置OwnerReference（删除BioTask时自动删除Job）
		if err := controllerutil.SetControllerReference(&bioTask, &job, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, &job); err != nil {
			return ctrl.Result{}, err
		}

		// 更新状态为Running
		bioTask.Status.Phase = "Running"
		bioTask.Status.JobName = jobName
		bioTask.Status.StartTime = time.Now().Format(time.RFC3339)
		if err := r.Status().Update(ctx, &bioTask); err != nil {
			return ctrl.Result{}, err
		}

		r.Recorder.Event(&bioTask, corev1.EventTypeNormal, "JobCreated", "Successfully created job "+jobName)
		return ctrl.Result{}, nil
	}

	// 6. Job已存在，检查状态
	if job.Status.Succeeded > 0 {
		bioTask.Status.Phase = "Completed"
		bioTask.Status.CompletionTime = time.Now().Format(time.RFC3339)
		r.Status().Update(ctx, &bioTask)
		r.Recorder.Event(&bioTask, corev1.EventTypeNormal, "Completed", "Task completed successfully")
		log.Printf("BioTask %s completed", bioTask.Name)
	} else if job.Status.Failed > 0 {
		bioTask.Status.Phase = "Failed"
		r.Status().Update(ctx, &bioTask)
		r.Recorder.Event(&bioTask, corev1.EventTypeWarning, "Failed", "Task failed")
		log.Printf("BioTask %s failed", bioTask.Name)
	}

	return ctrl.Result{}, nil
}

// buildJob 构建Kubernetes Job
func (r *BioTaskController) buildJob(bioTask *workflowv1.BioTask, jobName string) batchv1.Job {
	backoffLimit := int32(0)

	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: bioTask.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "bio-workflow-engine",
				"biotask-name":                 bioTask.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"biotask-name": bioTask.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "bio-task",
							Image:   bioTask.Spec.Image,
							Command: bioTask.Spec.Command,
							Args:    bioTask.Spec.Args,
						},
					},
				},
			},
		},
	}
}

// SetupWithManager 注册控制器到Manager
func (r *BioTaskController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workflowv1.BioTask{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
