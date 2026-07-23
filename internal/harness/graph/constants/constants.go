// Package constants provides constants for LangGraph Go.
package constants

// Reserved write keys.
const (
	// Input is for values passed as input to the graph.
	Input = "__input__"
	// Interrupt is for dynamic interrupts raised by nodes.
	Interrupt = "__interrupt__"
	// Resume is for values passed to resume a node after an interrupt.
	Resume = "__resume__"
	// Error is for errors raised by nodes.
	Error = "__error__"
	// NoWrites is a marker to signal node didn't write anything.
	NoWrites = "__no_writes__"
	// Tasks is for Send objects returned by nodes/edges.
	Tasks = "__pregel_tasks"
	// Return is for writes of a task where we simply record the return value.
	Return = "__return__"
	// Previous is the implicit branch that handles each node's Control values.
	Previous = "__previous__"
)

// Reserved cache namespaces.
const (
	// CacheNSWrites is the cache namespace for node writes.
	CacheNSWrites = "__pregel_ns_writes"
)

// Reserved config.configurable keys.
const (
	// ConfigKeySend holds the write function that accepts writes to state/edges/reserved keys.
	ConfigKeySend = "__pregel_send"
	// ConfigKeyRead holds the read function that returns a copy of the current state.
	ConfigKeyRead = "__pregel_read"
	// ConfigKeyCall holds the call function that accepts a node/func, args and returns a future.
	ConfigKeyCall = "__pregel_call"
	// ConfigKeyCheckpointer holds a BaseCheckpointSaver passed from parent graph to child graphs.
	ConfigKeyCheckpointer = "__pregel_checkpointer"
	// ConfigKeyStream holds a StreamProtocol passed from parent graph to child graphs.
	ConfigKeyStream = "__pregel_stream"
	// ConfigKeyCache holds a BaseCache made available to subgraphs.
	ConfigKeyCache = "__pregel_cache"
	// ConfigKeyResuming holds a boolean indicating if subgraphs should resume from a previous checkpoint.
	ConfigKeyResuming = "__pregel_resuming"
	// ConfigKeyTaskID holds the task ID for the current task.
	ConfigKeyTaskID = "__pregel_task_id"
	// ConfigKeyThreadID holds the thread ID for the current invocation.
	ConfigKeyThreadID = "thread_id"
	// ConfigKeyCheckpointMap holds a mapping of checkpoint_ns -> checkpoint_id for parent graphs.
	ConfigKeyCheckpointMap = "checkpoint_map"
	// ConfigKeyCheckpointID holds the current checkpoint_id, if any.
	ConfigKeyCheckpointID = "checkpoint_id"
	// ConfigKeyCheckpointNS holds the current checkpoint_ns, "" for root graph.
	ConfigKeyCheckpointNS = "checkpoint_ns"
	// ConfigKeyNodeFinished holds a callback to be called when a node is finished.
	ConfigKeyNodeFinished = "__pregel_node_finished"
	// ConfigKeyScratchpad holds a mutable dict for temporary storage scoped to the current task.
	ConfigKeyScratchpad = "__pregel_scratchpad"
	// ConfigKeyRunnerSubmit holds a function that receives tasks from runner.
	ConfigKeyRunnerSubmit = "__pregel_runner_submit"
	// ConfigKeyDurability holds the durability mode.
	ConfigKeyDurability = "__pregel_durability"
	// ConfigKeyRuntime holds a Runtime instance with context, store, stream writer, etc.
	ConfigKeyRuntime = "__pregel_runtime"
	// ConfigKeyResumeMap holds a mapping of task ns -> resume value for resuming tasks.
	ConfigKeyResumeMap = "__pregel_resume_map"
)

// Other constants.
const (
	// Push denotes push-style tasks, ie. those created by Send objects.
	Push = "__pregel_push"
	// Pull denotes pull-style tasks, ie. those triggered by edges.
	Pull = "__pregel_pull"
	// NSSep separates each level of a checkpoint namespace hierarchy (e.g. "parent|child").
	NSSep = "|"
	// NSEnd separates the namespace from the task_id within each level (e.g. "ns:task_id").
	NSEnd = ":"
	// Conf is the key for the configurable dict in RunnableConfig.
	Conf = "configurable"
	// NullTaskID is the task_id to use for writes that are not associated with a task.
	NullTaskID = "00000000-0000-0000-0000-000000000000"
	// Overwrite is the dict key for the overwrite value.
	Overwrite = "__overwrite__"
	// DefaultCheckpointMaxVersions is the default maximum number of checkpoint versions retained.
	DefaultCheckpointMaxVersions = 100
	// DefaultCheckpointListLimit is the default page size for listing checkpoints.
	DefaultCheckpointListLimit = 10
	// DefaultRecursionLimit is the default maximum Pregel superstep count.
	DefaultRecursionLimit = 50
)

// Public constants.
const (
	// TagNoStream is a tag to disable streaming for a chat model.
	TagNoStream = "nostream"
	// TagHidden is a tag to hide a node/edge from certain tracing/streaming environments.
	TagHidden = "langsmith:hidden"
	// End is the last (maybe virtual) node in graph-style Pregel.
	End = "__end__"
	// Start is the first (maybe virtual) node in graph-style Pregel.
	Start = "__start__"
)

// Reserved contains all reserved keys.
var Reserved = map[string]bool{
	TagHidden:              true,
	Input:                  true,
	Interrupt:              true,
	Resume:                 true,
	Error:                  true,
	NoWrites:               true,
	ConfigKeySend:          true,
	ConfigKeyRead:          true,
	ConfigKeyCall:          true,
	ConfigKeyCheckpointer:  true,
	ConfigKeyStream:        true,
	ConfigKeyCache:         true,
	ConfigKeyCheckpointMap: true,
	ConfigKeyResuming:      true,
	ConfigKeyTaskID:        true,
	ConfigKeyCheckpointID:  true,
	ConfigKeyCheckpointNS:  true,
	ConfigKeyNodeFinished:  true,
	ConfigKeyScratchpad:    true,
	ConfigKeyRunnerSubmit:  true,
	ConfigKeyDurability:    true,
	ConfigKeyRuntime:       true,
	ConfigKeyResumeMap:     true,
	Push:                   true,
	Pull:                   true,
	NSSep:                  true,
	NSEnd:                  true,
	Conf:                   true,
}

// IsReserved checks if a key is reserved.
func IsReserved(key string) bool {
	return Reserved[key]
}
