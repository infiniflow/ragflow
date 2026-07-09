package replay

import (
	"encoding/json"
	"fmt"
	"time"

	"ragflow/internal/harness/events"
	"ragflow/internal/harness/graph/constants"
)

// BuildCheckpoint reconstructs a flat map[string]any checkpoint from a sequence
// of events leading up to a fork point. This allows the Pregel engine to resume
// execution from that state as if it had been checkpointed during the original run.
//
// The returned map contains:
//   - Channel values extracted from EventStateWrite events
//   - __completed_tasks__ from EventNodeEnd events (NUL-separated)
//   - __step__ from the last EventStepStart/EventStepEnd event
//   - __last_state__ (JSON serialised)
//   - __last_completed_node__ from the last node event
//   - checkpoint_id metadata
//
// The second return value is the reconstructed checkpoint_id.
func BuildCheckpoint(originalEvents []*events.Event, threadID string) (map[string]any, string) {
	cp := make(map[string]any)
	cp[constants.ConfigKeyThreadID] = threadID

	checkpointID := fmt.Sprintf("fork-cp-%s-%d", threadID, time.Now().UnixNano())
	cp[constants.ConfigKeyCheckpointID] = checkpointID
	cp["__pregel_checkpoint_id"] = checkpointID

	var completedTasks []string
	var lastCompletedNode string
	var lastStep int

	// Collect channel values from state writes, track completed nodes.
	for _, ev := range originalEvents {
		switch ev.Type {
		case events.EventStateWrite:
			var st events.StateTransitionPayload
			if ev.Payload != nil {
				_ = json.Unmarshal(ev.Payload, &st)
			}
			if st.Channel != "" {
				cp[st.Channel] = st.NewValue
			}

		case events.EventNodeEnd:
			completedTasks = append(completedTasks, ev.Node)
			lastCompletedNode = ev.Node

		case events.EventStepEnd:
			if ev.Step > lastStep {
				lastStep = ev.Step
			}
		}
	}

	// If there are state writes, serialise the accumulated map as last_state.
	if len(cp) > 2 { // more than just thread_id and checkpoint_id
		lastState := make(map[string]any)
		for k, v := range cp {
			if k != constants.ConfigKeyThreadID && k != constants.ConfigKeyCheckpointID && k != "__pregel_checkpoint_id" {
				lastState[k] = v
			}
		}
		if ls, err := json.Marshal(lastState); err == nil {
			cp["__last_state__"] = string(ls)
		}
	}

	// Serialise completed tasks as NUL-separated string.
	if len(completedTasks) > 0 {
		var sb []byte
		for i, task := range completedTasks {
			if i > 0 {
				sb = append(sb, 0) // NUL separator
			}
			sb = append(sb, task...)
		}
		cp["__completed_tasks__"] = string(sb)
	}

	if lastCompletedNode != "" {
		cp["__last_completed_node__"] = lastCompletedNode
	}

	cp["__step__"] = float64(lastStep)

	return cp, checkpointID
}
