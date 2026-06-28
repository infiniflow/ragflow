//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Gap C — `GET /api/v1/agents/<canvas_id>/components/<component_id>/input-form`.
// Gap D — `POST /api/v1/agents/<canvas_id>/components/<component_id>/debug`.
//
// Both endpoints introspect the user_canvas DSL via
// internal/agent/dsl helpers and, for debug only, construct a runtime
// component via the production factory. The python reference uses
// @_require_canvas_access_sync/_async decorators which return
// OPERATING_ERROR (103) with the python permission message; we
// inline the check here via loader.LoadCanvasByID and surface the
// same envelope (CodeOperatingError + canvasNoAccessMessage) so
// existing clients can pattern-match the response.

package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/dsl"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// GetComponentInputForm GET /api/v1/agents/:canvas_id/components/:component_id/input-form
func (h *AgentHandler) GetComponentInputForm(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		jsonError(c, code, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	componentID := c.Param("component_id")
	if canvasID == "" || componentID == "" {
		jsonError(c, common.CodeArgumentError, "`canvas_id` and `component_id` are required.")
		return
	}

	cv, err := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		if err == dao.ErrUserCanvasNotFound {
			jsonError(c, common.CodeOperatingError, canvasNoAccessMessage)
			return
		}
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	form, err := dsl.ExtractComponentInputForm(cv.DSL, componentID)
	if err != nil {
		mapDSLError(c, componentID, err)
		return
	}
	c.JSON(200, gin.H{
		"code":    common.CodeSuccess,
		"data":    form,
		"message": "success",
	})
}

// DebugComponent POST /api/v1/agents/:canvas_id/components/:component_id/debug
//
// Body shape (python parity): {"params": {"input_name": {"value": ...}, ...}}
//
// The python reference calls `component.invoke(**{k: o["value"] for k, o in
// req["params"].items()})`. We replicate this by flattening each param's
// "value" field into the Invoke inputs map.
func (h *AgentHandler) DebugComponent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		jsonError(c, code, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	componentID := c.Param("component_id")
	if canvasID == "" || componentID == "" {
		jsonError(c, common.CodeArgumentError, "`canvas_id` and `component_id` are required.")
		return
	}

	var body struct {
		Params map[string]map[string]any `json:"params"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonError(c, common.CodeArgumentError, "Invalid request: "+err.Error())
		return
	}
	if body.Params == nil {
		jsonError(c, common.CodeArgumentError, "`params` is required.")
		return
	}

	cv, err := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		if err == dao.ErrUserCanvasNotFound {
			jsonError(c, common.CodeOperatingError, canvasNoAccessMessage)
			return
		}
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	name, err := dsl.ExtractComponentName(cv.DSL, componentID)
	if err != nil {
		mapDSLError(c, componentID, err)
		return
	}
	dslParams, _ := dsl.ExtractComponentParams(cv.DSL, componentID)

	// Build the Invoke inputs map by flattening the request body's
	// {param: {value: ...}} shape into {param: value}. Mirrors python
	// agent_api.py:830 (`component.invoke(**{k: o["value"] for k, o in
	// req["params"].items()})`).
	//
	// The body contract requires `params.*.value` — a missing value
	// field used to slip through as nil and still invoke the
	// component, which silently corrupted debug input. Now we fail
	// fast. CodeRabbit PR review #2.
	inputs := make(map[string]any, len(body.Params))
	for k, v := range body.Params {
		if v == nil {
			jsonError(c, common.CodeArgumentError, "`params."+k+".value` is required.")
			return
		}
		value, ok := v["value"]
		if !ok {
			jsonError(c, common.CodeArgumentError, "`params."+k+".value` is required.")
			return
		}
		inputs[k] = value
	}

	factory := runtime.DefaultFactory()
	if factory == nil {
		jsonError(c, common.CodeServerError, "component factory not initialised")
		return
	}
	comp, err := factory(name, dslParams)
	if err != nil {
		jsonError(c, common.CodeDataError, "component factory: "+err.Error())
		return
	}

	// D4: skip set_debug_inputs (python-only LLM debug hook). The
	// raw Invoke already supports the same inputs.
	//
	// Begin (and other stateful components) reads a *CanvasState
	// from the request context. We attach a fresh one here so
	// debug works on a single component without standing up the
	// full canvas compile.
	invokeCtx := runtime.WithState(c.Request.Context(), canvas.NewCanvasState("debug-"+componentID, "debug-task"))

	outputs, err := comp.Invoke(invokeCtx, inputs)
	if err != nil {
		jsonError(c, common.CodeServerError, "invoke: "+err.Error())
		return
	}

	c.JSON(200, gin.H{
		"code":    common.CodeSuccess,
		"data":    outputs,
		"message": "success",
	})
}

// mapDSLError translates a dsl extractor error into a 102 envelope.
// Centralised so both handlers return consistent error shapes. The
// default arm surfaces unknown errors as 500 (server error) so a
// future unmapped dsl sentinel doesn't silently masquerade as a
// user-data problem (code-review MEDIUM).
func mapDSLError(c *gin.Context, componentID string, err error) {
	switch {
	case errors.Is(err, dsl.ErrComponentNotFound):
		jsonError(c, common.CodeDataError, "component not found: "+componentID)
	case errors.Is(err, dsl.ErrMissingInputForm):
		jsonError(c, common.CodeDataError, "component has no input_form: "+componentID)
	case errors.Is(err, dsl.ErrMalformedDSL):
		jsonError(c, common.CodeDataError, "malformed dsl: "+err.Error())
	default:
		jsonError(c, common.CodeServerError, "internal: "+err.Error())
	}
}
