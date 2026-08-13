// Package core contains the deliberately small set of primitives shared by
// feature packages.
package core

type (
	WorkspaceID string
	TabID       string
	OperationID string
	RequestID   uint64
)

type RequestMeta struct {
	Workspace WorkspaceID
	Tab       TabID
	Operation OperationID
	Request   RequestID
}

func (m RequestMeta) Matches(other RequestMeta) bool {
	return m == other
}
