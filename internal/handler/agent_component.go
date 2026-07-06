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
	"context"
	"errors"
	"fmt"
	"strings"

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
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	componentID := c.Param("component_id")
	if canvasID == "" || componentID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`canvas_id` and `component_id` are required.")
		return
	}

	cv, err := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		if err == dao.ErrUserCanvasNotFound {
			common.ResponseWithCodeData(c, common.CodeOperatingError, nil, canvasNoAccessMessage)
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	form, err := h.componentInputForm(c.Request.Context(), cv.DSL, componentID, user.ID)
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

// componentInputForm returns the input-form schema for a single component.
// It first tries the static input_form stored in the DSL; if the component
// does not define one (e.g. Agent components that generate it dynamically),
// it instantiates the runtime component and calls its GetInputForm method.
// This mirrors Python's Canvas.get_component_input_form which invokes the
// component's own get_input_form when the static field is absent.
func (h *AgentHandler) componentInputForm(ctx context.Context, dslMap map[string]any, componentID, userID string) (map[string]any, error) {
	form, err := dsl.ExtractComponentInputForm(dslMap, componentID)
	if err == nil {
		return form, nil
	}
	if !errors.Is(err, dsl.ErrMissingInputForm) {
		return nil, err
	}

	name, err := dsl.ExtractComponentName(dslMap, componentID)
	if err != nil {
		return nil, err
	}
	params, _ := dsl.ExtractComponentParams(dslMap, componentID)
	comp, err := runtime.DefaultFactory()(name, params)
	if err != nil {
		return nil, fmt.Errorf("%w: component factory: %v", dsl.ErrMalformedDSL, err)
	}
	getter, ok := comp.(interface{ GetInputForm() map[string]any })
	if !ok {
		return nil, dsl.ErrMissingInputForm
	}
	form = getter.GetInputForm()
	if form == nil {
		form = map[string]any{}
	}
	return form, nil
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
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	componentID := c.Param("component_id")
	if canvasID == "" || componentID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`canvas_id` and `component_id` are required.")
		return
	}

	var body struct {
		Params map[string]map[string]any `json:"params"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	if body.Params == nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`params` is required.")
		return
	}

	cv, err := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		if err == dao.ErrUserCanvasNotFound {
			common.ResponseWithCodeData(c, common.CodeOperatingError, nil, canvasNoAccessMessage)
			return
		}
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
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
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"`params."+k+".value` is required.")
			return
		}
		value, ok := v["value"]
		if !ok {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"`params."+k+".value` is required.")
			return
		}
		inputs[k] = value
	}

	factory := runtime.DefaultFactory()
	if factory == nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "component factory not initialised")
		return
	}
	comp, err := factory(name, dslParams)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "component factory: "+err.Error())
		return
	}

	// D4: skip set_debug_inputs (python-only LLM debug hook). The
	// raw Invoke already supports the same inputs.
	//
	// Begin (and other stateful components) reads a *CanvasState
	// from the request context. We attach a fresh one here so
	// debug works on a single component without standing up the
	// full canvas compile.
	//
	// Seed state.Sys["tenant_id"] with the canvas owner so that
	// components which resolve LLM credentials from the tenant
	// tables (e.g. AgentComponent) can find the API key in single-
	// component debug mode. Mirrors Python's @add_tenant_id_to_kwargs
	// decorator for the debug endpoint.
	debugState := canvas.NewCanvasState("debug-"+componentID, "debug-task")
	debugState.Sys["tenant_id"] = user.ID
	for key, value := range inputs {
		if strings.HasPrefix(key, "sys.") && key != "sys.tenant_id" {
			debugState.Sys[strings.TrimPrefix(key, "sys.")] = value
		}
	}
	invokeCtx := runtime.WithState(c.Request.Context(), debugState)

	outputs, err := comp.Invoke(invokeCtx, inputs)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "invoke: "+err.Error())
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
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "component not found: "+componentID)
	case errors.Is(err, dsl.ErrMissingInputForm):
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "component has no input_form: "+componentID)
	case errors.Is(err, dsl.ErrMalformedDSL):
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "malformed dsl: "+err.Error())
	default:
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "internal: "+err.Error())
	}
}
