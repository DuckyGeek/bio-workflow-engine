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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	workflowv1 "github.com/yourusername/bio-workflow-engine/pkg/apis/v1"
	"github.com/yourusername/bio-workflow-engine/pkg/dag"
)

// Prometheus 指标定义
var (
	workflowsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bioworkflow_workflows_total",
			Help: "Total number of BioWorkflows processed, by phase",
		},
		[]string{"phase"},
	)

	workflowsRunning = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "bioworkflow_workflows_running",
			Help: "Current number of running BioWorkflows",
		},
	)

	stepDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "bioworkflow_step_duration_seconds",
			Help:    "Duration of each step execution in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	pendingSteps = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "bioworkflow_pending_steps",
			Help: "Number of steps pending execution",
		},
	)
)

type BioWorkflowController struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (r *BioWorkflowController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Printf("===== Reconcile called for BioWorkflow: %s/%s =====", req.Namespace, req.Name)

	var workflow workflowv1.BioWorkflow
	if err := r.Get(ctx, req.NamespacedName, &workflow); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if workflow.Status.Phase == "Completed" || workflow.Status.Phase == "Failed" {
		log.Printf("Workflow already completed/failed, skipping")
		return ctrl.Result{}, nil
	}

	oldPhase := workflow.Status.Phase

	if workflow.Status.Phase == "" {
		log.Printf("Initializing workflow...")
		workflow.Status.Phase = "Pending"
		workflow.Status.StepStatus = make(map[string]string)
		for _, step := range workflow.Spec.Steps {
			workflow.Status.StepStatus[step.Name] = "Pending"
		}
		workflow.Status.StartTime = time.Now().Format(time.RFC3339)
		if err := r.Status().Update(ctx, &workflow); err != nil {
			return ctrl.Result{}, err
		}
		log.Printf("Workflow %s initialized with %d steps", workflow.Name, len(workflow.Spec.Steps))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	steps := make([]dag.Step, len(workflow.Spec.Steps))
	for i, s := range workflow.Spec.Steps {
		steps[i] = dag.Step{
			Name:      s.Name,
			DependsOn: s.DependsOn,
		}
	}
	order, err := dag.TopologicalSort(steps)
	if err != nil {
		log.Printf("DAG validation failed: %v", err)
		workflow.Status.Phase = "Failed"
		_ = r.Status().Update(ctx, &workflow)

		workflowsTotal.WithLabelValues("Failed").Inc()
		if oldPhase == "Running" {
			workflowsRunning.Dec()
		}
		return ctrl.Result{}, err
	}
	log.Printf("Execution order: %v", order)

	allCompleted := true

	for _, stepName := range order {
		stepStatus, exists := workflow.Status.StepStatus[stepName]
		if !exists {
			workflow.Status.StepStatus[stepName] = "Pending"
			stepStatus = "Pending"
		}

		log.Printf("Step %s: status = %s", stepName, stepStatus)

		switch stepStatus {
		case "Pending":
			depsCompleted := true
			var stepDef *workflowv1.WorkflowStep

			for i := range workflow.Spec.Steps {
				if workflow.Spec.Steps[i].Name == stepName {
					stepDef = &workflow.Spec.Steps[i]
					break
				}
			}
			if stepDef == nil {
				log.Printf("Step %s not found in spec", stepName)
				continue
			}

			log.Printf("Step %s depends on: %v", stepName, stepDef.DependsOn)
			for _, dep := range stepDef.DependsOn {
				depStatus, exists := workflow.Status.StepStatus[dep]
				if !exists || depStatus != "Completed" {
					log.Printf("Dependency %s not completed yet (status: %s)", dep, depStatus)
					depsCompleted = false
					allCompleted = false
					break
				}
			}

			if depsCompleted {
				log.Printf("Starting step: %s", stepName)
				jobName := fmt.Sprintf("%s-%s", workflow.Name, stepName)

				job := r.buildJob(&workflow, stepDef, jobName)

				if err := controllerutil.SetControllerReference(&workflow, &job, r.Scheme); err != nil {
					return ctrl.Result{}, err
				}

				if err := r.Create(ctx, &job); err != nil {
					return ctrl.Result{}, err
				}

				workflow.Status.StepStatus[stepName] = "Running"
				workflow.Status.Phase = "Running"
				if err := r.Status().Update(ctx, &workflow); err != nil {
					return ctrl.Result{}, err
				}
				r.Recorder.Event(&workflow, corev1.EventTypeNormal, "StepStarted", "Started step "+stepName)
				log.Printf("Step %s started, Job %s created", stepName, jobName)

				if oldPhase != "Running" && workflow.Status.Phase == "Running" {
					workflowsRunning.Inc()
					workflowsTotal.WithLabelValues("Running").Inc()
				}

				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			allCompleted = false

		case "Running":
			jobName := fmt.Sprintf("%s-%s", workflow.Name, stepName)
			var job batchv1.Job
			err := r.Get(ctx, types.NamespacedName{Namespace: workflow.Namespace, Name: jobName}, &job)
			if err != nil {
				log.Printf("Job %s not found, may need to requeue", jobName)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			if job.Status.Succeeded > 0 {
				log.Printf("Step %s completed (Job succeeded)", stepName)

				if job.CreationTimestamp.Time.IsZero() {
					log.Printf("Warning: Job %s has no creation timestamp, skipping duration recording", jobName)
				} else {
					duration := time.Since(job.CreationTimestamp.Time).Seconds()
					stepDuration.Observe(duration)
					log.Printf("Step %s duration: %.2f seconds", stepName, duration)
				}

				workflow.Status.StepStatus[stepName] = "Completed"
				if err := r.Status().Update(ctx, &workflow); err != nil {
					return ctrl.Result{}, err
				}
				r.Recorder.Event(&workflow, corev1.EventTypeNormal, "StepCompleted", "Completed step "+stepName)

			} else if job.Status.Failed > 0 {
				log.Printf("Step %s failed (Job failed)", stepName)
				workflow.Status.StepStatus[stepName] = "Failed"
				workflow.Status.Phase = "Failed"
				_ = r.Status().Update(ctx, &workflow)
				r.Recorder.Event(&workflow, corev1.EventTypeWarning, "StepFailed", "Failed step "+stepName)

				workflowsTotal.WithLabelValues("Failed").Inc()
				if oldPhase == "Running" {
					workflowsRunning.Dec()
				}
				return ctrl.Result{}, nil
			}
			allCompleted = false

		case "Completed":
			continue

		case "Failed":
			workflow.Status.Phase = "Failed"
			_ = r.Status().Update(ctx, &workflow)

			workflowsTotal.WithLabelValues("Failed").Inc()
			if oldPhase == "Running" {
				workflowsRunning.Dec()
			}
			return ctrl.Result{}, nil
		}
	}

	// 计算待执行步骤数
	pending := 0
	for _, stepName := range order {
		if workflow.Status.StepStatus[stepName] == "Pending" {
			pending++
		}
	}
	pendingSteps.Set(float64(pending))

	// ============================================================
	// 4.4 模拟弹性伸缩逻辑
	// ============================================================
	const threshold = 3 // 当待执行步骤数超过 3 时，触发扩容信号
	if pending > threshold {
		log.Printf("⚠️ [SCALE-UP] Pending steps (%d) exceeds threshold (%d). "+
			"Triggering scale-up! (Would add more worker nodes in production)", pending, threshold)
		// 在实际生产环境中，这里会：
		// 1. 调用 KEDA API 触发 ScaledObject 的伸缩
		// 2. 或者直接修改 Deployment 的 replicas 字段
		// 3. 或者调用云厂商的 API 增加节点
	} else {
		log.Printf("✅ [SCALE-STABLE] Pending steps: %d, no scaling needed (threshold: %d)", pending, threshold)
	}
	// ============================================================

	if allCompleted {
		log.Printf("All steps completed!")
		workflow.Status.Phase = "Completed"
		workflow.Status.CompletionTime = time.Now().Format(time.RFC3339)
		if err := r.Status().Update(ctx, &workflow); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&workflow, corev1.EventTypeNormal, "WorkflowCompleted", "All steps completed")
		log.Printf("BioWorkflow %s completed", workflow.Name)

		workflowsTotal.WithLabelValues("Completed").Inc()
		if oldPhase == "Running" {
			workflowsRunning.Dec()
		}
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *BioWorkflowController) buildJob(workflow *workflowv1.BioWorkflow, step *workflowv1.WorkflowStep, jobName string) batchv1.Job {
	backoffLimit := int32(0)

	volumes := []corev1.Volume{
		{
			Name: "workflow-data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "workflow-data",
					ReadOnly:  false,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "workflow-data",
			MountPath: "/workspace/data",
			ReadOnly:  false,
		},
	}

	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: workflow.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "bio-workflow-engine",
				"workflow-name":                workflow.Name,
				"step-name":                    step.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"workflow-name": workflow.Name,
						"step-name":     step.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       volumes,
					Containers: []corev1.Container{
						{
							Name:         "step",
							Image:        step.Image,
							Command:      step.Command,
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}
}

func (r *BioWorkflowController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workflowv1.BioWorkflow{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
