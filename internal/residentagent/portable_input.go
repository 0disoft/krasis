package residentagent

type productSessionGuard interface {
	productInputGuard
	Done() <-chan struct{}
	Close() error
}
