package gateway

// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import (
	"context"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	openaiGo "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const provider = "gateway"

type TextEmbeddingConfig struct {
	Dimensions     int                                       `json:"dimensions,omitempty"`
	EncodingFormat openaiGo.EmbeddingNewParamsEncodingFormat `json:"encodingFormat,omitempty"`
}

// EmbedderRef represents the main structure for an embedding model's definition.
type EmbedderRef struct {
	Name         string
	ConfigSchema TextEmbeddingConfig // Represents the schema, can be used for default config
	Label        string
	Supports     *ai.EmbedderSupports
	Dimensions   int
}

var (
	supportedModels = map[string]ai.ModelOptions{
		"gemini-2.5-flash": {
			Label:    "Gemini 2.5 Flash",
			Versions: []string{},
			Supports: &ai.ModelSupports{
				Multiturn:  true,
				Tools:      true,
				ToolChoice: true,
				SystemRole: true,
				// Media:       true,
				Constrained: ai.ConstrainedSupportNoTools,
			},
			Stage: ai.ModelStageStable,
		},
	}

	supportedEmbeddingModels = map[string]EmbedderRef{}
)

type OpenAI struct {
	// APIKey is the API key for the OpenAI API. If empty, the values of the environment variable "OPENAI_API_KEY" will be consulted.
	// Request a key at https://platform.openai.com/api-keys
	APIKey string
	// Optional: Opts are additional options for the OpenAI client.
	// Can include other options like WithOrganization, WithBaseURL, etc.
	Opts []option.RequestOption

	openAICompatible *compat_oai.OpenAICompatible
}

// Name implements genkit.Plugin.
func (o *OpenAI) Name() string {
	return provider
}

// Init implements genkit.Plugin.
func (o *OpenAI) Init(ctx context.Context) []api.Action {
	apiKey := o.APIKey

	// if api key is not set, get it from environment variable
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if apiKey == "" {
		panic("openai plugin initialization failed: apiKey is required")
	}

	if o.openAICompatible == nil {
		o.openAICompatible = &compat_oai.OpenAICompatible{}
	}

	// set the options
	o.openAICompatible.Opts = []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if len(o.Opts) > 0 {
		o.openAICompatible.Opts = append(o.openAICompatible.Opts, o.Opts...)
	}

	o.openAICompatible.Provider = provider
	compatActions := o.openAICompatible.Init(ctx)

	var actions []api.Action
	actions = append(actions, compatActions...)

	// define default models
	for model, opts := range supportedModels {
		aiModel := o.DefineModel(model, opts)
		action, ok := aiModel.(api.Action)
		if !ok {
			panic("model is not an action")
		}
		actions = append(actions, action)
	}

	// define default embedders
	for _, embedder := range supportedEmbeddingModels {
		opts := &ai.EmbedderOptions{
			ConfigSchema: core.InferSchemaMap(embedder.ConfigSchema),
			Label:        embedder.Label,
			Supports:     embedder.Supports,
			Dimensions:   embedder.Dimensions,
		}
		aiEmbedder := o.DefineEmbedder(embedder.Name, opts)
		action, ok := aiEmbedder.(api.Action)
		if !ok {
			panic("embedder is not an action")
		}
		actions = append(actions, action)
	}

	return actions
}

func (o *OpenAI) Model(g *genkit.Genkit, name string) ai.Model {
	return o.openAICompatible.Model(g, api.NewName(provider, name))
}

func (o *OpenAI) DefineModel(id string, opts ai.ModelOptions) ai.Model {
	return o.openAICompatible.DefineModel(provider, id, opts)
}

func (o *OpenAI) DefineEmbedder(id string, opts *ai.EmbedderOptions) ai.Embedder {
	return o.openAICompatible.DefineEmbedder(provider, id, opts)
}

func (o *OpenAI) Embedder(g *genkit.Genkit, name string) ai.Embedder {
	return o.openAICompatible.Embedder(g, api.NewName(provider, name))
}

func (o *OpenAI) ListActions(ctx context.Context) []api.ActionDesc {
	return o.openAICompatible.ListActions(ctx)
}

func (o *OpenAI) ResolveAction(atype api.ActionType, name string) api.Action {
	return o.openAICompatible.ResolveAction(atype, name)
}
