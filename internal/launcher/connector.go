package launcher

import (
	"context"
	"time"
)

type ConnectionTarget struct {
	Host               string
	Port               uint16
	Database           string
	User               string
	SSLMode            string
	AdvancedParameters map[string]string
}

type Credential struct {
	Password string
}

type Clock interface {
	Now() time.Time
}

type ConnectionInfo struct {
	ServerVersion string
	Database      string
	Server        string
	Latency       time.Duration
}

type Session interface {
	Ping(context.Context) error
	Close()
}

type Connector interface {
	Test(context.Context, ConnectionTarget, Credential) (ConnectionInfo, error)
	Connect(context.Context, ConnectionTarget, Credential) (Session, ConnectionInfo, error)
}
