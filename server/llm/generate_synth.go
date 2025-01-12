package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/po-studio/server/config"
	sc "github.com/po-studio/server/supercollider"
	openai "github.com/sashabaranov/go-openai"
)

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

	if userPrompt == "" {
		userPrompt = `
		Generate a single SuperCollider SynthDef that creates a continuously evolving, musical ambient environment, rather than just sound effects. 
		The result should evoke a lush ambient track: layered, harmonic pads, gentle melodic fragments, and soft, evolving textures that feel like "music" rather than random noise.
		`
	}

	mainPrompt := fmt.Sprintf(`	
	USER PROMPT START
	%s
	USER PROMPT END

	Requirements and guidance:
	- Establish a tonal center, for example A minor, and quantize all pitched elements to notes in that scale (A, B, C, D, E, F, G) or their harmonic variants.
	- Use pitched oscillators (Sine, Saw) that evolve slowly, occasionally gliding or shifting, but staying within a musical scale.
	- Include a gentle melodic element that emerges over time. Use Demand UGens or slowly changing LFOs to pick pitches from a set scale.
	- Keep noise-based textures subtle, using them as soft, filtered washes that support the harmonic content rather than dominate it.
	- Introduce very subtle rhythmic pulses—delicate bell-like tones or soft plucked sounds—that feel organic and not like abrupt sound effects.
	- Use filtering and reverbs/delays to create a sense of spaciousness, but keep them musical and not overly chaotic.
	- Consider slow changes in timbre and spectral emphasis rather than wild, unpredictable modulations.
	- Limit overall distortion or harsh nonlinearity; any waveshaping should be gentle and maintain a musical feel.
	- Use amplitude management (Limiter, EnvGen) to keep levels safe and balanced at around 0.5 max amplitude.
	- Aim for about 300 lines of code to allow enough complexity for evolving musical structure.
	- Declare all variables at the start and assign the final sound to 'sound'. Do not declare any variables in the template more than once.

	Generate ONLY the core SuperCollider synthesis code that will be interpolated into this template:

	// SYNTHDEF TEMPLATE START
	SynthDef.new("name", { |out=0, amp=0.5|
		// YOUR CODE HERE
		Out.ar(out, sound * amp);
	}).writeDefFile("/app/supercollider/synthdefs");
	// SYNTHDEF TEMPLATE END
	
	Return ONLY the raw SuperCollider code that replaces // YOUR CODE HERE.
	`, userPrompt)

	// hardcode to O1Preview for now
	if model == "" {
		model = openai.O1Preview
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
			MaxCompletionTokens: 20000,
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
