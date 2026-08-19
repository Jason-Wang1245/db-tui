package postgres

import (
	"sync"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jason-Wang1245/db-tui/internal/sqltab"
)

type noticeRouter struct {
	mu    sync.RWMutex
	sinks map[*pgconn.PgConn]*noticeCollector
}

type noticeCollector struct {
	mu      sync.Mutex
	notices []sqltab.Notice
	bytes   int64
	cutOff  bool
}

func newNoticeRouter() *noticeRouter {
	return &noticeRouter{sinks: make(map[*pgconn.PgConn]*noticeCollector)}
}

func (router *noticeRouter) register(connection *pgconn.PgConn) (*noticeCollector, func()) {
	collector := &noticeCollector{}
	router.mu.Lock()
	router.sinks[connection] = collector
	router.mu.Unlock()
	return collector, func() {
		router.mu.Lock()
		delete(router.sinks, connection)
		router.mu.Unlock()
	}
}

func (router *noticeRouter) handle(connection *pgconn.PgConn, notice *pgconn.Notice) {
	if router == nil || notice == nil {
		return
	}
	router.mu.RLock()
	collector := router.sinks[connection]
	router.mu.RUnlock()
	if collector == nil {
		return
	}
	severity := notice.SeverityUnlocalized
	if severity == "" {
		severity = notice.Severity
	}
	captured := sqltab.Notice{
		Severity: sanitizeServerText(severity),
		Message:  sanitizeServerText(notice.Message),
		SQLState: sanitizeSQLState(notice.Code),
		Detail:   sanitizeServerText(notice.Detail),
		Hint:     sanitizeServerText(notice.Hint),
	}
	size := int64(len(captured.Severity) + len(captured.Message) + len(captured.SQLState) + len(captured.Detail) + len(captured.Hint))
	collector.mu.Lock()
	if collector.bytes <= sqltab.MaxCapturedDataBytes-size {
		collector.notices = append(collector.notices, captured)
		collector.bytes += size
	} else {
		collector.cutOff = true
	}
	collector.mu.Unlock()
}

func (collector *noticeCollector) snapshot() ([]sqltab.Notice, bool) {
	if collector == nil {
		return nil, false
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return append([]sqltab.Notice(nil), collector.notices...), collector.cutOff
}
