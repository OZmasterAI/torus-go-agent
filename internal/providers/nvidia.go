package providers

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// nvidiaNIMBaseURL is the OpenAI-compatible NVIDIA NIM inference endpoint.
const nvidiaNIMBaseURL = "https://integrate.api.nvidia.com/v1"

// NvidiaNIMModel is a chat model listed by the live NVIDIA NIM catalog.
type NvidiaNIMModel struct {
	ID      string
	OwnedBy string
	Created int64
}

// NvidiaNIMExcludeSubstrings lists substrings that identify non-chat models
// (embeddings, rerankers, vision/audio, image gen, biology, etc.) so they can
// be filtered out of the free chat-model set.
var NvidiaNIMExcludeSubstrings = []string{
	"embed", "bge", "nv-embed", "rerankqa", "reward", "neva", "nvclip",
	"streampetr", "deplot", "paligemma", "kosmos", "nemoretriever",
	"starcoder", "fuyu", "parse", "grounding-dino", "esm2", "diffdock",
	"molmim", "genomics", "riva", "voicechat", "studiovoice", "eyecontact",
	"parakeet", "canary", "vila", "cosmos-transfer", "genmol", "alphafold",
	"openfold", "msa-search", "sparsedrive", "bevformer", "usdsearch",
	"usdvalidate", "usdcode", "megatron-1b-nmt", "proteinmpnn",
	"ai-generated-image", "stable-diffusion", "flux", "trellis",
	"magpie-tts", "world-2", "arctic-embed",
}

// nvidiaNIMFreeModelFallback seeds the free set when the live catalog is
// unreachable (offline / API down). Kept broad so behaviour degrades to a
// reasonable static list rather than nothing.
var nvidiaNIMFreeModelFallback = []string{
	"qwen/qwen3.5-122b-a10b",
	"z-ai/glm-5.2",
	"stepfun-ai/step-3.5-flash",
	"minimaxai/minimax-m2.5",
	"deepseek-ai/deepseek-v3.2",
	"deepseek-ai/deepseek-v3.1-terminus",
	"moonshotai/kimi-k2-thinking",
	"qwen/qwen3-coder-480b-a35b-instruct",
	"openai/gpt-oss-120b",
	"nvidia/llama-3.3-nemotron-super-49b-v1.5",
	"nvidia/nemotron-3-super-120b-a12b",
}

// isNvidiaNIMChatModel reports whether a model ID is a text/chat model (i.e. not
// matched by any of the non-chat exclude substrings).
func isNvidiaNIMChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, sub := range NvidiaNIMExcludeSubstrings {
		if strings.Contains(lower, sub) {
			return false
		}
	}
	return true
}

type nvidiaNIMModelsResp struct {
	Data []struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// FetchNvidiaNIMChatModels fetches the live NVIDIA NIM model catalog and returns
// the chat models (non-chat models filtered out), newest first. Because NIM's
// /v1/models endpoint exposes no pricing/free flag, every listed chat model is
// treated as a free hosted endpoint — this is what makes the set self-update as
// NVIDIA adds/removes models. Returns nil on any error; callers fall back to
// nvidiaNIMFreeModelFallback.
func FetchNvidiaNIMChatModels() []NvidiaNIMModel {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(nvidiaNIMBaseURL + "/models")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var data nvidiaNIMModelsResp
	if json.Unmarshal(body, &data) != nil {
		return nil
	}

	var out []NvidiaNIMModel
	for _, m := range data.Data {
		if isNvidiaNIMChatModel(m.ID) {
			out = append(out, NvidiaNIMModel{ID: m.ID, OwnedBy: m.OwnedBy, Created: m.Created})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}

// FetchNvidiaNIMFreeModelIDs returns the IDs of the live free NVIDIA NIM chat
// models, falling back to nvidiaNIMFreeModelFallback when the catalog is
// unreachable.
func FetchNvidiaNIMFreeModelIDs() []string {
	models := FetchNvidiaNIMChatModels()
	if len(models) == 0 {
		ids := make([]string, len(nvidiaNIMFreeModelFallback))
		copy(ids, nvidiaNIMFreeModelFallback)
		return ids
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}
