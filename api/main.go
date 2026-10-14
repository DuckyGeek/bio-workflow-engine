// Package main 是生物信息工作流引擎的 REST API 服务入口
package main

import (
	"context"
	"embed"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ============================================================
// 嵌入静态文件（前端页面）
// ============================================================

//go:embed static/*
var staticFiles embed.FS

// ============================================================
// 用户认证相关
// ============================================================

// 内存存储用户数据（生产环境应该用数据库）
var (
	users     = make(map[string]string) // username -> password (明文，生产环境应加密)
	usersMu   sync.RWMutex
	jwtSecret = []byte("bio-workflow-secret-key-change-in-production")
)

// UserCredentials 注册/登录请求
type UserCredentials struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Claims JWT 声明
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// generateToken 生成 JWT Token
func generateToken(username string) (string, error) {
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "bio-workflow-engine",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// validateToken 验证 JWT Token
func validateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// authMiddleware JWT 认证中间件
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Header 获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// 格式: Bearer <token>
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format, expected: Bearer <token>"})
			c.Abort()
			return
		}

		claims, err := validateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token: " + err.Error()})
			c.Abort()
			return
		}

		// 将用户名存入上下文
		c.Set("username", claims.Username)
		c.Next()
	}
}

// ============================================================
// 主函数
// ============================================================

func main() {
	// 初始化测试用户（方便测试）
	usersMu.Lock()
	users["admin"] = "admin123"
	users["test"] = "test123"
	usersMu.Unlock()

	// =====================================================
	// 初始化 Kubernetes 客户端
	// =====================================================

	var config *rest.Config
	var err error

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Fatalf("Failed to get Kubernetes config: %v", err)
		}
	}

	// 创建动态客户端（用于操作 CRD）
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create dynamic client: %v", err)
	}

	// 创建标准客户端（用于获取 Pod 日志）
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create clientset: %v", err)
	}

	bioWorkflowGVR := schema.GroupVersionResource{
		Group:    "workflow.example.com",
		Version:  "v1",
		Resource: "bioworkflows",
	}

	// =====================================================
	// 设置 Gin 路由
	// =====================================================

	router := gin.Default()

	// ============================================================
	// 前端页面（无需认证）
	// ============================================================

	router.StaticFS("/static", http.FS(staticFiles))
	router.GET("/", func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			c.String(http.StatusNotFound, "Page not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// ============================================================
	// 健康检查（无需认证）
	// ============================================================

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "bio-workflow-engine-api",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	// ============================================================
	// 认证路由（无需认证）
	// ============================================================

	// 用户注册
	router.POST("/register", func(c *gin.Context) {
		var creds UserCredentials
		if err := c.ShouldBindJSON(&creds); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
			return
		}

		usersMu.Lock()
		defer usersMu.Unlock()

		if _, exists := users[creds.Username]; exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
			return
		}

		users[creds.Username] = creds.Password
		c.JSON(http.StatusCreated, gin.H{
			"message":  "User registered successfully",
			"username": creds.Username,
		})
	})

	// 用户登录
	router.POST("/login", func(c *gin.Context) {
		var creds UserCredentials
		if err := c.ShouldBindJSON(&creds); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
			return
		}

		usersMu.RLock()
		password, exists := users[creds.Username]
		usersMu.RUnlock()

		if !exists || password != creds.Password {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
			return
		}

		token, err := generateToken(creds.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":    token,
			"username": creds.Username,
			"expires":  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})
	})

	// ============================================================
	// 工作流 API（需要认证）
	// ============================================================

	auth := router.Group("/api")
	auth.Use(authMiddleware())
	{
		// POST /api/workflows - 创建工作流
		auth.POST("/workflows", func(c *gin.Context) {
			var req CreateWorkflowRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
				return
			}

			if req.Name == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
				return
			}
			if len(req.Steps) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "at least one step is required"})
				return
			}

			bioWorkflow := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "workflow.example.com/v1",
					"kind":       "BioWorkflow",
					"metadata": map[string]interface{}{
						"name": req.Name,
					},
					"spec": map[string]interface{}{
						"steps": req.Steps,
					},
				},
			}

			ctx := context.Background()
			result, err := dynamicClient.Resource(bioWorkflowGVR).
				Namespace("default").
				Create(ctx, bioWorkflow, metav1.CreateOptions{})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create workflow: " + err.Error()})
				return
			}

			c.JSON(http.StatusCreated, gin.H{
				"message":  "Workflow created successfully",
				"workflow": result.Object,
			})
		})

		// GET /api/workflows - 列出所有工作流
		auth.GET("/workflows", func(c *gin.Context) {
			ctx := context.Background()
			list, err := dynamicClient.Resource(bioWorkflowGVR).
				Namespace("default").
				List(ctx, metav1.ListOptions{})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list workflows: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"workflows": list.Items,
				"total":     len(list.Items),
			})
		})

		// GET /api/workflows/:name - 获取单个工作流详情
		auth.GET("/workflows/:name", func(c *gin.Context) {
			name := c.Param("name")
			ctx := context.Background()

			result, err := dynamicClient.Resource(bioWorkflowGVR).
				Namespace("default").
				Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"workflow": result.Object,
			})
		})

		// DELETE /api/workflows/:name - 删除工作流
		auth.DELETE("/api/workflows/:name", func(c *gin.Context) {
			name := c.Param("name")
			ctx := context.Background()

			err := dynamicClient.Resource(bioWorkflowGVR).
				Namespace("default").
				Delete(ctx, name, metav1.DeleteOptions{})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete workflow: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"message": "Workflow deleted successfully",
				"name":    name,
			})
		})

		// ============================================================
		// GET /api/workflows/:name/logs - 获取工作流日志（完整版）
		// ============================================================
		auth.GET("/workflows/:name/logs", func(c *gin.Context) {
			name := c.Param("name")
			ctx := context.Background()

			// 获取 BioWorkflow
			result, err := dynamicClient.Resource(bioWorkflowGVR).
				Namespace("default").
				Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found: " + err.Error()})
				return
			}

			// 提取 stepStatus
			status, found, _ := unstructured.NestedMap(result.Object, "status")
			if !found {
				c.JSON(http.StatusOK, gin.H{"logs": map[string]string{}})
				return
			}
			stepStatus, found, _ := unstructured.NestedStringMap(status, "stepStatus")
			if !found {
				c.JSON(http.StatusOK, gin.H{"logs": map[string]string{}})
				return
			}

			logs := make(map[string]string)

			for stepName := range stepStatus {
				// 查找对应的 Pod
				pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
					LabelSelector: "workflow-name=" + name + ",step-name=" + stepName,
				})
				if err != nil {
					logs[stepName] = "Error: " + err.Error()
					continue
				}

				if len(pods.Items) == 0 {
					logs[stepName] = "No pod found (step may not have started yet)"
					continue
				}

				// 获取第一个 Pod 的日志
				podName := pods.Items[0].Name
				req := clientset.CoreV1().Pods("default").GetLogs(podName, &corev1.PodLogOptions{})
				logStream, err := req.Stream(ctx)
				if err != nil {
					logs[stepName] = "Error getting logs: " + err.Error()
					continue
				}
				defer logStream.Close()

				// 读取日志内容
				buf := new(strings.Builder)
				_, err = io.Copy(buf, logStream)
				if err != nil {
					logs[stepName] = "Error reading logs: " + err.Error()
					continue
				}
				logContent := buf.String()
				if logContent == "" {
					logs[stepName] = "(no logs yet)"
				} else {
					logs[stepName] = logContent
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"workflow": name,
				"logs":     logs,
			})
		})

		// ============================================================
		// GET /api/workflows/:name/result - 下载结果文件
		// ============================================================
		auth.GET("/workflows/:name/result", func(c *gin.Context) {
			name := c.Param("name")
			ctx := context.Background()

			// 获取 BioWorkflow
			result, err := dynamicClient.Resource(bioWorkflowGVR).
				Namespace("default").
				Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found: " + err.Error()})
				return
			}

			// 提取工作流数据
			spec, _, _ := unstructured.NestedMap(result.Object, "spec")
			status, _, _ := unstructured.NestedMap(result.Object, "status")
			metadata, _, _ := unstructured.NestedMap(result.Object, "metadata")

			// 构建结果报告
			steps := []interface{}{}
			if spec != nil {
				if s, ok := spec["steps"].([]interface{}); ok {
					steps = s
				}
			}

			stepStatus := map[string]string{}
			if status != nil {
				if ss, ok := status["stepStatus"].(map[string]interface{}); ok {
					for k, v := range ss {
						if vs, ok := v.(string); ok {
							stepStatus[k] = vs
						}
					}
				}
			}

			phase := ""
			if status != nil {
				if p, ok := status["phase"].(string); ok {
					phase = p
				}
			}

			report := map[string]interface{}{
				"workflow":     name,
				"status":       phase,
				"generated_at": time.Now().Format(time.RFC3339),
				"metadata":     metadata,
				"steps":        steps,
				"step_status":  stepStatus,
				"summary": map[string]int{
					"total":     len(steps),
					"completed": countStatus(stepStatus, "Completed"),
					"running":   countStatus(stepStatus, "Running"),
					"pending":   countStatus(stepStatus, "Pending"),
					"failed":    countStatus(stepStatus, "Failed"),
				},
			}

			// 返回 JSON 文件
			c.Header("Content-Disposition", "attachment; filename=workflow-"+name+"-result.json")
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusOK, report)
		})
	}

	// =====================================================
	// 启动服务
	// =====================================================

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("API server starting on port %s", port)
	log.Printf("Web UI: http://localhost:%s", port)
	log.Printf("Default users: admin/admin123, test/test123")
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// ============================================================
// 辅助函数
// ============================================================

func countStatus(stepStatus map[string]string, status string) int {
	count := 0
	for _, v := range stepStatus {
		if v == status {
			count++
		}
	}
	return count
}

// ============================================================
// 请求/响应结构体定义
// ============================================================

type CreateWorkflowRequest struct {
	Name  string         `json:"name"`
	Steps []WorkflowStep `json:"steps"`
}

type WorkflowStep struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Command   []string `json:"command"`
	DependsOn []string `json:"dependsOn,omitempty"`
}
