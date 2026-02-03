package model

// Redis Stream 消息结构
type ProbeTask struct {
	TaskID     string `json:"task_id"`
	URL        string `json:"url"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Host       string `json:"host"`
	PolicyID   uint   `json:"policy_id"`
	RetryCount int    `json:"retry_count"`
}

// Stream 消息字段名
const (
	StreamKey         = "stream:icp:probe"
	ConsumerGroup     = "icp_workers"
	StreamFieldTaskID = "task_id"
	StreamFieldURL    = "url"
	StreamFieldIP     = "ip"
	StreamFieldPort   = "port"
	StreamFieldProto  = "protocol"
	StreamFieldPolicy = "policy_id"
	StreamFieldRetry  = "retry_count"
)

// Redis Key 前缀
const (
	KeyPrefixDailyCheck = "icp:daily:" // 每日去重缓存
	KeyPrefixLock       = "icp:lock:"  // 分布式锁
	KeyPrefixStat       = "icp:stat:"  // 统计缓存
)

// CacheEntry 缓存条目
type CacheEntry struct {
	IsRegistered int    `json:"is_registered"`
	ICPCode      string `json:"icp_code"`
	CheckTime    int64  `json:"check_time"`
}
