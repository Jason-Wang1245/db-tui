package launcher

import "context"

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

type ConnectionInfo struct {
	ServerVersion string
	Database      string
	Server        string
}

type Session interface {
	Ping(context.Context) error
	Close()
}

type Connector interface {
	Test(context.Context, ConnectionTarget, Credential) (ConnectionInfo, error)
	Connect(context.Context, ConnectionTarget, Credential) (Session, ConnectionInfo, error)
}
