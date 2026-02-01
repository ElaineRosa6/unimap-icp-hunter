package scheduler

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// PolicyGenerator 策略生成器
type PolicyGenerator struct {
	config *SchedulerConfig
	logger *zap.Logger
}

// NewPolicyGenerator 创建策略生成器
func NewPolicyGenerator(config *SchedulerConfig, logger *zap.Logger) *PolicyGenerator {
	return &PolicyGenerator{
		config: config,
		logger: logger,
	}
}

// GenerateDailyPolicies 生成每日扫描策略
func (g *PolicyGenerator) GenerateDailyPolicies() []PolicyConfig {
	policies := []PolicyConfig{}

	// 从配置中读取策略
	for _, policy := range g.config.Policies {
		if policy.Enabled {
			policies = append(policies, policy)
		}
	}

	g.logger.Info("Generated daily policies", zap.Int("count", len(policies)))
	return policies
}

// GeneratePoliciesByRegion 按省份生成策略
func (g *PolicyGenerator) GeneratePoliciesByRegion(regions []string) []PolicyConfig {
	policies := []PolicyConfig{}

	for _, region := range regions {
		// 生成按省份的UQL
		uql := fmt.Sprintf(`country="CN" && region="%s" && port IN ["80", "443", "8080", "8443"]`, region)

		policy := PolicyConfig{
			Name:       fmt.Sprintf("region_%s", region),
			UQL:        uql,
			Engines:    []string{"fofa", "hunter"},
			PageSize:   100,
			MaxRecords: 5000,
			Ports:      []int{80, 443, 8080, 8443},
		}

		policies = append(policies, policy)
	}

	return policies
}

// GeneratePoliciesByPort 按端口生成策略
func (g *PolicyGenerator) GeneratePoliciesByPort(ports []int) []PolicyConfig {
	policies := []PolicyConfig{}

	for _, port := range ports {
		protocol := "http"
		if port == 443 {
			protocol = "https"
		}

		uql := fmt.Sprintf(`country="CN" && port="%d" && protocol="%s"`, port, protocol)

		policy := PolicyConfig{
			Name:       fmt.Sprintf("port_%d", port),
			UQL:        uql,
			Engines:    []string{"fofa", "hunter"},
			PageSize:   100,
			MaxRecords: 5000,
			Ports:      []int{port},
		}

		policies = append(policies, policy)
	}

	return policies
}

// GeneratePoliciesByIPRange 按IP段生成策略
func (g *PolicyGenerator) GeneratePoliciesByIPRange(ipRanges []string) []PolicyConfig {
	policies := []PolicyConfig{}

	for _, ipRange := range ipRanges {
		uql := fmt.Sprintf(`country="CN" && ip="%s" && port IN ["80", "443"]`, ipRange)

		policy := PolicyConfig{
			Name:       fmt.Sprintf("iprange_%s", ipRange),
			UQL:        uql,
			Engines:    []string{"fofa", "hunter"},
			PageSize:   100,
			MaxRecords: 2000,
			Ports:      []int{80, 443},
		}

		policies = append(policies, policy)
	}

	return policies
}

// GenerateIncrementalPolicy 生成增量扫描策略（基于时间）
func (g *PolicyGenerator) GenerateIncrementalPolicy(since time.Time) PolicyConfig {
	// 这里需要各引擎支持时间过滤
	// 简化实现
	uql := fmt.Sprintf(`country="CN" && port IN ["80", "443"]`)

	return PolicyConfig{
		Name:       fmt.Sprintf("incremental_%s", since.Format("20060102")),
		UQL:        uql,
		Engines:    []string{"fofa", "hunter"},
		PageSize:   100,
		MaxRecords: 10000,
		Ports:      []int{80, 443},
	}
}

// ValidatePolicy 验证策略配置
func (g *PolicyGenerator) ValidatePolicy(policy PolicyConfig) error {
	if policy.Name == "" {
		return fmt.Errorf("policy name cannot be empty")
	}

	if policy.UQL == "" {
		return fmt.Errorf("policy UQL cannot be empty")
	}

	if len(policy.Engines) == 0 {
		return fmt.Errorf("policy must specify at least one engine")
	}

	if policy.PageSize <= 0 {
		policy.PageSize = 100
	}

	if policy.MaxRecords <= 0 {
		policy.MaxRecords = 5000
	}

	return nil
}

// OptimizePolicies 优化策略（合并相似策略，避免重复）
func (g *PolicyGenerator) OptimizePolicies(policies []PolicyConfig) []PolicyConfig {
	if len(policies) <= 1 {
		return policies
	}

	optimized := []PolicyConfig{}
	seen := make(map[string]bool)

	for _, policy := range policies {
		// 使用UQL和引擎组合作为key去重
		key := policy.UQL + ":" + fmt.Sprint(policy.Engines)

		if !seen[key] {
			optimized = append(optimized, policy)
			seen[key] = true
		}
	}

	return optimized
}

// GeneratePriorityPolicies 生成优先级策略（按端口优先级）
func (g *PolicyGenerator) GeneratePriorityPolicies() []PolicyConfig {
	// 优先级：80, 443, 8080, 8443, 其他
	portPriorities := []struct {
		ports      []int
		priority   int
		pageSize   int
		maxRecords int
	}{
		{[]int{80, 443}, 1, 200, 10000},      // 高优先级
		{[]int{8080, 8443}, 2, 100, 5000},    // 中优先级
		{[]int{8000, 9000, 7000}, 3, 50, 2000}, // 低优先级
	}

	policies := []PolicyConfig{}
	for _, p := range portPriorities {
		// 构建IN查询
		portStrs := []string{}
		for _, port := range p.ports {
			portStrs = append(portStrs, fmt.Sprintf(`"%d"`, port))
		}

		uql := fmt.Sprintf(`country="CN" && port IN [%s]`, portStrs)

		policy := PolicyConfig{
			Name:       fmt.Sprintf("priority_%d", p.priority),
			UQL:        uql,
			Engines:    []string{"fofa", "hunter"},
			PageSize:   p.pageSize,
			MaxRecords: p.maxRecords,
			Ports:      p.ports,
		}

		policies = append(policies, policy)
	}

	return policies
}

// GenerateReportPolicies 生成报表统计策略
func (g *PolicyGenerator) GenerateReportPolicies() []PolicyConfig {
	// 按省份统计
	regions := []string{"北京", "上海", "广东", "浙江", "江苏", "山东", "福建", "四川", "湖北", "湖南"}
	policies := []PolicyConfig{}

	for _, region := range regions {
		uql := fmt.Sprintf(`country="CN" && region="%s" && port IN ["80", "443"]`, region)

		policy := PolicyConfig{
			Name:       fmt.Sprintf("report_%s", region),
			UQL:        uql,
			Engines:    []string{"fofa", "hunter"},
			PageSize:   50,
			MaxRecords: 2000,
			Ports:      []int{80, 443},
		}

		policies = append(policies, policy)
	}

	return policies
}
