package priority

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// PriorityManager 规则优先级管理器
type PriorityManager struct {
	rules          map[string]*RulePriority
	ruleGroups     map[string][]string
	conflicts      map[string]ConflictInfo
	mu             sync.RWMutex
}

// RulePriority 规则优先级信息
type RulePriority struct {
	RuleID        string
	Priority      int
	Group         string
	Dependencies  []string
	Exclusions    []string
	Enabled       bool
}

// ConflictInfo 冲突信息
type ConflictInfo struct {
	RuleID        string
	ConflictsWith []string
	Resolution    string
}

// NewPriorityManager 创建优先级管理器
func NewPriorityManager() *PriorityManager {
	return &PriorityManager{
		rules:      make(map[string]*RulePriority),
		ruleGroups: make(map[string][]string),
		conflicts:  make(map[string]ConflictInfo),
	}
}

// AddRule 添加规则优先级
func (pm *PriorityManager) AddRule(rule *RulePriority) error {
	if rule.RuleID == "" {
		return errors.New("rule ID cannot be empty")
	}
	
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.rules[rule.RuleID] = rule
	
	// 添加到组
	if rule.Group != "" {
		pm.ruleGroups[rule.Group] = append(pm.ruleGroups[rule.Group], rule.RuleID)
	}
	
	// 检查冲突
	pm.checkConflicts(rule)
	
	return nil
}

// UpdateRule 更新规则优先级
func (pm *PriorityManager) UpdateRule(rule *RulePriority) error {
	if rule.RuleID == "" {
		return errors.New("rule ID cannot be empty")
	}
	
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// 检查规则是否存在
	if _, exists := pm.rules[rule.RuleID]; !exists {
		return fmt.Errorf("rule %s not found", rule.RuleID)
	}
	
	// 从旧组中移除
	if oldRule, exists := pm.rules[rule.RuleID]; exists && oldRule.Group != "" {
		pm.removeFromGroup(oldRule.Group, rule.RuleID)
	}
	
	// 更新规则
	pm.rules[rule.RuleID] = rule
	
	// 添加到新组
	if rule.Group != "" {
		pm.ruleGroups[rule.Group] = append(pm.ruleGroups[rule.Group], rule.RuleID)
	}
	
	// 重新检查冲突
	delete(pm.conflicts, rule.RuleID)
	pm.checkConflicts(rule)
	
	return nil
}

// RemoveRule 移除规则
func (pm *PriorityManager) RemoveRule(ruleID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	rule, exists := pm.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule %s not found", ruleID)
	}
	
	// 从组中移除
	if rule.Group != "" {
		pm.removeFromGroup(rule.Group, ruleID)
	}
	
	// 移除冲突信息
	delete(pm.conflicts, ruleID)
	
	// 移除规则
	delete(pm.rules, ruleID)
	
	return nil
}

// GetRule 获取规则优先级
func (pm *PriorityManager) GetRule(ruleID string) (*RulePriority, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	rule, exists := pm.rules[ruleID]
	return rule, exists
}

// GetRulesByPriority 按优先级获取规则
func (pm *PriorityManager) GetRulesByPriority() []*RulePriority {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	rules := make([]*RulePriority, 0, len(pm.rules))
	for _, rule := range pm.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	
	// 按优先级排序（优先级数字越小，优先级越高）
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority< rules[j].Priority
	})
	
	return rules
}

// GetRulesByGroup 获取组内规则
func (pm *PriorityManager) GetRulesByGroup(group string) []*RulePriority {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	ruleIDs, exists := pm.ruleGroups[group]
	if !exists {
		return []*RulePriority{}
	}
	
	rules := make([]*RulePriority, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		if rule, exists := pm.rules[ruleID]; exists && rule.Enabled {
			rules = append(rules, rule)
		}
	}
	
	// 按优先级排序
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})
	
	return rules
}

// GetAllRules 获取所有规则
func (pm *PriorityManager) GetAllRules() []*RulePriority {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	rules := make([]*RulePriority, 0, len(pm.rules))
	for _, rule := range pm.rules {
		rules = append(rules, rule)
	}
	
	return rules
}

// checkConflicts 检查规则冲突
func (pm *PriorityManager) checkConflicts(rule *RulePriority) {
	// 检查排除规则
	for _, excludedRuleID := range rule.Exclusions {
		if excludedRule, exists := pm.rules[excludedRuleID]; exists {
			pm.addConflict(rule.RuleID, excludedRuleID, "exclusion")
			
			// 如果排除规则的优先级更高，也添加冲突
			if excludedRule.Priority < rule.Priority {
				pm.addConflict(excludedRuleID, rule.RuleID, "exclusion")
			}
		}
	}
	
	// 检查依赖规则
	for _, dependencyRuleID := range rule.Dependencies {
		if _, exists := pm.rules[dependencyRuleID]; !exists {
			pm.addConflict(rule.RuleID, dependencyRuleID, "missing_dependency")
		}
	}
	
	// 检查同组规则冲突
	if rule.Group != "" {
		for _, ruleID := range pm.ruleGroups[rule.Group] {
			if ruleID != rule.RuleID {
				otherRule := pm.rules[ruleID]
				// 检查是否有共同的排除规则
				for _, excludedRuleID := range rule.Exclusions {
					if contains(otherRule.Exclusions, excludedRuleID) {
						pm.addConflict(rule.RuleID, ruleID, "conflicting_exclusions")
						break
					}
				}
			}
		}
	}
}

// addConflict 添加冲突信息
func (pm *PriorityManager) addConflict(ruleID, conflictRuleID, conflictType string) {
	conflict, exists := pm.conflicts[ruleID]
	if !exists {
		conflict = ConflictInfo{
			RuleID:        ruleID,
			ConflictsWith: []string{},
			Resolution:    "pending",
		}
	}
	
	// 检查是否已经存在该冲突
	if !contains(conflict.ConflictsWith, conflictRuleID) {
		conflict.ConflictsWith = append(conflict.ConflictsWith, conflictRuleID)
	}
	
	pm.conflicts[ruleID] = conflict
}

// GetConflicts 获取冲突信息
func (pm *PriorityManager) GetConflicts() map[string]ConflictInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	// 返回副本以避免并发修改
	conflicts := make(map[string]ConflictInfo)
	for ruleID, conflict := range pm.conflicts {
		conflicts[ruleID] = conflict
	}
	
	return conflicts
}

// ResolveConflict 解决冲突
func (pm *PriorityManager) ResolveConflict(ruleID string, resolution string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	conflict, exists := pm.conflicts[ruleID]
	if !exists {
		return fmt.Errorf("no conflicts found for rule %s", ruleID)
	}
	
	conflict.Resolution = resolution
	pm.conflicts[ruleID] = conflict
	
	return nil
}

// RemoveConflict 移除冲突
func (pm *PriorityManager) RemoveConflict(ruleID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	delete(pm.conflicts, ruleID)
}

// RemoveConflictsByRuleID 移除与特定规则相关的所有冲突
func (pm *PriorityManager) RemoveConflictsByRuleID(ruleID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// 移除规则自身的冲突
	delete(pm.conflicts, ruleID)
	
	// 移除其他规则与该规则的冲突
	for conflictRuleID, conflict := range pm.conflicts {
		newConflicts := []string{}
		for _, conflictingRuleID := range conflict.ConflictsWith {
			if conflictingRuleID != ruleID {
				newConflicts = append(newConflicts, conflictingRuleID)
			}
		}
		
		if len(newConflicts) == 0 {
			delete(pm.conflicts, conflictRuleID)
		} else {
			conflict.ConflictsWith = newConflicts
			pm.conflicts[conflictRuleID] = conflict
		}
	}
}

// IsRuleEnabled 检查规则是否启用
func (pm *PriorityManager) IsRuleEnabled(ruleID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	rule, exists := pm.rules[ruleID]
	return exists && rule.Enabled
}

// EnableRule 启用规则
func (pm *PriorityManager) EnableRule(ruleID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	rule, exists := pm.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule %s not found", ruleID)
	}
	
	rule.Enabled = true
	
	return nil
}

// DisableRule 禁用规则
func (pm *PriorityManager) DisableRule(ruleID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	rule, exists := pm.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule %s not found", ruleID)
	}
	
	rule.Enabled = false
	
	return nil
}

// SetRulePriority 设置规则优先级
func (pm *PriorityManager) SetRulePriority(ruleID string, priority int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	rule, exists := pm.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule %s not found", ruleID)
	}
	
	rule.Priority = priority
	
	return nil
}

// GetGroups 获取所有组
func (pm *PriorityManager) GetGroups() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	groups := make([]string, 0, len(pm.ruleGroups))
	for group := range pm.ruleGroups {
		groups = append(groups, group)
	}
	
	return groups
}

// GetRulesByPriorityRange 获取指定优先级范围的规则
func (pm *PriorityManager) GetRulesByPriorityRange(minPriority, maxPriority int) []*RulePriority {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	rules := make([]*RulePriority, 0)
	for _, rule := range pm.rules {
		if rule.Enabled && rule.Priority >= minPriority && rule.Priority<= maxPriority {
			rules = append(rules, rule)
		}
	}
	
	// 按优先级排序
	sort.Slice(rules, func(i, j int) bool{
		return rules[i].Priority < rules[j].Priority
	})
	
	return rules
}

// removeFromGroup 从组中移除规则
func (pm *PriorityManager) removeFromGroup(group, ruleID string) {
	ruleIDs := pm.ruleGroups[group]
	newRuleIDs := []string{}
	for _, id := range ruleIDs {
		if id != ruleID {
			newRuleIDs = append(newRuleIDs, id)
		}
	}
	
	if len(newRuleIDs) == 0 {
		delete(pm.ruleGroups, group)
	} else {
		pm.ruleGroups[group] = newRuleIDs
	}
}

// contains 检查切片是否包含元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}