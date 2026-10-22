// Mole - Ultra-lightweight personal AI agent

package agent

import (
	"github.com/kiry163/mole/pkg/agent/interfaces"
	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/media"
	"github.com/kiry163/mole/pkg/providers"
)

// Pipeline holds the runtime dependencies used by Pipeline methods.
// It is constructed by runTurn via NewPipeline and passed to sub-methods
// so that the coordinator can delegate phase execution.
type Pipeline struct {
	Bus            interfaces.MessageBus
	Cfg            *config.Config
	ContextManager *sessionContextManager
	Fallback       *providers.FallbackChain
	MediaStore     media.MediaStore
	al             *AgentLoop
}

// NewPipeline creates a Pipeline from an AgentLoop instance.
func NewPipeline(al *AgentLoop) *Pipeline {
	return &Pipeline{
		Bus:            al.bus,
		Cfg:            al.GetConfig(),
		ContextManager: al.contextManager,
		Fallback:       al.fallback,
		MediaStore:     al.mediaStore,
		al:             al,
	}
}
