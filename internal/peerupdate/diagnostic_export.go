package peerupdate

import (
	"context"
	"errors"
)

type diagnosticExport struct {
	peer, id string
	ctx      context.Context
	done     chan struct{}
	err      error // Published by closing done.
}

func (m *Manager) hasDiagnosticExporter() bool {
	return m.ExportDiagnosticContext != nil || m.ExportDiagnostic != nil
}

// At most one actual exporter survives cancellation. Waiters may leave, but
// cannot release its slot or start another filesystem call while it is stuck.
func (m *Manager) runDiagnosticExport(ctx context.Context, peer, id string, authorized func() bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validID(peer, 64) || !validID(id, 32) || !m.hasDiagnosticExporter() || !authorized() {
		return ErrDenied
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrDenied
	}
	m.diagnostic.mu.Lock()
	job := m.diagnostic.export
	if job != nil {
		m.diagnostic.mu.Unlock()
		m.mu.Unlock()
		if job.peer != peer || job.id != id {
			return ErrBusy
		}
		return waitDiagnosticExport(ctx, job)
	}
	if m.diagnosticRequestHandledLocked(peer, id) {
		m.diagnostic.mu.Unlock()
		m.mu.Unlock()
		return nil
	}
	run, cancel := context.WithTimeout(ctx, diagnosticPollTimeout)
	job = &diagnosticExport{peer: peer, id: id, ctx: run, done: make(chan struct{})}
	m.diagnostic.export = job
	if m.cancels == nil {
		m.cancels = make(map[uint64]context.CancelFunc)
	}
	m.nextWorker++
	worker := m.nextWorker
	m.cancels[worker] = cancel
	m.workers.Add(1)
	m.diagnostic.mu.Unlock()
	m.mu.Unlock()
	go func() {
		defer m.workers.Done()
		defer cancel()
		if run.Err() != nil {
			job.err = run.Err()
		} else if !authorized() {
			job.err = ErrDenied
		} else if m.ExportDiagnosticContext != nil {
			job.err = m.ExportDiagnosticContext(run, peer, id)
		} else {
			job.err = m.ExportDiagnostic(peer, id)
		}
		if job.err == nil {
			job.err = run.Err()
		}
		if job.err != nil {
			code := "peer_diagnostic_export_failed"
			if errors.Is(job.err, context.Canceled) {
				code = "peer_diagnostic_export_cancelled"
			} else if errors.Is(job.err, context.DeadlineExceeded) {
				code = "peer_diagnostic_export_timeout"
			}
			m.event(code)
		}
		m.mu.Lock()
		delete(m.cancels, worker)
		m.diagnostic.mu.Lock()
		m.diagnostic.export = nil
		m.diagnostic.mu.Unlock()
		m.mu.Unlock()
		close(job.done)
	}()
	return waitDiagnosticExport(ctx, job)
}

func waitDiagnosticExport(ctx context.Context, job *diagnosticExport) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-job.ctx.Done():
		// The worker cancels this context during cleanup after publishing
		// done. That cancellation must not replace a completed result.
		select {
		case <-job.done:
			return job.err
		default:
			return job.ctx.Err()
		}
	case <-job.done:
		return job.err
	}
}
