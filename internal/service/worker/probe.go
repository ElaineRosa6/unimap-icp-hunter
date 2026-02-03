package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unimap-icp-hunter/project/internal/model"
	"go.uber.org/zap"
)

// ProbeExecutor HTTP探测执行器
type ProbeExecutor struct {
	redisClient *redis.Client
	config      ProbeConfig
	logger      *zap.Logger
	wg          sync.WaitGroup
	stopChan    chan struct{}
	mu          sync.Mutex
}

// ProbeConfig 探测配置
type ProbeConfig struct {
	QueueName      string
	ConsumerGroup  string
	ConsumerName   string
	WorkerPoolSize int
	HTTP           HTTPConfig
	Retry          RetryConfig
}

// HTTPConfig HTTP请求配置
type HTTPConfig struct {
	Timeout          time.Duration
	MaxRedirects     int
	UserAgent        string
	FollowRedirects  bool
	DisableTLSVerify bool
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts   int
	BackoffFactor int
	InitialDelay  time.Duration
}

// NewProbeExecutor 创建探测执行器
func NewProbeExecutor(redisClient *redis.Client, config ProbeConfig, logger *zap.Logger) *ProbeExecutor {
	return &ProbeExecutor{
		redisClient: redisClient,
		config:      config,
		logger:      logger,
		stopChan:    make(chan struct{}),
	}
}

// Start 启动工作协程池
func (p *ProbeExecutor) Start(ctx context.Context) error {
	// 创建消费者组（如果不存在）
	err := p.createConsumerGroup(ctx)
	if err != nil {
		return fmt.Errorf("failed to create consumer group: %v", err)
	}

	p.logger.Info("Starting probe executor", zap.Int("workers", p.config.WorkerPoolSize))

	// 启动工作协程
	for i := 0; i < p.config.WorkerPoolSize; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}

	return nil
}

// Stop 停止执行器
func (p *ProbeExecutor) Stop() {
	close(p.stopChan)
	p.wg.Wait()
}

// createConsumerGroup 创建Redis Stream消费者组
func (p *ProbeExecutor) createConsumerGroup(ctx context.Context) error {
	// 检查消费者组是否存在
	_, err := p.redisClient.XInfoGroups(ctx, p.config.QueueName).Result()
	if err != nil && strings.Contains(err.Error(), "NOGROUP") {
		// 创建消费者组
		_, err = p.redisClient.XGroupCreate(ctx, p.config.QueueName, p.config.ConsumerGroup, "0").Result()
		if err != nil {
			return err
		}
	}
	return nil
}

// worker 工作协程
func (p *ProbeExecutor) worker(ctx context.Context, workerID int) {
	defer p.wg.Done()

	logger := p.logger.With(zap.Int("worker_id", workerID))

	for {
		select {
		case <-p.stopChan:
			logger.Info("Worker stopped")
			return
		case <-ctx.Done():
			logger.Info("Worker context cancelled")
			return
		default:
			// 从Redis Stream读取任务
			tasks, err := p.readTask(ctx)
			if err != nil {
				if err == redis.Nil {
					// 没有任务，短暂休眠
					time.Sleep(1 * time.Second)
					continue
				}
				logger.Error("Failed to read task", zap.Error(err))
				continue
			}

			// 处理任务
			for _, task := range tasks {
				p.processTask(ctx, logger, task)
			}
		}
	}
}

// readTask 从Redis Stream读取任务
func (p *ProbeExecutor) readTask(ctx context.Context) ([]*model.ProbeTask, error) {
	// 使用XREADGROUP读取任务
	streams, err := p.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    p.config.ConsumerGroup,
		Consumer: p.config.ConsumerName,
		Streams:  []string{p.config.QueueName, ">"},
		Count:    1,
		Block:    5 * time.Second,
	}).Result()

	if err != nil {
		return nil, err
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, redis.Nil
	}

	tasks := []*model.ProbeTask{}
	for _, msg := range streams[0].Messages {
		task := &model.ProbeTask{}

		// 解析字段
		if val, ok := msg.Values[model.StreamFieldTaskID].(string); ok {
			task.TaskID = val
		}
		if val, ok := msg.Values[model.StreamFieldURL].(string); ok {
			task.URL = val
		}
		if val, ok := msg.Values[model.StreamFieldIP].(string); ok {
			task.IP = val
		}
		if val, ok := msg.Values[model.StreamFieldPort].(string); ok {
			if port, err := strconv.Atoi(val); err == nil {
				task.Port = port
			} else {
				p.logger.Warn("Failed to parse port", zap.String("value", val), zap.Error(err))
			}
		} else if val, ok := msg.Values[model.StreamFieldPort].(int64); ok {
			task.Port = int(val)
		}
		if val, ok := msg.Values[model.StreamFieldProto].(string); ok {
			task.Protocol = val
		}
		if val, ok := msg.Values[model.StreamFieldPolicy].(string); ok {
			if policyID, err := strconv.ParseUint(val, 10, 32); err == nil {
				task.PolicyID = uint(policyID)
			} else {
				p.logger.Warn("Failed to parse policy ID", zap.String("value", val), zap.Error(err))
			}
		} else if val, ok := msg.Values[model.StreamFieldPolicy].(int64); ok {
			task.PolicyID = uint(val)
		}
		if val, ok := msg.Values[model.StreamFieldRetry].(string); ok {
			if retryCount, err := strconv.Atoi(val); err == nil {
				task.RetryCount = retryCount
			} else {
				p.logger.Warn("Failed to parse retry count", zap.String("value", val), zap.Error(err))
			}
		} else if val, ok := msg.Values[model.StreamFieldRetry].(int64); ok {
			task.RetryCount = int(val)
		}

		// 保存message ID用于ACK
		task.TaskID = msg.ID

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// processTask 处理单个任务
func (p *ProbeExecutor) processTask(ctx context.Context, logger *zap.Logger, task *model.ProbeTask) {
	// 检查URL有效性
	if !p.isValidURL(task.URL) {
		logger.Warn("Invalid URL", zap.String("url", task.URL))
		p.ackTask(ctx, task.TaskID)
		return
	}

	// 执行HTTP请求
	response, err := p.executeHTTP(ctx, task.URL)
	if err != nil {
		logger.Error("HTTP request failed", zap.String("url", task.URL), zap.Error(err))

		// 重试逻辑
		if task.RetryCount < p.config.Retry.MaxAttempts {
			p.retryTask(ctx, task)
		} else {
			// 记录失败
			p.recordFailure(ctx, task, err)
			p.ackTask(ctx, task.TaskID)
		}
		return
	}

	// 成功，发送到分析器
	p.sendToAnalyzer(ctx, task, response)
	p.ackTask(ctx, task.TaskID)
}

// isValidURL 验证URL格式
func (p *ProbeExecutor) isValidURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	if parsed.Host == "" {
		return false
	}

	return true
}

// executeHTTP 执行HTTP请求
func (p *ProbeExecutor) executeHTTP(ctx context.Context, targetURL string) (*HTTPResponse, error) {
	// 创建带超时的context
	httpCtx, cancel := context.WithTimeout(ctx, p.config.HTTP.Timeout)
	defer cancel()

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: p.config.HTTP.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !p.config.HTTP.FollowRedirects {
				return http.ErrUseLastResponse
			}
			if len(via) >= p.config.HTTP.MaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: p.config.HTTP.DisableTLSVerify,
			},
			DisableKeepAlives: true,
		},
	}

	// 创建请求
	req, err := http.NewRequestWithContext(httpCtx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	// 设置Header
	req.Header.Set("User-Agent", p.config.HTTP.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "close")

	// 执行请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 限制1MB
	if err != nil {
		return nil, err
	}

	// 提取响应头
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[strings.ToLower(k)] = v[0]
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       string(body),
		Location:   resp.Request.URL.String(),
	}, nil
}

// retryTask 重试任务
func (p *ProbeExecutor) retryTask(ctx context.Context, task *model.ProbeTask) {
	task.RetryCount++

	// 计算退避时间
	delay := p.config.Retry.InitialDelay
	for i := 1; i < task.RetryCount; i++ {
		delay *= time.Duration(p.config.Retry.BackoffFactor)
	}

	// 延迟后重新入队
	go func() {
		time.Sleep(delay)

		// 重新推送到队列
		fields := map[string]interface{}{
			model.StreamFieldTaskID: task.TaskID,
			model.StreamFieldURL:    task.URL,
			model.StreamFieldIP:     task.IP,
			model.StreamFieldPort:   task.Port,
			model.StreamFieldProto:  task.Protocol,
			model.StreamFieldPolicy: task.PolicyID,
			model.StreamFieldRetry:  task.RetryCount,
		}

		_, err := p.redisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: p.config.QueueName,
			Values: fields,
		}).Result()

		if err != nil {
			p.logger.Error("Failed to retry task", zap.Error(err))
		}
	}()
}

// ackTask 确认任务完成
func (p *ProbeExecutor) ackTask(ctx context.Context, messageID string) {
	if err := p.redisClient.XAck(ctx, p.config.QueueName, p.config.ConsumerGroup, messageID).Err(); err != nil {
		p.logger.Error("Failed to acknowledge task", zap.String("message_id", messageID), zap.Error(err))
	}
}

// recordFailure 记录失败任务
func (p *ProbeExecutor) recordFailure(ctx context.Context, task *model.ProbeTask, err error) {
	// 可以将失败信息存储到数据库或日志
	p.logger.Warn("Task failed permanently",
		zap.String("url", task.URL),
		zap.Int("retry_count", task.RetryCount),
		zap.Error(err),
	)
}

// sendToAnalyzer 发送结果到分析器（通过channel或消息队列）
func (p *ProbeExecutor) sendToAnalyzer(ctx context.Context, task *model.ProbeTask, response *HTTPResponse) {
	// 这里应该发送到内容分析器
	// 简化实现：直接调用分析器
	analyzer := NewContentAnalyzer(p.logger)
	result := analyzer.Analyze(task, response)

	// 存储结果到数据库
	p.storeResult(ctx, result)
}

// storeResult 存储检测结果
func (p *ProbeExecutor) storeResult(ctx context.Context, result *model.ICPCheck) {
	// 这里应该调用repository层存储到数据库
	// 简化实现：打印日志
	p.logger.Info("ICP Check Result",
		zap.String("url", result.URL),
		zap.Int("is_registered", result.IsRegistered),
		zap.String("icp_code", result.ICPCode),
	)
}

// HTTPResponse HTTP响应结构
type HTTPResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       string
	Location   string
}
