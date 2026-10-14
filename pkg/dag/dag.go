package dag

import (
	"fmt"
)

// Step 表示DAG中的一个节点
type Step struct {
	Name      string
	DependsOn []string
}

// TopologicalSort 对步骤列表进行拓扑排序
// 返回：按执行顺序排列的步骤名列表
// 如果存在循环依赖，返回错误
func TopologicalSort(steps []Step) ([]string, error) {
	// 1. 构建步骤名到步骤的映射（快速查找）
	stepMap := make(map[string]Step)
	for _, s := range steps {
		stepMap[s.Name] = s
	}

	// 2. 计算每个节点的入度（依赖数）
	// 入度 = 有多少个前置步骤依赖它
	indegree := make(map[string]int)
	for _, s := range steps {
		indegree[s.Name] = len(s.DependsOn)
	}

	// 3. 找到所有入度为0的节点（没有依赖，可以立即执行）
	queue := []string{}
	for name, degree := range indegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// 4. Kahn算法：不断处理入度为0的节点
	result := []string{}
	processed := make(map[string]bool)

	for len(queue) > 0 {
		// 取出第一个节点
		current := queue[0]
		queue = queue[1:]

		result = append(result, current)
		processed[current] = true

		// 遍历所有步骤，找到依赖 current 的节点
		for _, s := range steps {
			// 检查 s 是否依赖 current
			for _, dep := range s.DependsOn {
				if dep == current {
					// 减少被依赖节点的入度
					indegree[s.Name]--
					// 如果入度变为0，加入队列
					if indegree[s.Name] == 0 {
						// 确保不会重复添加
						found := false
						for _, q := range queue {
							if q == s.Name {
								found = true
								break
							}
						}
						if !found {
							queue = append(queue, s.Name)
						}
					}
				}
			}
		}
	}

	// 5. 检查是否有未处理的节点（存在循环依赖）
	if len(result) != len(steps) {
		remaining := []string{}
		for _, s := range steps {
			if !processed[s.Name] {
				remaining = append(remaining, s.Name)
			}
		}
		return nil, fmt.Errorf("circular dependency detected: %v", remaining)
	}

	return result, nil
}

// ValidateDAG 验证步骤列表是否构成有效的DAG
func ValidateDAG(steps []Step) error {
	_, err := TopologicalSort(steps)
	return err
}
