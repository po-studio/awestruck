package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/po-studio/go-webrtc-server/config"
	sc "github.com/po-studio/go-webrtc-server/supercollider"
	openai "github.com/sashabaranov/go-openai"
)

// NB: not ideal to depend on the LLM to set the right output path here
// also a waste of tokens since we know we need to write to a specific path
// for every synth in order to generate the synthdef binary
// return to this...

func GenerateSynthCode(provider, prompt, model string) (string, error) {
	var llmResponse string
	var err error

	log.Printf("[SYNTH-GEN] Starting synth generation with provider=%s, model=%s", provider, model)

	if provider == "" {
		provider = "openai"
		log.Printf("[SYNTH-GEN] No provider specified, defaulting to %s", provider)
	}

	switch provider {
	case "openai":
		log.Printf("[SYNTH-GEN] Generating synth code with OpenAI")
		llmResponse, err = generateWithOpenAI(prompt, model)
	}

	if err != nil {
		log.Printf("[SYNTH-GEN][ERROR] Failed to generate synth code: %v", err)
		return "", err
	}

	// Generate unique ID for the synth
	id := fmt.Sprintf("%s-%s-%s", provider, model, formatTimestamp())
	log.Printf("[SYNTH-GEN] Generated synth ID: %s", id)

	// Save the synthdef
	log.Printf("[SYNTH-GEN] Saving synthdef with ID=%s", id)
	if err := sc.SaveSynthDef(id, provider, model, llmResponse); err != nil {
		log.Printf("[SYNTH-GEN][ERROR] Failed to save synthdef: %v", err)
		return "", fmt.Errorf("failed to save synthdef: %v", err)
	}
	log.Printf("[SYNTH-GEN] Successfully saved synthdef")

	return llmResponse, nil
}

func generateWithOpenAI(userPrompt, model string) (string, error) {
	log.Printf("[OPENAI] Starting OpenAI code generation with model=%s", model)
	key := config.Get().OpenAIAPIKey
	mainPrompt := fmt.Sprintf(`
		Generate a single SuperCollider SynthDef that produces a continuously evolving, otherworldly sonic environment reminiscent of a complex handcrafted patch. The result should feel organic, layered, and immersive, with an intricate interplay of textures that evolve unpredictably over time.

		Requirements and guidance:
		- The synth should run continuously and evolve without external control messages.
		- Use multiple sources: a mixture of tonal elements (e.g., saw or sine waves), noisy textures (e.g., filtered noise), and percussive impulses.
		- Incorporate multiple layers of random triggers, pitch shifts, frequency modulation, and resonant filtering to achieve evolving complexity.
		- Employ LocalIn and LocalOut to create feedback loops, feeding signals back into themselves to build richness.
		- Utilize dynamic filtering (RLPF, HPF, BPF, etc.) modulated by slow-moving random LFOs, as well as amplitude tracking or pitch-tracking as nonlinear modulation sources.
		- Include pitch shifting (PitchShift.ar) and nonlinear waveshaping (e.g., tanh) to add harmonic richness and unpredictability.
		- Introduce various reverbs (GVerb, FreeVerb) and delay-based effects (CombL, CombC, DelayL) to produce a sense of depth and space. Large reverb times and spatialization (Splay, Rotate2, etc.) are encouraged.
		- Gradually bring in new elements over time with EnvGen, Line.kr, Demand UGens, or low-frequency triggers that reveal or hide layers as it evolves.
		- Keep amplitude safe: consider using Limiter or gentle amplitude envelopes, and ensure the final output does not exceed safe levels.
		- Aim for about 100-200 lines of code. Don’t worry about exact code length; just ensure enough complexity.
		- The final signal should be assigned to a variable called ‘sound’.
		- Do not include SuperCollider SynthDef wrapper code, comments, or markdown formatting in the final output – only the raw code that would go inside the SynthDef where ‘sound’ is defined.

		Generate ONLY the core SuperCollider synthesis code that will be interpolated into this template:

		SynthDef.new("name", { |out=0, amp=0.5|
			var sound;
			// YOUR CODE HERE - Define 'sound' variable
			Out.ar(out, sound * amp);
		}).writeDefFile("/app/supercollider/synthdefs");

		Requirements:
		1. Declare all variables at the start
		2. Final output must be assigned to 'sound' variable
		3. Keep amplitude levels safe (below 1.0)
		4. Use proper audio (ar) and control (kr) rates
		5. DO NOT include markdown code block markers or comments

		Return ONLY the raw SuperCollider code that would replace // YOUR CODE HERE. No comments, no SynthDef wrapper, no markdown formatting.
	`, userPrompt)
	if model == "" {
		model = openai.GPT4o
	}

	client := openai.NewClient(key)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: mainPrompt,
				},
			},
			MaxTokens: 10000,
		},
	)

	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	// Clean the response by removing markdown code block markers
	content := resp.Choices[0].Message.Content
	content = strings.TrimPrefix(content, "```supercollider")
	content = strings.TrimPrefix(content, "```scd")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	log.Printf("[OPENAI] Generated code (cleaned):\n%s", content)
	return content, nil
}

func formatTimestamp() string {
	return time.Now().Format("2006_01_02_15_04_05")
}
