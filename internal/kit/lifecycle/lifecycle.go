package lifecycle

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"go.uber.org/zap"

	"overmind/internal/platform/logging"
)

// Lifecycle 负责统一管理组件启动顺序和优雅停机顺序。
type Lifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	hooks  []Hook
}

// Hook 描述单个组件的启动与停止回调。
type Hook struct {
	Name    string
	OnStart func(context.Context) error
	OnStop  func(context.Context) error
}

func New() *Lifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifecycle{
		ctx:    ctx,
		cancel: cancel,
		hooks:  make([]Hook, 0),
	}
}

func (l *Lifecycle) Append(hook Hook) {
	l.hooks = append(l.hooks, hook)
}

// Run 按注册顺序启动组件，并在收到系统信号后按逆序停机。
func (l *Lifecycle) Run() error {
	for _, hook := range l.hooks {
		if hook.OnStart != nil {
			logging.L().Info("starting component", zap.String("name", hook.Name))
			if err := hook.OnStart(l.ctx); err != nil {
				logging.L().Error("failed to start component", zap.String("name", hook.Name), zap.Error(err))
				return err
			}
		}
	}

	logging.L().Info("application started, waiting for shutdown signal")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	s := <-quit
	logging.L().Info("received signal, initiating shutdown", zap.String("signal", s.String()))

	l.cancel()
	for i := len(l.hooks) - 1; i >= 0; i-- {
		hook := l.hooks[i]
		if hook.OnStop != nil {
			logging.L().Info("stopping component", zap.String("name", hook.Name))
			if err := hook.OnStop(context.Background()); err != nil {
				logging.L().Error("failed to stop component", zap.String("name", hook.Name), zap.Error(err))
			}
		}
	}

	logging.L().Info("shutdown complete")
	return nil
}
