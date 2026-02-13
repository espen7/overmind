package lifecycle

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"go.uber.org/zap"
	"overmind/internal/kit/log"
)

// Lifecycle 管理应用程序的启动和优雅停机流程
type Lifecycle struct {
	ctx    context.Context    // 全局根 Context，用于通知所有组件停止
	cancel context.CancelFunc // 取消函数
	wg     sync.WaitGroup     // (保留字段，暂未使用) 等待组
	hooks  []Hook             // 注册的生命周期钩子
}

// Hook 定义单个组件的起停逻辑
type Hook struct {
	Name    string                      // 组件名称 (用于日志)
	OnStart func(context.Context) error // 启动回调
	OnStop  func(context.Context) error // 停止回调
}

// New 创建一个新的生命周期管理器
func New() *Lifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifecycle{
		ctx:    ctx,
		cancel: cancel,
		hooks:  make([]Hook, 0),
	}
}

// Append 添加一个新的组件钩子。
// 注意：启动时按添加顺序执行，停止时按逆序执行 (LIFO)。
func (l *Lifecycle) Append(hook Hook) {
	l.hooks = append(l.hooks, hook)
}

// Run 启动应用并阻塞，直到收到由于系统信号（SIGINT/SIGTERM）触发的关闭指令。
func (l *Lifecycle) Run() error {
	// 1. 按顺序启动所有组件
	for _, hook := range l.hooks {
		if hook.OnStart != nil {
			log.Info("Starting component", zap.String("name", hook.Name))
			if err := hook.OnStart(l.ctx); err != nil {
				log.Error("Failed to start component", zap.String("name", hook.Name), zap.Error(err))
				return err
			}
		}
	}

	log.Info("Application started. Press Ctrl+C to shut down.")

	// 2. 阻塞并等待系统信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	s := <-quit
	log.Info("Received signal, initiating shutdown...", zap.String("signal", s.String()))

	// 3. 按逆序停止所有组件 (LIFO)
	// 例如：先停止 HTTP Server (停止接收请求)，再停止 Database (关闭连接)
	l.cancel() // 取消根 Context，通知所有监听 ctx.Done() 的协程

	for i := len(l.hooks) - 1; i >= 0; i-- {
		hook := l.hooks[i]
		if hook.OnStop != nil {
			log.Info("Stopping component", zap.String("name", hook.Name))
			// OnStop 不需要关注根 Context 是否已取消，它通常需要一个新的 Context (带超时)
			// 这里我们传入 Background，具体的超时控制由 Hook 内部决定
			if err := hook.OnStop(context.Background()); err != nil {
				log.Error("Failed to stop component", zap.String("name", hook.Name), zap.Error(err))
				// 即使某个组件停止失败，我们仍继续尝试停止其他组件
			}
		}
	}

	log.Info("Shutdown complete.")
	return nil
}
