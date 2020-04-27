package gdrive

import (
	"fmt"

	"github.com/reddit/baseplate.go/log"
	"github.com/reddit/baseplate.go/randbp"
	"go.uber.org/zap"
	"google.golang.org/api/drive/v3"
)

// TraceID is a hopefully unique id to represent a trace
type TraceID uint64

func (id TraceID) String() string {
	return fmt.Sprintf("%016x", uint64(id))
}

// NewTraceID returns a new, random, and non-zero trace id.
func NewTraceID() TraceID {
	for {
		if id := randbp.R.Uint64(); id != 0 {
			return TraceID(id)
		}
	}
}

// TracedClient represents a group of Drive API calls with an authorized Drive
// client and a trace id.
//
// One trace can and usually does contain multiple API calls.
type TracedClient struct {
	*drive.Service

	Logger *zap.SugaredLogger

	id TraceID
}

// NewTracedClient creates a new, top level trace.
//
// logger arg is optional.
// If it's nil, top level logger will be used as the base.
func NewTracedClient(client *drive.Service, logger *zap.SugaredLogger) TracedClient {
	id := NewTraceID()
	if logger == nil {
		logger = log.With()
	}
	logger = logger.Named(id.String())
	return TracedClient{
		Service: client,
		Logger:  logger,
		id:      id,
	}
}

// NewChild creates a new child trace.
func (tc TracedClient) NewChild() TracedClient {
	id := NewTraceID()
	return TracedClient{
		Service: tc.Service,
		Logger:  tc.Logger.Named(id.String()),
		id:      id,
	}
}
